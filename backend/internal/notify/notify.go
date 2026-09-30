// Package notify posts sync outcome summaries to external channels such as
// Discord webhooks. Delivery failures are logged and never surfaced to sync
// callers.
package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
)

// maxFailureReasons caps the failure list in one notification; anything
// beyond it collapses into a "+N more" line.
const maxFailureReasons = 5

// webhookTimeout bounds a single Discord delivery.
const webhookTimeout = 10 * time.Second

// SyncRunSummary is the outcome of one finished sync run.
type SyncRunSummary struct {
	Guild    string
	Trigger  string
	Status   string
	Total    int
	Updated  int
	Failed   int
	Duration time.Duration
	Reasons  []string
}

// DiscordNotifier delivers sync summaries to a Discord webhook. An empty
// webhook URL disables delivery entirely; errors are logged at warn and never
// returned to callers.
type DiscordNotifier struct {
	URL string
	Log *slog.Logger
	// HTTP is optional; when nil a client with webhookTimeout is used.
	HTTP *http.Client
	// Now is optional; when nil time.Now timestamps the payload.
	Now func() time.Time
}

// NewDiscord builds a notifier for the given webhook URL. The URL may be
// empty, which makes every notification a no-op.
func NewDiscord(url string, log *slog.Logger) *DiscordNotifier {
	return &DiscordNotifier{URL: url, Log: log}
}

// NotifySyncRun posts a compact summary of the finished run. It never fails
// and never blocks on anything but its own bounded delivery.
func (n *DiscordNotifier) NotifySyncRun(s SyncRunSummary) {
	if n == nil || n.URL == "" {
		return
	}
	now := time.Now()
	if n.Now != nil {
		now = n.Now()
	}
	body, err := json.Marshal(discordPayload(s, now))
	if err != nil {
		n.warn("discord notification payload failed", err)
		return
	}
	client := n.HTTP
	if client == nil {
		client = &http.Client{Timeout: webhookTimeout}
	}
	resp, err := client.Post(n.URL, "application/json", bytes.NewReader(body))
	if err != nil {
		n.warn("discord notification failed", err)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		n.warn("discord notification rejected", fmt.Errorf("webhook returned %d", resp.StatusCode))
	}
}

func (n *DiscordNotifier) warn(msg string, err error) {
	if n.Log != nil {
		n.Log.Warn(msg, "error", err)
	}
}

type webhookPayload struct {
	Content string         `json:"content"`
	Embeds  []webhookEmbed `json:"embeds"`
}

type webhookEmbed struct {
	Title     string         `json:"title"`
	Color     int            `json:"color"`
	Timestamp string         `json:"timestamp"`
	Fields    []webhookField `json:"fields"`
}

type webhookField struct {
	Name   string `json:"name"`
	Value  string `json:"value"`
	Inline bool   `json:"inline,omitempty"`
}

// discordPayload builds the webhook body: one content line plus a single
// embed carrying the counts and the capped failure reasons.
func discordPayload(s SyncRunSummary, now time.Time) webhookPayload {
	embed := webhookEmbed{
		Title:     "Guild sync · " + s.Guild,
		Color:     statusColor(domain.RunStatus(s.Status)),
		Timestamp: now.UTC().Format(time.RFC3339),
		Fields: []webhookField{
			{Name: "Trigger", Value: s.Trigger, Inline: true},
			{Name: "Synced", Value: strconv.Itoa(s.Updated) + " / " + strconv.Itoa(s.Total), Inline: true},
			{Name: "Failed", Value: strconv.Itoa(s.Failed), Inline: true},
			{Name: "Duration", Value: s.Duration.String(), Inline: true},
		},
	}
	if reasons := failureLines(s.Reasons); reasons != "" {
		embed.Fields = append(embed.Fields, webhookField{Name: "Failure reasons", Value: reasons})
	}
	return webhookPayload{
		Content: fmt.Sprintf("Guild sync for %s finished with status %s: %d/%d characters synced, %d failed (trigger: %s, duration: %s).",
			s.Guild, s.Status, s.Updated, s.Total, s.Failed, s.Trigger, s.Duration.String()),
		Embeds: []webhookEmbed{embed},
	}
}

// failureLines renders at most maxFailureReasons reasons, one per line, and
// collapses the remainder into a "+N more" line.
func failureLines(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	lines := reasons
	more := 0
	if len(reasons) > maxFailureReasons {
		lines, more = reasons[:maxFailureReasons], len(reasons)-maxFailureReasons
	}
	out := append([]string(nil), lines...)
	if more > 0 {
		out = append(out, "+"+strconv.Itoa(more)+" more")
	}
	return strings.Join(out, "\n")
}

func statusColor(status domain.RunStatus) int {
	switch status {
	case domain.RunSuccess:
		return 0x4f8f3a
	case domain.RunPartial:
		return 0xd08a2d
	case domain.RunFailed:
		return 0xa83a22
	default:
		return 0x6b7280
	}
}
