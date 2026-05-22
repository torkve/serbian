package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// EnsureOwnerUser is the idempotent first-run bootstrap that:
//  1. Creates user #1 ("owner") with `ownerToken` if no users exist yet.
//  2. Backfills the new srs_state table from srs_state_old (left behind by
//     migration 0004) under user_id = 1, then drops srs_state_old.
//
// Subsequent calls are no-ops. Returns the owner's user ID (always 1 in
// practice, but read from the DB to be defensive).
//
// `ownerToken` should be the legacy cfg.AuthToken — that's what the owner's
// existing cookie carries, so reusing it keeps their session alive across
// the migration.
func EnsureOwnerUser(ctx context.Context, db *sql.DB, ownerToken string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	// Defer FK validation to commit time so the cross-statement INSERTs
	// below (users → srs_state) work even though the new owner row isn't
	// "visible" to per-statement FK lookups in this SQLite driver.
	if _, err := tx.ExecContext(ctx, `PRAGMA defer_foreign_keys = ON`); err != nil {
		return 0, fmt.Errorf("defer fk: %w", err)
	}

	// Fast path: any existing user → bootstrap already happened.
	var existingID sql.NullInt64
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&existingID); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("check existing user: %w", err)
	}
	if existingID.Valid {
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return existingID.Int64, nil
	}

	if ownerToken == "" {
		return 0, errors.New("ensure owner: ownerToken is empty (cfg.AuthToken not set)")
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (name, token, created_at) VALUES ('owner', ?, unixepoch())`,
		ownerToken)
	if err != nil {
		return 0, fmt.Errorf("insert owner: %w", err)
	}
	ownerID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Backfill srs_state from the migrated-away srs_state_old, if present.
	// Migration 0004 leaves srs_state_old behind precisely so this transaction
	// can attribute its rows to the owner.
	var hasOld bool
	if err := tx.QueryRowContext(ctx,
		`SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'srs_state_old'`,
	).Scan(new(int)); err == nil {
		hasOld = true
	} else if !errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("probe srs_state_old: %w", err)
	}
	if hasOld {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO srs_state (user_id, task_id, ef, interval_days, reps, lapses, due_at, last_grade, last_seen_at)
			SELECT ?, task_id, ef, interval_days, reps, lapses, due_at, last_grade, last_seen_at
			FROM srs_state_old
		`, ownerID); err != nil {
			return 0, fmt.Errorf("backfill srs_state: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `DROP TABLE srs_state_old`); err != nil {
			return 0, fmt.Errorf("drop srs_state_old: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return ownerID, nil
}
