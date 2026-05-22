package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// LoadExpected returns the raw expected_json and rationale for a task. Task
// content is shared across users, so this stays user-agnostic.
func LoadExpected(ctx context.Context, db *sql.DB, taskID int64) (kind string, expected []byte, rationale string, err error) {
	row := db.QueryRowContext(ctx,
		`SELECT kind, expected_json, COALESCE(rationale,'') FROM tasks WHERE id = ?`, taskID)
	err = row.Scan(&kind, &expected, &rationale)
	return
}

// LoadFull returns kind, the user-facing prompt, expected_json and rationale.
// Used by the live Claude grader which needs to show the original prompt.
// Task content is shared, so this stays user-agnostic.
func LoadFull(ctx context.Context, db *sql.DB, taskID int64) (kind, prompt string, expected []byte, rationale string, err error) {
	row := db.QueryRowContext(ctx,
		`SELECT kind, prompt, expected_json, COALESCE(rationale,'') FROM tasks WHERE id = ?`, taskID)
	err = row.Scan(&kind, &prompt, &expected, &rationale)
	return
}

// LoadSRS reads the SRS state for (user, task), returning the zero value
// (with sensible defaults) if no row exists yet — a fresh task this user
// hasn't seen.
func LoadSRS(ctx context.Context, db *sql.DB, userID, taskID int64) (SRSState, error) {
	var s SRSState
	var due, last sql.NullInt64
	var grade sql.NullInt64
	row := db.QueryRowContext(ctx,
		`SELECT ef, interval_days, reps, lapses, due_at, last_grade, last_seen_at
		 FROM srs_state WHERE user_id = ? AND task_id = ?`, userID, taskID)
	if err := row.Scan(&s.EF, &s.IntervalDays, &s.Reps, &s.Lapses, &due, &grade, &last); err != nil {
		if err == sql.ErrNoRows {
			return SRSState{EF: 2.5}, nil
		}
		return s, err
	}
	if due.Valid {
		s.DueAt = time.Unix(due.Int64, 0)
	}
	if last.Valid {
		s.LastSeenAt = time.Unix(last.Int64, 0)
	}
	if grade.Valid {
		s.LastGrade = int(grade.Int64)
	}
	return s, nil
}

// SaveSRS upserts the SRS row keyed on (user_id, task_id).
func SaveSRS(ctx context.Context, db *sql.DB, userID, taskID int64, s SRSState) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO srs_state (user_id, task_id, ef, interval_days, reps, lapses, due_at, last_grade, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, task_id) DO UPDATE SET
		    ef            = excluded.ef,
		    interval_days = excluded.interval_days,
		    reps          = excluded.reps,
		    lapses        = excluded.lapses,
		    due_at        = excluded.due_at,
		    last_grade    = excluded.last_grade,
		    last_seen_at  = excluded.last_seen_at
	`, userID, taskID, s.EF, s.IntervalDays, s.Reps, s.Lapses,
		s.DueAt.Unix(), s.LastGrade, s.LastSeenAt.Unix())
	return err
}

// StartSession inserts a new session row for the given user, returning its id.
func StartSession(ctx context.Context, db *sql.DB, userID int64, composition map[string]int) (int64, error) {
	body, err := json.Marshal(composition)
	if err != nil {
		return 0, err
	}
	res, err := db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, started_at, composition) VALUES (?, unixepoch(), ?)`,
		userID, string(body))
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// EndSession marks ended_at = now. Scoped to user_id so a request authed as
// alice can't close bob's session.
func EndSession(ctx context.Context, db *sql.DB, userID, sessionID int64) error {
	_, err := db.ExecContext(ctx,
		`UPDATE sessions SET ended_at = unixepoch()
		 WHERE id = ? AND user_id = ? AND ended_at IS NULL`,
		sessionID, userID)
	return err
}

// LogAttempt writes an attempts row scoped to the user.
func LogAttempt(ctx context.Context, db *sql.DB, userID, sessionID, taskID int64,
	userAnswer, gradedBy, feedback string, grade int, durMS int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO attempts (user_id, session_id, task_id, user_answer, graded_by, grade, feedback, duration_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
	`, userID, sessionID, taskID, userAnswer, gradedBy, grade,
		nullIfEmpty(feedback), durMS)
	return err
}

// LogSkip writes an attempts row marked skipped=1. SRS state is intentionally
// not updated by this function; use DeferTask separately to bump due_at.
func LogSkip(ctx context.Context, db *sql.DB, userID, sessionID, taskID int64, durMS int) error {
	_, err := db.ExecContext(ctx, `
		INSERT INTO attempts (user_id, session_id, task_id, user_answer, graded_by, grade, feedback, duration_ms, skipped, created_at)
		VALUES (?, ?, ?, NULL, 'skip', 0, NULL, ?, 1, unixepoch())
	`, userID, sessionID, taskID, durMS)
	return err
}

// DeferTask pushes a (user, task) SRS row's due_at forward by `by` without
// touching ef / interval_days / reps / lapses. Used when the user skips a
// task ("not now, ask me later"). If no SRS row exists yet, one is created
// with default SM-2 values and the deferred due date.
func DeferTask(ctx context.Context, db *sql.DB, userID, taskID int64, by time.Duration) error {
	due := time.Now().Add(by).Unix()
	_, err := db.ExecContext(ctx, `
		INSERT INTO srs_state (user_id, task_id, ef, interval_days, reps, lapses, due_at)
		VALUES (?, ?, 2.5, 0, 0, 0, ?)
		ON CONFLICT(user_id, task_id) DO UPDATE SET due_at = excluded.due_at
	`, userID, taskID, due)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// SessionExists returns true if the session belongs to the given user and is
// still open.
func SessionExists(ctx context.Context, db *sql.DB, userID, sessionID int64) (bool, error) {
	var n int
	err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE id = ? AND user_id = ? AND ended_at IS NULL`,
		sessionID, userID).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("session lookup: %w", err)
	}
	return n > 0, nil
}
