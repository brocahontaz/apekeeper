package notify

import (
	"context"
	"encoding/json"
	"errors"
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
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.levels = append(c.levels, r.Level.String())
	c.matches = append(c.matches, r.Message)
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
	n.NotifySyncRun(summary())
	if sent {
		t.Fatal("an empty webhook URL must skip delivery entirely")
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
	n.NotifySyncRun(summary())
	mu.Lock()
	delivered, contentType := gotContent != "", gotType
	mu.Unlock()
	if !delivered || !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("delivery contentType=%q content=%q", contentType, gotContent)
	}
	if logs.has("warn", "discord notification failed") {
		t.Fatal("a successful delivery must not log a failure")
	}

	// A rejected delivery logs at warn and never returns an error.
	n.URL = srv.URL + "/missing"
	n.NotifySyncRun(summary())
	if !logs.has("warn", "discord notification rejected") {
		t.Fatal("a rejected delivery must log a warning")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
