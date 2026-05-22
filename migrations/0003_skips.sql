-- Adds a flag on attempts so a "skip / defer for later" can be recorded
-- without polluting session stats or SRS. Skip rows are written with
-- graded_by='skip', grade=0, skipped=1; aggregation queries filter on
-- skipped=0 to exclude them.
ALTER TABLE attempts ADD COLUMN skipped INTEGER NOT NULL DEFAULT 0;
CREATE INDEX idx_attempts_skipped ON attempts(skipped);
