-- Per-user difficulty preference. Each user gets a min/max range that the
-- session composer filters tasks by. Default range 3..6 covers C1-low
-- through C1+/C2-low — the bands an exam-prep user typically wants.
-- Validation (1 ≤ min ≤ max ≤ 6) lives in app code where the error
-- messages can be informative; a CHECK constraint here would just produce
-- a generic SQLite error.

ALTER TABLE users ADD COLUMN difficulty_min INTEGER NOT NULL DEFAULT 3;
ALTER TABLE users ADD COLUMN difficulty_max INTEGER NOT NULL DEFAULT 6;
