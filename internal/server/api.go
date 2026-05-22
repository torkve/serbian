package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"serbian/internal/tasks"
)

type startResponse struct {
	SessionID int64          `json:"session_id"`
	Tasks     []tasks.Task   `json:"tasks"`
	Mix       map[string]int `json:"mix"`
}

func (s *Server) handleSessionStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := UserFromContext(ctx)
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	mix := tasks.DefaultMix
	composition := map[string]int{
		"grammar":     mix.Grammar,
		"translation": mix.Translation,
		"speaking":    mix.Speaking,
	}
	tlist, err := s.composer.Compose(ctx, u.ID, mix, tasks.DefaultBudgetSeconds, u.DifficultyMin, u.DifficultyMax)
	if err != nil {
		httpError(w, "compose", err)
		return
	}
	if len(tlist) == 0 {
		writeJSON(w, http.StatusOK, startResponse{SessionID: 0, Tasks: nil, Mix: composition})
		return
	}
	id, err := tasks.StartSession(ctx, s.db, u.ID, composition)
	if err != nil {
		httpError(w, "start session", err)
		return
	}
	writeJSON(w, http.StatusOK, startResponse{SessionID: id, Tasks: tlist, Mix: composition})
}

type attemptRequest struct {
	TaskID     int64  `json:"task_id"`
	Answer     string `json:"answer"`
	DurationMS int    `json:"duration_ms"`
}

func (s *Server) handleSessionAttempt(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserIDFromContext(ctx)
	sessionID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad session id", http.StatusBadRequest)
		return
	}
	exists, err := tasks.SessionExists(ctx, s.db, userID, sessionID)
	if err != nil {
		httpError(w, "lookup session", err)
		return
	}
	if !exists {
		http.Error(w, "session not found or already ended", http.StatusNotFound)
		return
	}

	var req attemptRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.TaskID == 0 {
		http.Error(w, "task_id required", http.StatusBadRequest)
		return
	}

	kind, prompt, expected, rationale, err := tasks.LoadFull(ctx, s.db, req.TaskID)
	if err != nil {
		httpError(w, "load task", err)
		return
	}
	if kind == tasks.KindSpeak {
		http.Error(w, "use /api/session/{id}/speak for speaking tasks", http.StatusBadRequest)
		return
	}

	res, err := tasks.GradeText(kind, expected, req.Answer)
	if err != nil {
		httpError(w, "grade", err)
		return
	}
	res.Rationale = rationale

	if res.Ambiguous && (kind == tasks.KindTrRUSR || kind == tasks.KindTrSRRU) {
		s.maybeUpgradeWithClaude(ctx, kind, prompt, expected, req.Answer, &res)
	}

	if err := updateSRSAndLog(ctx, s.db, userID, sessionID, req, res); err != nil {
		httpError(w, "persist", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

type endResponse struct {
	SessionID   int64 `json:"session_id"`
	Attempts    int   `json:"attempts"`
	Correct     int   `json:"correct"`
	DurationSec int   `json:"duration_sec"`
}

func (s *Server) handleSessionEnd(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserIDFromContext(ctx)
	sessionID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad session id", http.StatusBadRequest)
		return
	}
	if err := tasks.EndSession(ctx, s.db, userID, sessionID); err != nil {
		httpError(w, "end session", err)
		return
	}
	var resp endResponse
	resp.SessionID = sessionID
	var startedAt, endedAt int64
	row := s.db.QueryRowContext(ctx,
		`SELECT started_at, COALESCE(ended_at, unixepoch()) FROM sessions
		 WHERE id = ? AND user_id = ?`, sessionID, userID)
	if err := row.Scan(&startedAt, &endedAt); err == nil {
		resp.DurationSec = int(endedAt - startedAt)
	}
	row = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN grade >= 3 THEN 1 ELSE 0 END), 0)
		 FROM attempts WHERE session_id = ? AND user_id = ? AND skipped = 0`,
		sessionID, userID)
	_ = row.Scan(&resp.Attempts, &resp.Correct)
	writeJSON(w, http.StatusOK, resp)
}

func updateSRSAndLog(ctx context.Context, db *sql.DB, userID, sessionID int64, req attemptRequest, res tasks.GradeResult) error {
	now := time.Now()
	state, err := tasks.LoadSRS(ctx, db, userID, req.TaskID)
	if err != nil {
		return err
	}
	state = tasks.UpdateSRS(state, res.Grade, now)
	if err := tasks.SaveSRS(ctx, db, userID, req.TaskID, state); err != nil {
		return err
	}
	return tasks.LogAttempt(ctx, db, userID, sessionID, req.TaskID, req.Answer,
		res.GradedBy, res.Feedback, res.Grade, req.DurationMS)
}

// maybeUpgradeWithClaude calls the Anthropic grader for ambiguous translation
// answers, gated by the daily call budget. Best-effort: silent fallthrough on
// any error (the fuzzy grade was already written into `res`). Budget is
// instance-global (not per-user) by design.
func (s *Server) maybeUpgradeWithClaude(ctx context.Context, kind, prompt string, expected []byte, userAnswer string, res *tasks.GradeResult) {
	if s.llm == nil {
		return
	}
	if s.cfg.DailyClaudeBudget <= 0 {
		return
	}
	today := time.Now().UTC().Format("2006-01-02")
	var used int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM claude_calls WHERE day = ? AND purpose LIKE 'grade%'`,
		today,
	).Scan(&used); err != nil {
		log.Printf("budget check: %v", err)
		return
	}
	if used >= s.cfg.DailyClaudeBudget {
		log.Printf("claude grade budget exhausted (%d/%d today)", used, s.cfg.DailyClaudeBudget)
		return
	}

	var exp tasks.Expected
	if err := json.Unmarshal(expected, &exp); err != nil {
		return
	}
	graded, usage, err := s.llm.GradeTranslation(ctx, prompt, exp.Answers, userAnswer)
	if err != nil {
		log.Printf("claude grade: %v", err)
		// Log the failed call too so the budget accounts for it.
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO claude_calls (day, purpose, model, input_tok, output_tok, ok, created_at)
			VALUES (?, ?, ?, ?, ?, 0, unixepoch())
		`, today, "grade:"+kind, s.cfg.AnthropicModel, usage.InputTokens+usage.CacheRead+usage.CacheCreate, usage.OutputTokens)
		return
	}
	_, _ = s.db.ExecContext(ctx, `
		INSERT INTO claude_calls (day, purpose, model, input_tok, output_tok, ok, created_at)
		VALUES (?, ?, ?, ?, ?, 1, unixepoch())
	`, today, "grade:"+kind, s.cfg.AnthropicModel, usage.InputTokens+usage.CacheRead+usage.CacheCreate, usage.OutputTokens)

	res.Grade = graded.Grade
	res.GradedBy = "claude"
	res.Correct = graded.Grade >= 3
	res.Feedback = graded.Feedback
	res.Ambiguous = false
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func httpError(w http.ResponseWriter, what string, err error) {
	log.Printf("%s: %v", what, err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}
