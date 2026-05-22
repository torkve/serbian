package stt

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// Client talks to a whisper.cpp `whisper-server` sidecar (binding to
// `cfg.WhisperURL`, default `http://127.0.0.1:8080/inference`).
type Client struct {
	serverURL string
	language  string
	http      *http.Client
}

func NewClient(serverURL, language string) *Client {
	if language == "" {
		language = "sr"
	}
	return &Client{
		serverURL: serverURL,
		language:  language,
		http:      &http.Client{Timeout: 2 * time.Minute},
	}
}

// Transcribe POSTs an audio blob to whisper-server and returns the transcript.
// `filename` is informational for the multipart form; whisper-server detects
// the actual audio format itself (it handles webm/opus, mp4/aac, wav, etc.
// when built with ffmpeg support).
func (c *Client) Transcribe(ctx context.Context, audio io.Reader, filename string) (string, error) {
	if filename == "" {
		filename = "audio.webm"
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if err := mw.WriteField("language", c.language); err != nil {
		return "", err
	}
	if err := mw.WriteField("temperature", "0.0"); err != nil {
		return "", err
	}
	if err := mw.WriteField("response_format", "json"); err != nil {
		return "", err
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(fw, audio); err != nil {
		return "", fmt.Errorf("copy audio: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.serverURL, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("whisper request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("whisper status %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}

	var out struct {
		Text string `json:"text"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("whisper decode: %w", err)
	}
	return strings.TrimSpace(out.Text), nil
}
