package push

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// Sender pushes Web Push notifications to a single subscription using the
// configured VAPID keypair.
type Sender struct {
	publicKey  string
	privateKey string
	subject    string
}

func NewSender(publicKey, privateKey, subject string) *Sender {
	return &Sender{publicKey: publicKey, privateKey: privateKey, subject: subject}
}

// Configured returns false when VAPID keys are not set.
func (s *Sender) Configured() bool {
	return s != nil && s.publicKey != "" && s.privateKey != ""
}

type Notification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
}

type Subscription struct {
	Endpoint string
	P256dh   string
	Auth     string
}

// Send pushes the notification. Returns the HTTP status from the push
// service (410/404 means the subscription is gone — caller should drop it).
func (s *Sender) Send(ctx context.Context, sub Subscription, n Notification) (int, error) {
	body, err := json.Marshal(n)
	if err != nil {
		return 0, err
	}
	wpSub := &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.P256dh,
			Auth:   sub.Auth,
		},
	}
	opts := &webpush.Options{
		Subscriber:      s.subject,
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             60 * 60 * 24,
	}
	resp, err := webpush.SendNotificationWithContext(ctx, body, wpSub, opts)
	if err != nil {
		return 0, fmt.Errorf("webpush send: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}
