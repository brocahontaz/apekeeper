package notify

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type logCapture struct {
	mu      sync.Mutex
	levels  []string
	matches []string
	// texts records each record's message plus its attributes so tests can
	// assert that no sanitized value (e.g. a webhook URL) ever leaks.
	texts []string
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Message)
	r.Attrs(func(a slog.Attr) bool {
		fmt.Fprintf(&b, " %s=%v", a.Key, a.Value)
		return true
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.levels = append(c.levels, r.Level.String())
	c.matches = append(c.matches, r.Message)
	c.texts = append(c.texts, b.String())
	return nil
}
func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) has(level, msg string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, l := range c.levels {
		if strings.EqualFold(l, level) && c.matches[i] == msg {
			return true
		}
	}
	return false
}

func (c *logCapture) leaks(needle string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, t := range c.texts {
		if strings.Contains(t, needle) {
			return true
		}
	}
	return false
}

func summary() SyncRunSummary {
	return SyncRunSummary{
		Guild:    "Ape Enclosure",
		Trigger:  "scheduled",
		Status:   "partial",
		Total:    120,
		Updated:  118,
		Failed:   2,
		Duration: 45 * time.Second,
		Reasons:  []string{"Alpha: rating unavailable", "Beta: raids unavailable"},
	}
}

func TestDiscordPayloadCarriesRunFields(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	p := discordPayload(summary(), now)

	wantContent := "Guild sync for Ape Enclosure finished with status partial: 118/120 characters synced, 2 failed (trigger: scheduled, duration: 45s)."
	if p.Content != wantContent {
		t.Fatalf("content=%q, want %q", p.Content, wantContent)
	}
	if len(p.Embeds) != 1 {
		t.Fatalf("embeds=%d, want one", len(p.Embeds))
	}
	embed := p.Embeds[0]
	if embed.Title != "Guild sync · Ape Enclosure" {
		t.Fatalf("title=%q", embed.Title)
	}
	if embed.Color != 0xd08a2d {
		t.Fatalf("partial color=%#x, want the partial amber", embed.Color)
	}
	if embed.Timestamp != "2026-09-30T12:00:00Z" {
		t.Fatalf("timestamp=%q, want an RFC3339 UTC timestamp", embed.Timestamp)
	}
	fields := map[string]string{}
	for _, f := range embed.Fields {
		fields[f.Name] = f.Value
	}
	wantFields := map[string]string{
		"Trigger":         "scheduled",
		"Synced":          "118 / 120",
		"Failed":          "2",
		"Duration":        "45s",
		"Failure reasons": "Alpha: rating unavailable\nBeta: raids unavailable",
	}
	for name, want := range wantFields {
		if fields[name] != want {
			t.Fatalf("field %s=%q, want %q", name, fields[name], want)
		}
	}
	if len(embed.Fields) != len(wantFields) {
		t.Fatalf("fields=%v, want exactly %v", embed.Fields, wantFields)
	}
}

func TestDiscordPayloadOmitsFailuresWithoutReasons(t *testing.T) {
	s := summary()
	s.Reasons = nil
	p := discordPayload(s, time.Now())
	for _, f := range p.Embeds[0].Fields {
		if f.Name == "Failure reasons" {
			t.Fatalf("unexpected failure reasons field: %v", f)
		}
	}
	if got := failureLines(nil); got != "" {
		t.Fatalf("failureLines(nil)=%q, want empty", got)
	}
}

func TestFailureLinesCapsReasonsAndCountsTheRest(t *testing.T) {
	reasons := []string{"r1", "r2", "r3", "r4", "r5", "r6", "r7"}
	want := "r1\nr2\nr3\nr4\nr5\n+2 more"
	if got := failureLines(reasons); got != want {
		t.Fatalf("failureLines=%q, want %q", got, want)
	}
	if got := failureLines(reasons[:5]); got != "r1\nr2\nr3\nr4\nr5" {
		t.Fatalf("failureLines(5)=%q, want all five without a more line", got)
	}
}

