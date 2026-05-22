package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// User is a thin row mapping for the users table. Tokens are stored verbatim
// (random 32-char base64-url strings); we don't hash them for now — the
// trust model is "anyone who can read the SQLite file already has admin".
type User struct {
	ID            int64
	Name          string
	Token         string
	CreatedAt     time.Time
	LastSeenAt    sql.NullTime
	DifficultyMin int // 1..6 — composer filters tasks to this band
	DifficultyMax int // 1..6 — see UpdateUserPrefs for validation
}

// DifficultyFloor and DifficultyCeiling bracket the legal preference values
// (and the tasks.difficulty CHECK in the schema). Kept here so handlers
// don't have to reach for hardcoded literals.
const (
	DifficultyFloor   = 1
	DifficultyCeiling = 6
)

var ErrUserNotFound = errors.New("user not found")

// LookupUserByToken returns the user matching the cookie token, or
// ErrUserNotFound. Read-only; safe to call from the auth middleware on every
// request.
func LookupUserByToken(ctx context.Context, db *sql.DB, token string) (*User, error) {
	if token == "" {
		return nil, ErrUserNotFound
	}
	var u User
	var createdAt int64
	var lastSeen sql.NullInt64
	row := db.QueryRowContext(ctx,
		`SELECT id, name, token, created_at, last_seen_at, difficulty_min, difficulty_max
		 FROM users WHERE token = ?`, token)
	if err := row.Scan(&u.ID, &u.Name, &u.Token, &createdAt, &lastSeen, &u.DifficultyMin, &u.DifficultyMax); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	u.CreatedAt = time.Unix(createdAt, 0)
	if lastSeen.Valid {
		u.LastSeenAt = sql.NullTime{Time: time.Unix(lastSeen.Int64, 0), Valid: true}
	}
	return &u, nil
}

// CreateUser inserts a new user. Returns ErrUserExists if the name is taken.
var ErrUserExists = errors.New("user already exists")

func CreateUser(ctx context.Context, db *sql.DB, name, token string) (*User, error) {
	if name == "" {
		return nil, errors.New("name is required")
	}
	if token == "" {
		return nil, errors.New("token is required")
	}
	// difficulty_min/max take their DEFAULT values (3 and 6) from migration
	// 0006 — no need to mention them in the column list.
	res, err := db.ExecContext(ctx,
		`INSERT INTO users (name, token, created_at) VALUES (?, ?, unixepoch())`,
		name, token)
	if err != nil {
		// modernc.org/sqlite surfaces the constraint name in the error
		// message; cheap substring check is good enough for our purpose.
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: name %q", ErrUserExists, name)
		}
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &User{
		ID:            id,
		Name:          name,
		Token:         token,
		CreatedAt:     time.Now(),
		DifficultyMin: 3,
		DifficultyMax: 6,
	}, nil
}

// ListUsers returns all users ordered by id.
func ListUsers(ctx context.Context, db *sql.DB) ([]User, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT id, name, token, created_at, last_seen_at, difficulty_min, difficulty_max
		 FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		var createdAt int64
		var lastSeen sql.NullInt64
		if err := rows.Scan(&u.ID, &u.Name, &u.Token, &createdAt, &lastSeen, &u.DifficultyMin, &u.DifficultyMax); err != nil {
			return nil, err
		}
		u.CreatedAt = time.Unix(createdAt, 0)
		if lastSeen.Valid {
			u.LastSeenAt = sql.NullTime{Time: time.Unix(lastSeen.Int64, 0), Valid: true}
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// ErrInvalidPrefs is returned by UpdateUserPrefs when the difficulty range
// is malformed (out of range or inverted). Handlers should map this to 400.
var ErrInvalidPrefs = errors.New("invalid difficulty range")

// UpdateUserPrefs sets the user's difficulty preference range. Validates
// 1 ≤ dMin ≤ dMax ≤ 6 and refuses inverted ranges. Returns ErrUserNotFound
// when no row matched (defensive — shouldn't happen for an authed handler).
func UpdateUserPrefs(ctx context.Context, db *sql.DB, userID int64, dMin, dMax int) error {
	if dMin < DifficultyFloor || dMax > DifficultyCeiling || dMin > dMax {
		return fmt.Errorf("%w: difficulty_min=%d difficulty_max=%d (need %d ≤ min ≤ max ≤ %d)",
			ErrInvalidPrefs, dMin, dMax, DifficultyFloor, DifficultyCeiling)
	}
	res, err := db.ExecContext(ctx,
		`UPDATE users SET difficulty_min = ?, difficulty_max = ? WHERE id = ?`,
		dMin, dMax, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrUserNotFound
	}
	return nil
}

// DeleteUserByName removes the user and cascades to all their per-user rows.
//
// The cascade is done explicitly here rather than via FK ON DELETE CASCADE
// because migration 0004 couldn't declare FKs on the ADD COLUMN statements
// (SQLite forbids REFERENCES on ADD COLUMN with non-NULL DEFAULT). srs_state
// is the one exception — it was rebuilt and does have proper FK cascade —
// but for symmetry the explicit DELETE here covers it too (the row is just
// gone twice, which is a no-op).
//
// Returns ErrUserNotFound when no users row matched.
func DeleteUserByName(ctx context.Context, db *sql.DB, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var id int64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM users WHERE name = ?`, name).Scan(&id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrUserNotFound
		}
		return err
	}

	// Order matters only when FKs are enforced; with foreign_keys=ON the
	// srs_state row would block on the users row if we tried to delete the
	// user first. Wipe per-user rows first to keep things uniform.
	for _, stmt := range []string{
		`DELETE FROM srs_state WHERE user_id = ?`,
		`DELETE FROM sessions  WHERE user_id = ?`,
		`DELETE FROM attempts  WHERE user_id = ?`,
		`DELETE FROM push_subs WHERE user_id = ?`,
		`DELETE FROM users     WHERE id      = ?`,
	} {
		if _, err := tx.ExecContext(ctx, stmt, id); err != nil {
			return fmt.Errorf("cascade delete (%s): %w", stmt, err)
		}
	}
	return tx.Commit()
}

// TouchLastSeen best-effort updates the user's last_seen_at. Called from the
// auth middleware; errors are non-fatal so callers should log+ignore.
func TouchLastSeen(ctx context.Context, db *sql.DB, userID int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE users SET last_seen_at = unixepoch() WHERE id = ?`, userID)
	return err
}

// isUniqueViolation is a loose check that handles both modernc.org/sqlite
// and mattn/go-sqlite3 error strings — neither exposes a typed error.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return contains(s, "UNIQUE constraint failed") || contains(s, "constraint failed: UNIQUE")
}

func contains(haystack, needle string) bool {
	if len(needle) == 0 {
		return true
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
