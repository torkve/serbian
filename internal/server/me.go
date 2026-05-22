package server

import (
	"encoding/json"
	"errors"
	"net/http"

	"serbian/internal/store"
)

// meResponse is the JSON shape returned by GET /api/me and PATCH
// /api/me/prefs. Token is deliberately omitted — once the cookie is set the
// client never needs it again, and leaking it via JSON would defeat the
// HttpOnly cookie.
type meResponse struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	DifficultyMin int    `json:"difficulty_min"`
	DifficultyMax int    `json:"difficulty_max"`
	CreatedAt     int64  `json:"created_at"`
	LastSeenAt    int64  `json:"last_seen_at,omitempty"`
}

func meResponseFor(u *store.User) meResponse {
	r := meResponse{
		ID:            u.ID,
		Name:          u.Name,
		DifficultyMin: u.DifficultyMin,
		DifficultyMax: u.DifficultyMax,
		CreatedAt:     u.CreatedAt.Unix(),
	}
	if u.LastSeenAt.Valid {
		r.LastSeenAt = u.LastSeenAt.Time.Unix()
	}
	return r
}

func (s *Server) handleGetMe(w http.ResponseWriter, r *http.Request) {
	u := UserFromContext(r.Context())
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	writeJSON(w, http.StatusOK, meResponseFor(u))
}

type updatePrefsRequest struct {
	DifficultyMin int `json:"difficulty_min"`
	DifficultyMax int `json:"difficulty_max"`
}

func (s *Server) handleUpdatePrefs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	u := UserFromContext(ctx)
	if u == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	var req updatePrefsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if err := store.UpdateUserPrefs(ctx, s.db, u.ID, req.DifficultyMin, req.DifficultyMax); err != nil {
		if errors.Is(err, store.ErrInvalidPrefs) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if errors.Is(err, store.ErrUserNotFound) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		httpError(w, "update prefs", err)
		return
	}
	// Mutate the in-context user so the response (and any downstream
	// middleware on this request) sees the new values without a re-fetch.
	u.DifficultyMin = req.DifficultyMin
	u.DifficultyMax = req.DifficultyMax
	writeJSON(w, http.StatusOK, meResponseFor(u))
}
