// Command annotate is a small back-fill helper for the task bank. It has
// two modes:
//
//	-mode dump  → reads all unflagged tasks from the DB and writes per-kind
//	              batched JSON files for the serbian-task-annotator sub-agent
//	              to consume. Each batch holds at most -batch-size rows.
//	-mode merge → reads sub-agent output files and applies their
//	              { topic, critical, forbidden } annotations back to the DB.
//	              Idempotent. Sanity-checks every critical/forbidden against
//	              the row's existing answers (same normalization the grader
//	              uses) and skips rows that would self-defeat the gate.
//
// Both modes share the standard -config flag and use the embedded
// migrations (in case a fresh DB needs them).
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"serbian"
	"serbian/internal/config"
	"serbian/internal/store"
	"serbian/internal/tasks"
)

func main() {
	var (
		mode       string
		configPath string
		batchSize  int
		outDir     string
		inDir      string
		dryRun     bool
	)
	flag.StringVar(&mode, "mode", "", "dump | merge")
	flag.StringVar(&configPath, "config", "data/config.json", "config file path")
	flag.IntVar(&batchSize, "batch-size", 50, "max rows per dump file")
	flag.StringVar(&outDir, "out", ".local/annotate/in", "(dump) output directory for batch input files")
	flag.StringVar(&inDir, "in", ".local/annotate/out", "(merge) input directory of annotator output files")
	flag.BoolVar(&dryRun, "dry-run", false, "(merge) report would-update counts without writing")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	db, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()
	migrations, _ := fs.Sub(serbian.MigrationsFS, "migrations")
	if err := store.Migrate(ctx, db, migrations); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	switch mode {
	case "dump":
		if err := runDump(ctx, db, outDir, batchSize); err != nil {
			log.Fatalf("dump: %v", err)
		}
	case "merge":
		if err := runMerge(ctx, db, inDir, dryRun); err != nil {
			log.Fatalf("merge: %v", err)
		}
	default:
		fmt.Fprintln(os.Stderr, "usage: annotate -mode dump|merge [flags]")
		flag.Usage()
		os.Exit(2)
	}
}

// batchItem is the JSON shape sent to the sub-agent. The agent uses
// `needs_topic` to decide whether to mint a new topic. `expected_json` is
// expanded into `answers` / `must_contain` (so the agent doesn't have to
// re-parse JSON nested in a string).
type batchItem struct {
	ID          int64           `json:"id"`
	Kind        string          `json:"kind"`
	NeedsTopic  bool            `json:"needs_topic"`
	Prompt      string          `json:"prompt"`
	Payload     json.RawMessage `json:"payload"`
	Answers     []string        `json:"answers,omitempty"`
	MustContain []string        `json:"must_contain,omitempty"`
}

// annotation is the JSON shape the sub-agent writes back.
type annotation struct {
	ID        int64    `json:"id"`
	Topic     *string  `json:"topic,omitempty"`
	Critical  []string `json:"critical,omitempty"`
	Forbidden []string `json:"forbidden,omitempty"`
}