func TestDiscordNotifierSkipsEmptyWebhookURL(t *testing.T) {
	sent := false
	n := &DiscordNotifier{
		URL: "",
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		HTTP: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			sent = true
			return nil, errors.New("must not send")
		})},
	}
	o := n.NotifySyncRun(summary())
	if sent {
		t.Fatal("an empty webhook URL must skip delivery entirely")
	}
	if o.Status != OutcomeSkipped || o.Detail != "" {
		t.Fatalf("outcome=%+v, want a detail-free skipped outcome", o)
	}
}

func TestDiscordNotifierPostsJSONAndLogsFailures(t *testing.T) {
	var mu sync.Mutex
	var gotContent string
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/missing") {
			// A refused webhook exercises the warn-on-rejection path.
			w.WriteHeader(500)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		gotType = r.Header.Get("Content-Type")
		if strings.HasPrefix(gotType, "application/json") {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
				return
			}
			if s, _ := body["content"].(string); s != "" {
				gotContent = s
			}
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()

	logs := &logCapture{}
	n := NewDiscord(srv.URL, slog.New(logs))
	o := n.NotifySyncRun(summary())
	mu.Lock()
	delivered, contentType := gotContent != "", gotType
	mu.Unlock()
	if o.Status != OutcomeSent || o.Detail != "" {
		t.Fatalf("outcome=%+v, want a detail-free sent outcome", o)
	}
	if !delivered || !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("delivery contentType=%q content=%q", contentType, gotContent)
	}
	if logs.has("warn", "discord notification failed") {
		t.Fatal("a successful delivery must not log a failure")
	}

	// A rejected delivery logs at warn, never returns an error, and reports
	// a status-code-only detail that cannot carry the webhook URL.
	n.URL = srv.URL + "/missing"
	o = n.NotifySyncRun(summary())
	if o.Status != OutcomeFailed || o.Detail != "webhook returned 500" {
		t.Fatalf("outcome=%+v, want failed with the status code only", o)
	}
	if !logs.has("warn", "discord notification rejected") {
		t.Fatal("a rejected delivery must log a warning")
	}
	if logs.leaks(srv.URL) {
		t.Fatal("a rejected delivery must not log the webhook URL")
	}
}

// A transport failure — here a server that is already gone — must report a
// failed outcome whose detail and logs classify the error without ever
// carrying the webhook URL or its token.
func TestDiscordNotifierNetworkFailureSanitizesDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	logs := &logCapture{}
	n := NewDiscord(url, slog.New(logs))
	o := n.NotifySyncRun(summary())
	if o.Status != OutcomeFailed || o.Detail == "" {
		t.Fatalf("outcome=%+v, want a failed outcome with a detail", o)
	}
	if strings.Contains(o.Detail, url) || strings.Contains(o.Detail, "http") {
		t.Fatalf("detail=%q, want an error class without the webhook URL", o.Detail)
	}
	if logs.leaks(url) {
		t.Fatal("a failed delivery must not log the webhook URL")
	}
}

// A hung webhook must not hang the caller: with a tiny client timeout the
// delivery gives up quickly and reports a failed, sanitized outcome.
func TestDiscordNotifierTimeoutBoundsDelivery(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		// Sleep long past the injected timeout, but short enough to keep the
		// test fast; Server.Close still reaps it cleanly.
		time.Sleep(300 * time.Millisecond)
	}))
	defer srv.Close()

	logs := &logCapture{}
	n := NewDiscord(srv.URL, slog.New(logs))
	n.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	start := time.Now()
	o := n.NotifySyncRun(summary())
	elapsed := time.Since(start)
	if o.Status != OutcomeFailed || !strings.Contains(o.Detail, "timeout") {
		t.Fatalf("outcome=%+v, want failed with a timeout detail", o)
	}
	if elapsed > time.Second {
		t.Fatalf("delivery took %v, want it bounded near the 50ms client timeout", elapsed)
	}
	if strings.Contains(o.Detail, srv.URL) || logs.leaks(srv.URL) {
		t.Fatal("a timed-out delivery must not leak the webhook URL")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
