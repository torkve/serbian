package main

import (
	"context"
	"errors"
	"flag"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"serbian"
	"serbian/internal/config"
	"serbian/internal/llm"
	"serbian/internal/push"
	"serbian/internal/server"
	"serbian/internal/store"
)

func init() {
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
}

func main() {
	var (
		addr       string
		configPath string
		backupOnly bool
		backupDir  string
		backupKeep int
		addUser    string
		deleteUser string
		listUsers  bool
	)
	flag.StringVar(&addr, "addr", "", "listen address (overrides config)")
	flag.StringVar(&configPath, "config", "data/config.json", "config file path")
	flag.BoolVar(&backupOnly, "backup", false, "take a one-shot backup and exit")
	flag.StringVar(&backupDir, "backup-dir", "data/backups", "directory for daily snapshots")
	flag.IntVar(&backupKeep, "backup-keep", 7, "number of recent backups to retain")
	flag.StringVar(&addUser, "add-user", "", "create a new user with this name, print their setup link, then exit")
	flag.StringVar(&deleteUser, "delete-user", "", "delete the user with this name (cascades to all their per-user data) and exit")
	flag.BoolVar(&listUsers, "list-users", false, "print the list of registered users and exit")
	flag.Parse()

	cfg, err := config.Load(configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	if addr != "" {
		cfg.Addr = addr
	}
	if err := cfg.Save(configPath); err != nil {
		log.Fatalf("save config: %v", err)
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	db, err := store.Open(rootCtx, cfg.DBPath)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer db.Close()

	migrations, err := fs.Sub(serbian.MigrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrations fs: %v", err)
	}
	if err := store.Migrate(rootCtx, db, migrations); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	// Bootstrap the owner user once migrations have applied. Idempotent —
	// no-op when users already exist.
	if _, err := store.EnsureOwnerUser(rootCtx, db, cfg.AuthToken); err != nil {
		log.Fatalf("bootstrap owner user: %v", err)
	}

	if backupOnly {
		path, err := store.Backup(rootCtx, db, backupDir)
		if err != nil {
			log.Fatalf("backup: %v", err)
		}
		if err := store.PruneBackups(backupDir, backupKeep); err != nil {
			log.Printf("warn: prune backups: %v", err)
		}
		log.Printf("backup written: %s", path)
		return
	}

	if listUsers {
		users, err := store.ListUsers(rootCtx, db)
		if err != nil {
			log.Fatalf("list users: %v", err)
		}
		// id  name        created             last_seen           token
		log.Printf("%-4s %-20s %-20s %-20s %s", "id", "name", "created", "last_seen", "token")
		for _, u := range users {
			lastSeen := "never"
			if u.LastSeenAt.Valid {
				lastSeen = u.LastSeenAt.Time.Format(time.RFC3339)
			}
			log.Printf("%-4d %-20s %-20s %-20s %s",
				u.ID, u.Name,
				u.CreatedAt.Format(time.RFC3339),
				lastSeen,
				u.Token)
		}
		return
	}

	if addUser != "" {
		token, err := config.RandToken(24)
		if err != nil {
			log.Fatalf("generate token: %v", err)
		}
		u, err := store.CreateUser(rootCtx, db, addUser, token)
		if err != nil {
			log.Fatalf("add user: %v", err)
		}
		base := cfg.PublicURL
		if base == "" {
			base = "http://localhost" + cfg.Addr
		}
		log.Printf("created user #%d %s", u.ID, u.Name)
		log.Printf("setup link: %s/setup?token=%s", strings.TrimRight(base, "/"), u.Token)
		return
	}

	if deleteUser != "" {
		if err := store.DeleteUserByName(rootCtx, db, deleteUser); err != nil {
			log.Fatalf("delete user: %v", err)
		}
		log.Printf("deleted user %s and all their per-user data", deleteUser)
		return
	}

	go store.StartBackupLoop(rootCtx, db, backupDir, backupKeep)

	web, err := fs.Sub(serbian.WebFS, "web")
	if err != nil {
		log.Fatalf("web fs: %v", err)
	}

	pushSender := push.NewSender(cfg.VAPIDPublic, cfg.VAPIDPrivate, cfg.VAPIDSubject)
	pushScheduler := push.NewScheduler(db, pushSender, cfg.TimeZone, cfg.ReminderTimes)
	go pushScheduler.Run(rootCtx)

	var llmClient *llm.Client
	if cfg.AnthropicAPIKey != "" {
		llmClient = llm.NewClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
		log.Printf("anthropic: live grading enabled (model=%s, budget=%d/day)", cfg.AnthropicModel, cfg.DailyClaudeBudget)
	} else {
		log.Println("anthropic: no API key configured, live grading disabled")
	}

	srv := server.New(cfg, db, web, pushScheduler, llmClient)
	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("listening on %s", cfg.Addr)
	base := cfg.PublicURL
	if base == "" {
		base = "http://localhost" + cfg.Addr
	}
	log.Printf("setup link: %s/setup?token=%s", strings.TrimRight(base, "/"), cfg.AuthToken)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutdown signal received")
		shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutCancel()
		_ = httpSrv.Shutdown(shutCtx)
		rootCancel()
	}()

	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("http: %v", err)
	}
}
