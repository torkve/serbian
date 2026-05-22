package server

import (
	"net/http"
	"strconv"

	"serbian/internal/tasks"
)

// handleSessionSpeak accepts multipart/form-data:
//   - task_id (form field): the speak task ID
//   - duration_ms (form field, optional): client-measured recording duration
//   - audio (file): the recorded audio blob (typically audio/webm;opus)
//
// It streams the audio through whisper-server, grades the transcript against
// the task's expected answers, persists the attempt + SRS update, and returns
// the GradeResult.
func (s *Server) handleSessionSpeak(w http.ResponseWriter, r *http.Request) {
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

	const maxAudioBytes = 16 << 20 // 16 MiB — generous for ~1 min webm/opus
	r.Body = http.MaxBytesReader(w, r.Body, maxAudioBytes)
	if err := r.ParseMultipartForm(maxAudioBytes); err != nil {
		http.Error(w, "bad multipart: "+err.Error(), http.StatusBadRequest)
		return
	}

	taskID, err := strconv.ParseInt(r.FormValue("task_id"), 10, 64)
	if err != nil || taskID == 0 {
		http.Error(w, "task_id required", http.StatusBadRequest)
		return
	}
	durationMS, _ := strconv.Atoi(r.FormValue("duration_ms"))

	audioFile, audioHeader, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "audio file required", http.StatusBadRequest)
		return
	}
	defer audioFile.Close()

	kind, expected, rationale, err := tasks.LoadExpected(ctx, s.db, taskID)
	if err != nil {
		httpError(w, "load task", err)
		return
	}
	if kind != tasks.KindSpeak {
		http.Error(w, "task is not a speak task", http.StatusBadRequest)
		return
	}

	transcript, err := s.stt.Transcribe(ctx, audioFile, audioHeader.Filename)
	if err != nil {
		httpError(w, "whisper transcribe", err)
		return
	}

	res, err := tasks.GradeSpeech(expected, transcript)
	if err != nil {
		httpError(w, "grade", err)
		return
	}
	res.Rationale = rationale

	req := attemptRequest{TaskID: taskID, Answer: transcript, DurationMS: durationMS}
	if err := updateSRSAndLog(ctx, s.db, userID, sessionID, req, res); err != nil {
		httpError(w, "persist", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