func runDump(ctx context.Context, db *sql.DB, outDir string, batchSize int) error {
	if batchSize < 1 || batchSize > 200 {
		return fmt.Errorf("batch-size must be 1..200")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, kind, COALESCE(topic, ''), prompt, payload_json, expected_json
		FROM tasks
		WHERE flagged = 0
		ORDER BY kind, id`)
	if err != nil {
		return fmt.Errorf("query tasks: %w", err)
	}
	defer rows.Close()

	// Group by kind so each batch is homogeneous — keeps the agent's prompt
	// focused on the rules for that kind.
	byKind := map[string][]batchItem{}
	total := 0
	for rows.Next() {
		var (
			id          int64
			kind        string
			topic       string
			prompt      string
			payload     []byte
			expectedRaw []byte
		)
		if err := rows.Scan(&id, &kind, &topic, &prompt, &payload, &expectedRaw); err != nil {
			return err
		}
		var exp tasks.Expected
		if err := json.Unmarshal(expectedRaw, &exp); err != nil {
			// Don't fail the whole dump — skip the row, surface a warning.
			log.Printf("warn: parse expected for id=%d: %v", id, err)
			continue
		}
		item := batchItem{
			ID:          id,
			Kind:        kind,
			NeedsTopic:  strings.TrimSpace(topic) == "",
			Prompt:      prompt,
			Payload:     json.RawMessage(payload),
			Answers:     exp.Answers,
			MustContain: exp.MustContain,
		}
		byKind[kind] = append(byKind[kind], item)
		total++
	}
	if err := rows.Err(); err != nil {
		return err
	}

	written := 0
	for _, kind := range sortedKeys(byKind) {
		items := byKind[kind]
		for i := 0; i < len(items); i += batchSize {
			end := i + batchSize
			if end > len(items) {
				end = len(items)
			}
			batch := items[i:end]
			path := filepath.Join(outDir, fmt.Sprintf("%s-%03d.json", kind, i/batchSize+1))
			b, err := json.MarshalIndent(batch, "", "  ")
			if err != nil {
				return fmt.Errorf("marshal %s: %w", path, err)
			}
			if err := os.WriteFile(path, b, 0o644); err != nil {
				return fmt.Errorf("write %s: %w", path, err)
			}
			written++
		}
	}
	log.Printf("dumped %d rows across %d batch files into %s", total, written, outDir)
	return nil
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func runMerge(ctx context.Context, db *sql.DB, inDir string, dryRun bool) error {
	entries, err := os.ReadDir(inDir)
	if err != nil {
		return fmt.Errorf("read %s: %w", inDir, err)
	}
	var stats struct {
		filesRead     int
		annotationsIn int
		topicApplied  int
		topicSkipped  int
		critApplied   int
		critSkipped   int
		forbidApplied int
		forbidSkipped int
		idNotFound    int
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(inDir, e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			log.Printf("warn: read %s: %v", path, err)
			continue
		}
		var anns []annotation
		if err := json.Unmarshal(body, &anns); err != nil {
			log.Printf("warn: parse %s: %v", path, err)
			continue
		}
		stats.filesRead++
		stats.annotationsIn += len(anns)

		// Single transaction per file. Skipped rows do not abort the rest.
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx: %w", err)
		}

		for _, a := range anns {
			if a.ID == 0 {
				continue
			}

			// Load the row's current state (topic + existing answers).
			var curTopic sql.NullString
			var expectedRaw []byte
			err := tx.QueryRowContext(ctx,
				`SELECT topic, expected_json FROM tasks WHERE id = ?`, a.ID,
			).Scan(&curTopic, &expectedRaw)
			if errors.Is(err, sql.ErrNoRows) {
				stats.idNotFound++
				continue
			}
			if err != nil {
				log.Printf("warn: row %d: %v", a.ID, err)
				continue
			}
			var exp tasks.Expected
			if err := json.Unmarshal(expectedRaw, &exp); err != nil {
				log.Printf("warn: row %d: parse expected: %v", a.ID, err)
				continue
			}

			// --- topic ---
			if a.Topic != nil && *a.Topic != "" {
				cur := ""
				if curTopic.Valid {
					cur = strings.TrimSpace(curTopic.String)
				}
				if cur == "" {
					if !dryRun {
						if _, err := tx.ExecContext(ctx,
							`UPDATE tasks SET topic = ? WHERE id = ? AND (topic IS NULL OR topic = '')`,
							*a.Topic, a.ID,
						); err != nil {
							log.Printf("warn: row %d: topic update: %v", a.ID, err)
							continue
						}
					}
					stats.topicApplied++
				} else {
					stats.topicSkipped++ // already set
				}
			}

			// --- critical ---
			if len(a.Critical) > 0 {
				clean, dropped := filterCritical(a.Critical, exp.Answers)
				if len(dropped) > 0 {
					log.Printf("warn: row %d: critical entries not in any answer dropped: %v", a.ID, dropped)
				}
				if len(clean) > 0 {
					critJSON, err := json.Marshal(clean)
					if err == nil {
						if !dryRun {
							if _, err := tx.ExecContext(ctx,
								`UPDATE tasks SET expected_json = json_set(expected_json, '$.critical', json(?)) WHERE id = ?`,
								string(critJSON), a.ID,
							); err != nil {
								log.Printf("warn: row %d: critical update: %v", a.ID, err)
								continue
							}
						}
						stats.critApplied++
					}
				} else {
					stats.critSkipped++
				}
			}

			// --- forbidden ---
			if len(a.Forbidden) > 0 {
				clean, dropped := filterForbidden(a.Forbidden, exp.Answers)
				if len(dropped) > 0 {
					log.Printf("warn: row %d: forbidden entries overlapping an answer dropped: %v", a.ID, dropped)
				}
				if len(clean) > 0 {
					forbJSON, err := json.Marshal(clean)
					if err == nil {
						if !dryRun {
							if _, err := tx.ExecContext(ctx,
								`UPDATE tasks SET expected_json = json_set(expected_json, '$.forbidden', json(?)) WHERE id = ?`,
								string(forbJSON), a.ID,
							); err != nil {
								log.Printf("warn: row %d: forbidden update: %v", a.ID, err)
								continue
							}
						}
						stats.forbidApplied++
					}
				} else {
					stats.forbidSkipped++
				}
			}
		}

		if dryRun {
			_ = tx.Rollback()
		} else if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit %s: %w", path, err)
		}
	}

	prefix := ""
	if dryRun {
		prefix = "(dry-run) "
	}
	log.Printf("%smerge done: files=%d annotations=%d topic_applied=%d topic_skipped=%d critical_applied=%d critical_skipped=%d forbidden_applied=%d forbidden_skipped=%d not_found=%d",
		prefix,
		stats.filesRead, stats.annotationsIn,
		stats.topicApplied, stats.topicSkipped,
		stats.critApplied, stats.critSkipped,
		stats.forbidApplied, stats.forbidSkipped,
		stats.idNotFound,
	)
	return nil
}

// filterCritical keeps only critical substrings that appear (after
// normalization) in at least one of the task's answers. Returns the kept
// list (preserving original casing/spelling) and the dropped list (for
// logging).
func filterCritical(crit []string, answers []string) (keep, drop []string) {
	normAnswers := make([]string, 0, len(answers))
	for _, a := range answers {
		normAnswers = append(normAnswers, tasks.Normalize(a))
	}
	for _, c := range crit {
		nc := tasks.Normalize(c)
		if nc == "" {
			drop = append(drop, c)
			continue
		}
		found := false
		for _, na := range normAnswers {
			if strings.Contains(na, nc) {
				found = true
				break
			}
		}
		if found {
			keep = append(keep, c)
		} else {
			drop = append(drop, c)
		}
	}
	return keep, drop
}

// filterForbidden is the inverse: keep entries that don't overlap any
// answer; drop any that do.
func filterForbidden(forb []string, answers []string) (keep, drop []string) {
	normAnswers := make([]string, 0, len(answers))
	for _, a := range answers {
		normAnswers = append(normAnswers, tasks.Normalize(a))
	}
	for _, f := range forb {
		nf := tasks.Normalize(f)
		if nf == "" {
			drop = append(drop, f)
			continue
		}
		clash := false
		for _, na := range normAnswers {
			if strings.Contains(na, nf) {
				clash = true
				break
			}
		}
		if clash {
			drop = append(drop, f)
		} else {
			keep = append(keep, f)
		}
	}
	return keep, drop
}
