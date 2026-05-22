package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"
	"time"
	"unicode"

	"serbian"
	"serbian/internal/config"
	"serbian/internal/llm"
	"serbian/internal/store"
	"serbian/internal/tasks"
)

func main() {
	var (
		kind       string
		topic      string
		difficulty int
		count      int
		model      string
		dryRun     bool
		configPath string
		verbose    bool
		importPath string
	)
	flag.StringVar(&kind, "kind", "", "task kind (cloze|conjugation|case|aspect|tr_ru_sr|tr_sr_ru|vocab|speak)")
	flag.StringVar(&topic, "topic", "", "topic tag, e.g. cases.instrumental")
	flag.IntVar(&difficulty, "difficulty", 3, "difficulty 1-6 (1≈B2-low, 3≈C1-low, 5=C1-high, 6=C1+ литерарно)")
	flag.IntVar(&count, "count", 10, "number of tasks to generate")
	flag.StringVar(&model, "model", "", "anthropic model override (default: from config)")
	flag.BoolVar(&dryRun, "dry-run", false, "print generated tasks to stdout instead of inserting")
	flag.StringVar(&configPath, "config", "data/config.json", "config file path")
	flag.BoolVar(&verbose, "v", false, "verbose log output")
	flag.StringVar(&importPath, "import", "", "import tasks from a JSON file (skips Anthropic call; use for sub-agent output)")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if importPath != "" {
		runImport(ctx, importPath, configPath, dryRun)
		return
	}

	if kind == "" {
		fmt.Fprintln(os.Stderr, "--kind is required (or pass --import to load from a JSON file)")
		flag.Usage()
		os.Exit(2)
	}
	if _, ok := allowedKinds[kind]; !ok {
		fmt.Fprintf(os.Stderr, "unknown kind %q; allowed: cloze, conjugation, case, aspect, tr_ru_sr, tr_sr_ru, vocab, speak\n", kind)
		os.Exit(2)
	}
	if difficulty < 1 || difficulty > 6 {
		fmt.Fprintln(os.Stderr, "--difficulty must be 1..6")
		os.Exit(2)
	}
	if count < 1 || count > 50 {
		fmt.Fprintln(os.Stderr, "--count must be 1..50")
		os.Exit(2)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if cfg.AnthropicAPIKey == "" {
		fmt.Fprintln(os.Stderr, "no Anthropic API key configured. Set ANTHROPIC_API_KEY or add 'anthropic_api_key' to "+configPath)
		os.Exit(1)
	}
	usedModel := cfg.AnthropicModel
	if model != "" {
		usedModel = model
	}

	prompts, err := loadPrompts()
	if err != nil {
		log.Fatalf("load prompts: %v", err)
	}
	userPrompt, err := prompts.Render(kind, llm.PromptVars{
		Kind:       kind,
		Topic:      topic,
		Difficulty: difficulty,
		Count:      count,
	})
	if err != nil {
		log.Fatalf("render prompt: %v", err)
	}
	if verbose {
		log.Printf("model=%s prompt:\n%s", usedModel, userPrompt)
	}

	client := llm.NewClient(cfg.AnthropicAPIKey, usedModel)
	generated, usage, err := client.GenerateTasks(ctx, llm.SystemPrompt, userPrompt, count)
	if err != nil {
		log.Fatalf("generate: %v", err)
	}
	log.Printf("generated %d tasks (in=%d out=%d cache_read=%d cache_create=%d)",
		len(generated), usage.InputTokens, usage.OutputTokens, usage.CacheRead, usage.CacheCreate)

	valid, rejected := validate(generated, kind)
	for _, r := range rejected {
		log.Printf("reject: %s — %s", r.reason, abbreviate(r.task.Prompt, 60))
	}

	if dryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		if err := enc.Encode(valid); err != nil {
			log.Fatal(err)
		}
		return
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

	inserted, dupes, err := insertAll(ctx, db, valid, kind, difficulty, topic, "pregen:claude:"+usedModel)
	if err != nil {
		log.Fatalf("insert: %v", err)
	}
	if err := logCall(ctx, db, "pregen:"+kind, usedModel, usage); err != nil {
		log.Printf("warn: log claude_call: %v", err)
	}
	log.Printf("inserted=%d dupes_skipped=%d rejected=%d", inserted, dupes, len(rejected))
}

var allowedKinds = map[string]bool{
	tasks.KindCloze: true, tasks.KindConjugation: true, tasks.KindCase: true, tasks.KindAspect: true,
	tasks.KindTrRUSR: true, tasks.KindTrSRRU: true, tasks.KindVocab: true, tasks.KindSpeak: true,
}

func loadPrompts() (*llm.Prompts, error) {
	sub, err := fs.Sub(serbian.PromptsFS, "prompts")
	if err != nil {
		return nil, err
	}
	return llm.LoadPrompts(sub)
}

type rejection struct {
	task   llm.GeneratedTask
	reason string
}

// validate filters out items missing required fields, lacking Cyrillic, or
// with malformed payload/expected JSON.
func validate(items []llm.GeneratedTask, kind string) ([]llm.GeneratedTask, []rejection) {
	var ok []llm.GeneratedTask
	var bad []rejection
	for _, it := range items {
		if strings.TrimSpace(it.Prompt) == "" {
			bad = append(bad, rejection{it, "empty prompt"})
			continue
		}
		if !hasCyrillic(it.Prompt) {
			bad = append(bad, rejection{it, "no Cyrillic in prompt"})
			continue
		}
		if len(it.Payload) == 0 {
			bad = append(bad, rejection{it, "missing payload"})
			continue
		}
		if !isJSONObject(it.Payload) {
			bad = append(bad, rejection{it, "payload is not a JSON object"})
			continue
		}
		if len(it.Expected) == 0 {
			bad = append(bad, rejection{it, "missing expected"})
			continue
		}
		if !isJSONObject(it.Expected) {
			bad = append(bad, rejection{it, "expected is not a JSON object"})
			continue
		}
		// Check expected.answers exists and is non-empty array
		var exp tasks.Expected
		if err := json.Unmarshal(it.Expected, &exp); err != nil {
			bad = append(bad, rejection{it, "expected JSON unmarshal: " + err.Error()})
			continue
		}
		if len(exp.Answers) == 0 {
			bad = append(bad, rejection{it, "expected.answers is empty"})
			continue
		}
		ok = append(ok, it)
	}
	return ok, bad
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Cyrillic, r) {
			return true
		}
	}
	return false
}

