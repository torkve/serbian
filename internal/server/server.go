package server

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"serbian/internal/config"
	"serbian/internal/llm"
	"serbian/internal/push"
	"serbian/internal/store"
	"serbian/internal/stt"
	"serbian/internal/tasks"
)

const (
	cookieName   = "serbian_auth"
	cookieMaxAge = 60 * 60 * 24 * 365 * 5 // ~5 years
)

type Server struct {
	cfg           *config.Config
	db            *sql.DB
	web           fs.FS
	mux           *http.ServeMux
	composer      *tasks.Composer
	stt           *stt.Client
	pushScheduler *push.Scheduler
	llm           *llm.Client // nil when no Anthropic API key configured
}

func New(cfg *config.Config, db *sql.DB, web fs.FS, pushScheduler *push.Scheduler, llmClient *llm.Client) *Server {
	s := &Server{
		cfg:           cfg,
		db:            db,
		web:           web,
		mux:           http.NewServeMux(),
		composer:      tasks.NewComposer(db),
		stt:           stt.NewClient(cfg.WhisperURL, cfg.WhisperLanguage),
		pushScheduler: pushScheduler,
		llm:           llmClient,
	}
	s.routes()
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("ok"))
	})
	s.mux.HandleFunc("GET /setup", s.handleSetup)

	s.mux.Handle("GET /api/me", s.requireAuth(http.HandlerFunc(s.handleGetMe)))
	s.mux.Handle("PATCH /api/me/prefs", s.requireAuth(http.HandlerFunc(s.handleUpdatePrefs)))

	s.mux.Handle("POST /api/session/start", s.requireAuth(http.HandlerFunc(s.handleSessionStart)))
	s.mux.Handle("POST /api/session/{id}/attempt", s.requireAuth(http.HandlerFunc(s.handleSessionAttempt)))
	s.mux.Handle("POST /api/session/{id}/speak", s.requireAuth(http.HandlerFunc(s.handleSessionSpeak)))
	s.mux.Handle("POST /api/session/{id}/skip", s.requireAuth(http.HandlerFunc(s.handleSessionSkip)))
	s.mux.Handle("POST /api/session/{id}/end", s.requireAuth(http.HandlerFunc(s.handleSessionEnd)))

	s.mux.Handle("GET /api/push/vapid", s.requireAuth(http.HandlerFunc(s.handlePushVAPID)))
	s.mux.Handle("POST /api/push/subscribe", s.requireAuth(http.HandlerFunc(s.handlePushSubscribe)))
	s.mux.Handle("POST /api/push/unsubscribe", s.requireAuth(http.HandlerFunc(s.handlePushUnsubscribe)))
	s.mux.Handle("POST /api/push/test", s.requireAuth(http.HandlerFunc(s.handlePushTest)))

	s.mux.Handle("GET /admin/review", s.requireAuth(http.HandlerFunc(s.handleAdminReview)))
	s.mux.Handle("GET /api/admin/tasks", s.requireAuth(http.HandlerFunc(s.handleAdminListTasks)))
	s.mux.Handle("POST /api/admin/tasks/{id}/flag", s.requireAuth(http.HandlerFunc(s.handleAdminFlagTask)))
	s.mux.Handle("DELETE /api/admin/tasks/{id}", s.requireAuth(http.HandlerFunc(s.handleAdminDeleteTask)))

	s.mux.Handle("GET /", s.requireAuth(http.FileServerFS(s.web)))
}

// publicPrefix returns the path prefix the app is mounted under as seen by
// the browser. When deployed behind a reverse proxy that strips the prefix
// before forwarding, the proxy must set `X-Forwarded-Prefix` so we emit the
// right Set-Cookie Path and the right Location on the auth redirect.
// Returns "" (root) or e.g. "/serbian" (no trailing slash).
func publicPrefix(r *http.Request) string {
	p := strings.TrimSpace(r.Header.Get("X-Forwarded-Prefix"))
	if p == "" {
		return ""
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return strings.TrimRight(p, "/")
}

// publicScheme picks the scheme the BROWSER used, honoring nginx's
// X-Forwarded-Proto. Used only for the Secure cookie attribute.
func publicScheme(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-Proto"); v != "" {
		return v
	}
	if r.TLS != nil {
		return "https"
	}
	return "http"
}

// ctxKey is a private type for request-context keys so the value can't be
// accidentally read with a string literal from another package.
type ctxKey int

const (
	ctxUser ctxKey = iota
)

// UserFromContext returns the authenticated user for the request, or nil if
// the middleware didn't set it (which means the handler was reached without
// going through requireAuth — that's a programming bug).
func UserFromContext(ctx context.Context) *store.User {
	u, _ := ctx.Value(ctxUser).(*store.User)
	return u
}

// UserIDFromContext is a thin wrapper kept for handlers that only need the
// id. Returns 0 when the context wasn't authed.
func UserIDFromContext(ctx context.Context) int64 {
	if u := UserFromContext(ctx); u != nil {
		return u.ID
	}
	return 0
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	u, err := store.LookupUserByToken(r.Context(), s.db, token)
	if err != nil {
		if !errors.Is(err, store.ErrUserNotFound) {
			log.Printf("setup: lookup user: %v", err)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(setupErrorHTML))
		return
	}
	// Browser-visible base. "/" when mounted at the origin root, or
	// "/serbian/" when behind nginx with X-Forwarded-Prefix /serbian.
	target := publicPrefix(r) + "/"
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    u.Token,
		Path:     target,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   publicScheme(r) == "https",
		MaxAge:   cookieMaxAge,
	})
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		var u *store.User
		if err == nil {
			u, err = store.LookupUserByToken(r.Context(), s.db, c.Value)
		}
		if err != nil || u == nil {
			if err != nil && !errors.Is(err, http.ErrNoCookie) && !errors.Is(err, store.ErrUserNotFound) {
				log.Printf("auth: %v", err)
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(authPromptHTML))
			return
		}
		// Best-effort last-seen bump. Errors here aren't worth failing on.
		if err := store.TouchLastSeen(r.Context(), s.db, u.ID); err != nil {
			log.Printf("auth: touch last_seen: %v", err)
		}
		ctx := context.WithValue(r.Context(), ctxUser, u)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

const authPromptHTML = `<!doctype html>
<html lang="sr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Приступ ограничен</title>
<style>body{font:18px/1.5 -apple-system,system-ui,sans-serif;padding:2em;max-width:32em;margin:auto;color:#222}</style>
</head><body>
<h1>Приступ ограничен</h1>
<p>Отворите везу за пријаву коју је сервер исписао у логу при покретању (<code>/setup?token=…</code>).</p>
</body></html>`

const setupErrorHTML = `<!doctype html>
<html lang="sr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Неисправан токен</title>
<style>body{font:18px/1.5 -apple-system,system-ui,sans-serif;padding:2em;max-width:32em;margin:auto;color:#222}</style>
</head><body>
<h1>Неисправан токен</h1>
<p>Токен у вези није исправан. Проверите URL у логу сервера.</p>
</body></html>`
