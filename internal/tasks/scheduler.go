package tasks

import (
	"context"
	"database/sql"
	"encoding/json"
	"math/rand/v2"
	"strings"
)

type Mix struct {
	Grammar     int
	Translation int
	Speaking    int
}

var DefaultMix = Mix{Grammar: 2, Translation: 2, Speaking: 1}

const DefaultBudgetSeconds = 130

type Composer struct {
	db *sql.DB
}

func NewComposer(db *sql.DB) *Composer { return &Composer{db: db} }

// Compose builds a per-user session: tasks the user hasn't seen yet (no
// srs_state row) or whose due_at has elapsed, balanced by modality and
// capped by est-seconds budget. dMin/dMax bound the difficulty band the
// user has selected in settings — tasks outside this band are never
// surfaced.
func (c *Composer) Compose(ctx context.Context, userID int64, mix Mix, budgetSec, dMin, dMax int) ([]Task, error) {
	grammar, err := c.dueByModality(ctx, userID, ModalityGrammar, mix.Grammar*4, dMin, dMax)
	if err != nil {
		return nil, err
	}
	translation, err := c.dueByModality(ctx, userID, ModalityTranslation, mix.Translation*4, dMin, dMax)
	if err != nil {
		return nil, err
	}
	speaking, err := c.dueByModality(ctx, userID, ModalitySpeaking, mix.Speaking*4, dMin, dMax)
	if err != nil {
		return nil, err
	}

	wanted := mix.Grammar + mix.Translation + mix.Speaking
	var selected []Task
	selected = append(selected, takeFirst(grammar, mix.Grammar)...)
	selected = append(selected, takeFirst(translation, mix.Translation)...)
	selected = append(selected, takeFirst(speaking, mix.Speaking)...)

	if len(selected) < wanted {
		more, err := c.dueAny(ctx, userID, 8, dMin, dMax)
		if err != nil {
			return nil, err
		}
		seen := map[int64]bool{}
		for _, t := range selected {
			seen[t.ID] = true
		}
		for _, t := range more {
			if seen[t.ID] {
				continue
			}
			selected = append(selected, t)
			seen[t.ID] = true
			if len(selected) >= wanted {
				break
			}
		}
	}

	// Enforce time budget. Always keep at least one task.
	sum := 0
	out := make([]Task, 0, len(selected))
	for _, t := range selected {
		s := EstSeconds(t.Kind)
		t.EstSec = s
		if len(out) > 0 && sum+s > budgetSec {
			break
		}
		out = append(out, t)
		sum += s
	}

	rand.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out, nil
}

func takeFirst(ts []Task, n int) []Task {
	if len(ts) <= n {
		return ts
	}
	return ts[:n]
}

// dueByModality returns at most `limit` tasks of any kind in the given
// modality that are due for this user. "Due" means: either no srs_state row
// exists (unseen → treated as due_at = 0) or due_at <= now. The difficulty
// band [dMin,dMax] further restricts what the user sees.
func (c *Composer) dueByModality(ctx context.Context, userID int64, mod Modality, limit, dMin, dMax int) ([]Task, error) {
	kinds := KindsByModality(mod)
	if len(kinds) == 0 {
		return nil, nil
	}
	ph := strings.TrimRight(strings.Repeat("?,", len(kinds)), ",")
	args := make([]any, 0, len(kinds)+4)
	args = append(args, userID)
	for _, k := range kinds {
		args = append(args, k)
	}
	args = append(args, dMin, dMax, limit)
	q := `SELECT t.id, t.kind, t.difficulty, COALESCE(t.topic,''), t.prompt, t.payload_json
	      FROM tasks t
	      LEFT JOIN srs_state s ON s.task_id = t.id AND s.user_id = ?
	      WHERE t.flagged = 0 AND t.kind IN (` + ph + `)
	        AND t.difficulty BETWEEN ? AND ?
	        AND (s.due_at IS NULL OR s.due_at <= unixepoch())
	      ORDER BY COALESCE(s.due_at, 0) ASC, RANDOM()
	      LIMIT ?`
	return scanTasks(ctx, c.db, q, args...)
}

// dueAny is the modality-agnostic top-up used when a modality has too few
// due items to fill the quota. Honors the same difficulty band.
func (c *Composer) dueAny(ctx context.Context, userID int64, limit, dMin, dMax int) ([]Task, error) {
	q := `SELECT t.id, t.kind, t.difficulty, COALESCE(t.topic,''), t.prompt, t.payload_json
	      FROM tasks t
	      LEFT JOIN srs_state s ON s.task_id = t.id AND s.user_id = ?
	      WHERE t.flagged = 0
	        AND t.difficulty BETWEEN ? AND ?
	        AND (s.due_at IS NULL OR s.due_at <= unixepoch())
	      ORDER BY COALESCE(s.due_at, 0) ASC, RANDOM()
	      LIMIT ?`
	return scanTasks(ctx, c.db, q, userID, dMin, dMax, limit)
}

func scanTasks(ctx context.Context, db *sql.DB, q string, args ...any) ([]Task, error) {
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		var payload []byte
		if err := rows.Scan(&t.ID, &t.Kind, &t.Difficulty, &t.Topic, &t.Prompt, &payload); err != nil {
			return nil, err
		}
		t.Payload = json.RawMessage(payload)
		out = append(out, t)
	}
	return out, rows.Err()
}
