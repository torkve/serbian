-- Raise the difficulty ceiling again: relax CHECK from 1..6 to 1..8 so the
-- task bank can hold genuine C2 content (d7 = true C2 academic/literary,
-- d8 = C2+ mastery / archaic-literary). Mirrors migration 0005 exactly —
-- only the upper CHECK bound differs.
--
-- SQLite still can't ALTER an existing CHECK constraint, so the rebuild
-- dance is the only way: create a parallel table with the new CHECK, copy
-- the rows, drop the old table, rename. PRAGMA defer_foreign_keys = ON
-- lets the DROP + RENAME execute without per-statement FK validation
-- tripping on srs_state.task_id / attempts.task_id mid-transaction.

PRAGMA defer_foreign_keys = ON;

CREATE TABLE tasks_new (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT NOT NULL,
    difficulty    INTEGER NOT NULL CHECK (difficulty BETWEEN 1 AND 8),
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
