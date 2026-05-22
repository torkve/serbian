package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"serbian/internal/tasks"
)

// deferInterval is how far into the future a skipped task's due_at is pushed.
// The task remains eligible for the next session but won't dominate the
// composer's "due now" window immediately after a skip.
const deferInterval = 10 * time.Minute

type skipRequest struct {
	TaskID     int64 `json:"task_id"`
	DurationMS int   `json:"duration_ms"`
}

// handleSessionSkip lets the client mark a task as deferred ("not now, ask me
// later"). The task's SRS state is left untouched aside from bumping due_at
// forward by deferInterval; an attempts row is logged with skipped=1 so we
// can later see how often each task gets deferred.
func (s *Server) handleSessionSkip(w http.ResponseWriter, r *http.Request) {
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

	var req skipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.TaskID == 0 {
		http.Error(w, "task_id required", http.StatusBadRequest)
		return
	}

	// Confirm the task exists (LoadExpected returns sql.ErrNoRows for unknown ids).
	if _, _, _, err := tasks.LoadExpected(ctx, s.db, req.TaskID); err != nil {
		httpError(w, "load task", err)
		return
	}

	if err := tasks.DeferTask(ctx, s.db, userID, req.TaskID, deferInterval); err != nil {
		httpError(w, "defer task", err)
		return
	}
	if err := tasks.LogSkip(ctx, s.db, userID, sessionID, req.TaskID, req.DurationMS); err != nil {
		httpError(w, "log skip", err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"deferred_by_sec": int(deferInterval.Seconds()),
	})
}
