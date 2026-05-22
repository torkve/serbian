CREATE TABLE tasks (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    kind          TEXT NOT NULL,
    difficulty    INTEGER NOT NULL CHECK (difficulty BETWEEN 1 AND 5),
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
CREATE INDEX idx_tasks_kind_diff ON tasks(kind, difficulty);
CREATE INDEX idx_tasks_topic ON tasks(topic);
CREATE INDEX idx_tasks_source ON tasks(source);

CREATE TABLE srs_state (
    task_id       INTEGER PRIMARY KEY REFERENCES tasks(id) ON DELETE CASCADE,
    ef            REAL NOT NULL DEFAULT 2.5,
    interval_days REAL NOT NULL DEFAULT 0,
    reps          INTEGER NOT NULL DEFAULT 0,
    lapses        INTEGER NOT NULL DEFAULT 0,
    due_at        INTEGER NOT NULL,
    last_grade    INTEGER,
    last_seen_at  INTEGER
);
CREATE INDEX idx_srs_due ON srs_state(due_at);

CREATE TABLE sessions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    started_at  INTEGER NOT NULL,
    ended_at    INTEGER,
    composition TEXT NOT NULL
);

CREATE TABLE attempts (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id     INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    session_id  INTEGER NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    user_answer TEXT,
    graded_by   TEXT NOT NULL,
    grade       INTEGER NOT NULL,
    feedback    TEXT,
    duration_ms INTEGER,
    created_at  INTEGER NOT NULL
);
CREATE INDEX idx_attempts_task ON attempts(task_id);
CREATE INDEX idx_attempts_session ON attempts(session_id);

CREATE TABLE push_subs (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    endpoint   TEXT NOT NULL UNIQUE,
    p256dh     TEXT NOT NULL,
    auth       TEXT NOT NULL,
    ua         TEXT,
    created_at INTEGER NOT NULL,
    last_ok_at INTEGER,
    failures   INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE claude_calls (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    day        TEXT NOT NULL,
    purpose    TEXT NOT NULL,
    model      TEXT NOT NULL,
    input_tok  INTEGER NOT NULL DEFAULT 0,
    output_tok INTEGER NOT NULL DEFAULT 0,
    ok         INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);
CREATE INDEX idx_claude_calls_day ON claude_calls(day);

CREATE TRIGGER tasks_init_srs AFTER INSERT ON tasks
BEGIN
    INSERT INTO srs_state (task_id, due_at) VALUES (NEW.id, unixepoch());
END;
