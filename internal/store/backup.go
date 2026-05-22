package store

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Backup creates a hot snapshot of the open DB at `dir/serbian-YYYY-MM-DD.db`
// using SQLite's `VACUUM INTO`. Safe to call while the main server is
// running — VACUUM INTO holds a read lock only.
func Backup(ctx context.Context, db *sql.DB, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir backups: %w", err)
	}
	stamp := time.Now().UTC().Format("2006-01-02")
	out := filepath.Join(dir, "serbian-"+stamp+".db")
	// If today's backup exists already, overwrite by removing first
	// (VACUUM INTO refuses to write to an existing file).
	_ = os.Remove(out)
	if _, err := db.ExecContext(ctx, "VACUUM INTO ?", out); err != nil {
		return "", fmt.Errorf("vacuum into %s: %w", out, err)
	}
	return out, nil
}

// PruneBackups keeps the `keep` most-recent serbian-*.db backups in `dir`.
func PruneBackups(dir string, keep int) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "serbian-") && strings.HasSuffix(n, ".db") {
			matches = append(matches, n)
		}
	}
	if len(matches) <= keep {
		return nil
	}
	// Sort lexically, newer last (serbian-YYYY-MM-DD.db sorts chronologically).
	sort.Strings(matches)
	toDelete := matches[:len(matches)-keep]
	for _, n := range toDelete {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			return err
		}
	}
	return nil
}

// StartBackupLoop runs daily backups. First backup fires after a small delay
// (so an immediately-restarting server doesn't churn), subsequent ones fire
// at ~04:30 local each day. Keeps the most recent `keep` files.
func StartBackupLoop(ctx context.Context, db *sql.DB, dir string, keep int) {
	// Initial delay so a server that restarts every few minutes doesn't
	// backup on every boot — but if it stays up, take a first backup ~1
	// minute in so the user sees the file appear quickly.
	firstDelay := time.Minute
	t := time.NewTimer(firstDelay)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			path, err := Backup(ctx, db, dir)
			if err != nil {
				log.Printf("backup: %v", err)
			} else {
				log.Printf("backup: wrote %s", path)
			}
			if err := PruneBackups(dir, keep); err != nil {
				log.Printf("backup: prune: %v", err)
			}
			t.Reset(nextBackupSlot(time.Now()))
		}
	}
}

// nextBackupSlot returns the duration until the next 04:30 local time.
func nextBackupSlot(now time.Time) time.Duration {
	target := time.Date(now.Year(), now.Month(), now.Day(), 4, 30, 0, 0, now.Location())
	if !target.After(now) {
		target = target.AddDate(0, 0, 1)
	}
	return time.Until(target)
}
