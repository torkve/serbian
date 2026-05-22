package server

import (
	"encoding/json"
	"net/http"
)

type subscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

func (s *Server) handlePushVAPID(w http.ResponseWriter, r *http.Request) {
	if s.cfg.VAPIDPublic == "" {
		http.Error(w, "VAPID not configured (run ./bin/vapid)", http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": s.cfg.VAPIDPublic})
}

func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserIDFromContext(ctx)
	var req subscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	if req.Endpoint == "" || req.Keys.P256dh == "" || req.Keys.Auth == "" {
		http.Error(w, "missing fields", http.StatusBadRequest)
		return
	}
	ua := r.Header.Get("User-Agent")
	// On endpoint conflict we re-assign to the *current* user — handles the
	// case where a device was previously subscribed under another account
	// and is now being re-registered.
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO push_subs (user_id, endpoint, p256dh, auth, ua, created_at, failures)
		VALUES (?, ?, ?, ?, ?, unixepoch(), 0)
		ON CONFLICT(endpoint) DO UPDATE SET
		    user_id = excluded.user_id,
		    p256dh  = excluded.p256dh,
		    auth    = excluded.auth,
		    ua      = excluded.ua
	`, userID, req.Endpoint, req.Keys.P256dh, req.Keys.Auth, ua); err != nil {
		httpError(w, "save subscription", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePushUnsubscribe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID := UserIDFromContext(ctx)
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Endpoint == "" {
		http.Error(w, "endpoint required", http.StatusBadRequest)
		return
	}
	// Scoped to the requesting user so a malicious cookie can't delete
	// another user's subscription. Endpoint is unique globally, so the
	// user_id filter is belt-and-suspenders.
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM push_subs WHERE endpoint = ? AND user_id = ?`,
		req.Endpoint, userID); err != nil {
		httpError(w, "unsubscribe", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	if s.pushScheduler == nil || !s.pushScheduler.Configured() {
		http.Error(w, "VAPID keys not configured — run ./bin/vapid and add them to data/config.json", http.StatusServiceUnavailable)
		return
	}
	if err := s.pushScheduler.Fire(r.Context(), "test"); err != nil {
		httpError(w, "fire test push", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