func isJSONObject(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return strings.HasPrefix(t, "{") && strings.HasSuffix(t, "}")
}

func contentHash(kind, prompt string, payload json.RawMessage) string {
	h := sha256.New()
	h.Write([]byte(kind))
	h.Write([]byte{0})
	h.Write([]byte(prompt))
	h.Write([]byte{0})
	canonical, _ := canonicalJSON(payload)
	h.Write(canonical)
	return hex.EncodeToString(h.Sum(nil))[:32]
}

// canonicalJSON re-encodes with sorted keys for stable hashing.
func canonicalJSON(raw json.RawMessage) ([]byte, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return raw, err
	}
	return marshalCanonical(v)
}

func marshalCanonical(v any) ([]byte, error) {
	return json.Marshal(v) // json.Marshal already sorts map keys alphabetically
}

func insertAll(ctx context.Context, db *sql.DB, items []llm.GeneratedTask, kind string, difficulty int, topic, source string) (inserted, dupes int, err error) {
	stmt, err := db.PrepareContext(ctx, `
		INSERT INTO tasks (kind, difficulty, topic, prompt, payload_json, expected_json, rationale, source, content_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, unixepoch())
		ON CONFLICT(content_hash) DO NOTHING
	`)
	if err != nil {
		return 0, 0, err
	}
	defer stmt.Close()
	for _, it := range items {
		hash := contentHash(kind, it.Prompt, it.Payload)
		res, err := stmt.ExecContext(ctx, kind, difficulty, nullIfEmpty(topic),
			it.Prompt, string(it.Payload), string(it.Expected), nullIfEmpty(it.Rationale),
			source, hash)
		if err != nil {
			return inserted, dupes, fmt.Errorf("insert: %w", err)
		}
		n, _ := res.RowsAffected()
		if n == 0 {
			dupes++
		} else {
			inserted++
		}
	}
	return inserted, dupes, nil
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func logCall(ctx context.Context, db *sql.DB, purpose, model string, u llm.Usage) error {
	today := time.Now().UTC().Format("2006-01-02")
	_, err := db.ExecContext(ctx, `
		INSERT INTO claude_calls (day, purpose, model, input_tok, output_tok, ok, created_at)
		VALUES (?, ?, ?, ?, ?, 1, unixepoch())
	`, today, purpose, model, u.InputTokens+u.CacheRead+u.CacheCreate, u.OutputTokens)
	return err
}

func abbreviate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// importBatch matches what the serbian-task-author subagent writes.
type importBatch struct {
	Kind       string              `json:"kind"`
	Difficulty int                 `json:"difficulty"`
	Topic      string              `json:"topic"`
	Tasks      []llm.GeneratedTask `json:"tasks"`
}

func runImport(ctx context.Context, path, configPath string, dryRun bool) {
	body, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("read %s: %v", path, err)
	}
	var batch importBatch
	if err := json.Unmarshal(body, &batch); err != nil {
		log.Fatalf("parse %s: %v", path, err)
	}
	if !allowedKinds[batch.Kind] {
		log.Fatalf("import: unknown kind %q (allowed: cloze, conjugation, case, aspect, tr_ru_sr, tr_sr_ru, vocab, speak)", batch.Kind)
	}
	if batch.Difficulty < 1 || batch.Difficulty > 6 {
		log.Fatalf("import: difficulty must be 1..6 (got %d)", batch.Difficulty)
	}
	if len(batch.Tasks) == 0 {
		log.Fatalf("import: no tasks in batch")
	}

	valid, rejected := validate(batch.Tasks, batch.Kind)
	for _, r := range rejected {
		log.Printf("reject: %s — %s", r.reason, abbreviate(r.task.Prompt, 60))
	}

	if dryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		_ = enc.Encode(valid)
		return
	}

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

	inserted, dupes, err := insertAll(ctx, db, valid, batch.Kind, batch.Difficulty, batch.Topic, "subagent")
	if err != nil {
		log.Fatalf("insert: %v", err)
	}
	log.Printf("imported from %s: kind=%s diff=%d topic=%q inserted=%d dupes_skipped=%d rejected=%d",
		path, batch.Kind, batch.Difficulty, batch.Topic, inserted, dupes, len(rejected))
}
