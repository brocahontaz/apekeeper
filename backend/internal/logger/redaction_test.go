package logger

import (
	"strings"
	"testing"
)

func TestRedactSensitiveValues(t *testing.T) {
	for _, value := range []string{"access_token=abc", "refresh_token=def", "authorization=Bearer xyz", "https://discord.test/webhook/secret"} {
		if got := Redact(value); got == value || got == "" {
			t.Fatalf("value was not redacted: %q", value)
		}
	}
	got := Redact(`{"request":{"access_token":"abc","nested":{"webhook_url":"https://discord.test/webhook/token"}},"safe":"ok"}`)
	if strings.Contains(got, "abc") || strings.Contains(got, "/webhook/token") || !strings.Contains(got, "safe") {
		t.Fatalf("nested value was not safely redacted: %s", got)
	}
}
