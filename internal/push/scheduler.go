package push

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"
)

// Scheduler fires daily reminders at configured local times. One in-process
// goroutine; on restart it recomputes the next slot from settings, so a
// restart neither double-fires nor skips.
type Scheduler struct {
	db            *sql.DB
	sender        *Sender
	timezone      string
	reminderTimes []string
}

func NewScheduler(db *sql.DB, sender *Sender, timezone string, reminderTimes []string) *Scheduler {
	return &Scheduler{db: db, sender: sender, timezone: timezone, reminderTimes: reminderTimes}
}

func (s *Scheduler) Run(ctx context.Context) {
	if !s.sender.Configured() {
		log.Println("push: VAPID not configured; scheduler disabled")
		return
	}
	if len(s.reminderTimes) == 0 {
		log.Println("push: no reminder_times configured; scheduler disabled")
		return
	}
	loc, err := time.LoadLocation(s.timezone)
	if err != nil {
		log.Printf("push: bad timezone %q, using UTC: %v", s.timezone, err)
		loc = time.UTC
	}
	log.Printf("push: scheduler running in %s with slots %v", loc.String(), s.reminderTimes)

	for {
		next := s.nextSlot(time.Now().In(loc), loc)
		if next.IsZero() {
			log.Println("push: no valid reminder slot; scheduler exiting")
			return
		}
		wait := time.Until(next)
		log.Printf("push: next reminder %s (in %s)", next.Format(time.RFC3339), wait.Round(time.Second))
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
			s.fire(ctx, "scheduled")
		}
	}
}

func (s *Scheduler) nextSlot(now time.Time, loc *time.Location) time.Time {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	var best time.Time
	for _, ts := range s.reminderTimes {
		h, m, ok := parseHMM(ts)
		if !ok {
			continue
		}
		t := time.Date(today.Year(), today.Month(), today.Day(), h, m, 0, 0, loc)
		if !t.After(now) {
			continue
		}
		if best.IsZero() || t.Before(best) {
			best = t
		}
	}
	if !best.IsZero() {
		return best
	}
	// Roll over to tomorrow's earliest slot.
	tomorrow := today.AddDate(0, 0, 1)
	var earliest time.Time
	for _, ts := range s.reminderTimes {
		h, m, ok := parseHMM(ts)
		if !ok {
			continue
		}
		t := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), h, m, 0, 0, loc)
		if earliest.IsZero() || t.Before(earliest) {
			earliest = t
		}
	}
	return earliest
}

func parseHMM(s string) (int, int, bool) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// Configured returns whether VAPID keys are set (i.e. the scheduler can
// actually send pushes).
func (s *Scheduler) Configured() bool {
	return s != nil && s.sender != nil && s.sender.Configured()
}

// Fire sends the standard reminder to every subscription. Exposed so the
// /api/push/test endpoint can use the same path. The trigger label appears
// in the fire-start / fire-done log lines so operators can tell scheduled
// ticks apart from manual test invocations.
func (s *Scheduler) Fire(ctx context.Context, trigger string) error {
	return s.fire(ctx, trigger)
}

func (s *Scheduler) fire(ctx context.Context, trigger string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT id, endpoint, p256dh, auth FROM push_subs`)
	if err != nil {
		return fmt.Errorf("query subs: %w", err)
	}
	type record struct {
		ID  int64
		Sub Subscription
	}
	var subs []record
	for rows.Next() {
		var r record
		if err := rows.Scan(&r.ID, &r.Sub.Endpoint, &r.Sub.P256dh, &r.Sub.Auth); err != nil {
			rows.Close()
			return err
		}
		subs = append(subs, r)
	}
	rows.Close()

	log.Printf("push: fire start trigger=%s subs=%d", trigger, len(subs))

	n := Notification{Title: "Српски", Body: "Време је за кратку вежбу.", URL: "/?from=push"}
	var okCount, goneCount, errCount int
	for _, r := range subs {
		status, err := s.sender.Send(ctx, r.Sub, n)
		if err != nil {
			log.Printf("push: send sub=#%d error=%v endpoint=%s", r.ID, err, r.Sub.Endpoint)
			errCount++
			s.bumpFailure(ctx, r.ID)
			continue
		}
		switch {
		case status == 410 || status == 404:
			log.Printf("push: send sub=#%d status=%d gone endpoint=%s", r.ID, status, r.Sub.Endpoint)
			goneCount++
			s.bumpFailure(ctx, r.ID)
		case status >= 200 && status < 300:
			log.Printf("push: send sub=#%d status=%d ok endpoint=%s", r.ID, status, r.Sub.Endpoint)
			okCount++
			s.markOK(ctx, r.ID)
		default:
			// 5xx from the push service, or anything else unexpected. Count
			// as an error and bump the failure counter — a broken upstream
			// should eventually retire a stale sub like a 410 would.
			log.Printf("push: send sub=#%d status=%d unexpected endpoint=%s", r.ID, status, r.Sub.Endpoint)
			errCount++
			s.bumpFailure(ctx, r.ID)
		}
	}
	log.Printf("push: fire done trigger=%s sent=%d gone=%d errors=%d", trigger, okCount, goneCount, errCount)
	return nil
}

func (s *Scheduler) bumpFailure(ctx context.Context, id int64) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE push_subs SET failures = failures + 1 WHERE id = ?`, id); err != nil {
		log.Printf("push: bump fail: %v", err)
	}
	if _, err := s.db.ExecContext(ctx,
		`DELETE FROM push_subs WHERE id = ? AND failures >= 3`, id); err != nil {
		log.Printf("push: drop dead sub: %v", err)
	}
}

func (s *Scheduler) markOK(ctx context.Context, id int64) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE push_subs SET last_ok_at = unixepoch(), failures = 0 WHERE id = ?`, id); err != nil {
		log.Printf("push: mark ok: %v", err)
	}
}
