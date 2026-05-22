-- Multi-user support. Per-user tables (srs_state, sessions, attempts,
-- push_subs) gain a user_id column. Task content (tasks, claude_calls,
-- settings) stays shared across users.
--
-- SQLite limitation: `ALTER TABLE … ADD COLUMN … NOT NULL DEFAULT … REFERENCES …`
-- is rejected ("Cannot add a REFERENCES column with non-NULL default value"),
-- so the user_id columns below intentionally omit the FK constraint. The
-- delete-cascade is enforced in Go (internal/store/users.go DeleteUserByName)
-- via explicit per-table DELETE statements inside a transaction.
--
-- The single owner from the pre-multi-user era becomes user #1 — the Go
-- bootstrap (internal/store/bootstrap.go) handles the INSERT INTO users +
-- backfill of srs_state_old immediately after this migration applies.

CREATE TABLE users (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL UNIQUE,
    token         TEXT NOT NULL UNIQUE,
    created_at    INTEGER NOT NULL,
    last_seen_at  INTEGER
);

-- The trigger created a single srs_state row per task. With multi-user that
-- would mean "for which user?" — we replace it with lazy creation at session-
-- compose time (LEFT JOIN in scheduler.go), so unseen tasks for any user are
-- treated as due immediately and SaveSRS does the insert on first grade.
DROP TRIGGER IF EXISTS tasks_init_srs;

-- srs_state primary key changes from (task_id) to (user_id, task_id). SQLite
-- can't ALTER PRIMARY KEY in place, so we rename + recreate. The Go-side
-- bootstrap backfills srs_state from srs_state_old once user #1 exists, then
-- drops srs_state_old. Since this is a fresh CREATE TABLE we *can* keep the
-- proper REFERENCES + ON DELETE CASCADE here.
ALTER TABLE srs_state RENAME TO srs_state_old;
CREATE TABLE srs_state (
    user_id       INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id       INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    ef            REAL NOT NULL DEFAULT 2.5,
    interval_days REAL NOT NULL DEFAULT 0,
    reps          INTEGER NOT NULL DEFAULT 0,
    lapses        INTEGER NOT NULL DEFAULT 0,
    due_at        INTEGER NOT NULL,
    last_grade    INTEGER,
    last_seen_at  INTEGER,
    PRIMARY KEY (user_id, task_id)
);
CREATE INDEX idx_srs_user_due ON srs_state(user_id, due_at);

-- Plain ADD COLUMN — DEFAULT 1 backfills existing rows to user #1 (which the
-- Go bootstrap creates immediately after this migration). No REFERENCES due
-- to the SQLite restriction noted above; cascade handled in Go.
ALTER TABLE sessions  ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE attempts  ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE push_subs ADD COLUMN user_id INTEGER NOT NULL DEFAULT 1;

CREATE INDEX idx_sessions_user  ON sessions(user_id);
CREATE INDEX idx_attempts_user  ON attempts(user_id);
CREATE INDEX idx_push_subs_user ON push_subs(user_id);
