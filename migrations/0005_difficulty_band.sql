-- Raise the difficulty ceiling: relax CHECK from 1..5 to 1..6 so the task
-- bank has headroom for genuinely C1+ content (literary / journalistic /
-- academic register). The bulk recalibration lives in the subagent prompt
-- (.claude/agents/serbian-task-author.md); this migration only widens the
-- column's allowed range.
--
-- SQLite can't ALTER an existing CHECK constraint, so the standard rebuild
-- dance is the only way: create a parallel table with the new CHECK, copy
-- the rows over, drop the old table, rename. The tasks PK is unchanged
-- (still INTEGER PRIMARY KEY AUTOINCREMENT), so all FKs pointing at it
-- (srs_state.task_id, attempts.task_id) keep resolving after the rename;
-- PRAGMA defer_foreign_keys = ON lets the DROP + RENAME execute without
-- per-statement FK validation tripping on those references mid-transaction.

PRAGMA defer_foreign_keys = ON;

CREATE TABLE tasks_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT NOT NULL,
    difficulty    INTEGER NOT NULL CHECK (difficulty BETWEEN 1 AND 6),
    topic         TEXT,
    prompt        TEXT NOT NULL,
    payload_json  TEXT NOT NULL,
    expected_json TEXT NOT NULL,
    rationale     TEXT,
    source        TEXT NOT NULL,
    content_hash  TEXT NOT NULL UNIQUE,
    flagged       INTEGER NOT NULL DEFAULT 0,
    created_at    INTEGER NOT NULL
);

INSERT INTO tasks_new
    SELECT id, kind, difficulty, topic, prompt, payload_json, expected_json,
           rationale, source, content_hash, flagged, created_at
    FROM tasks;

DROP TABLE tasks;
ALTER TABLE tasks_new RENAME TO tasks;

CREATE INDEX idx_tasks_kind_diff ON tasks(kind, difficulty);
CREATE INDEX idx_tasks_topic     ON tasks(topic);
CREATE INDEX idx_tasks_source    ON tasks(source);
