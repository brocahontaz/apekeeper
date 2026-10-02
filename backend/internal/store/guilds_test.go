package store

import "testing"

func TestLegacyDefaultsPreserveExplicitGuildSettings(t *testing.T) {
	if schedule, webhook := legacyDefaults("02:00", "https://guild.example/hook", "04:00", "https://legacy.example/hook"); schedule != "02:00" || webhook != "https://guild.example/hook" {
		t.Fatalf("explicit settings overwritten: schedule=%q webhook=%q", schedule, webhook)
	}
	if schedule, webhook := legacyDefaults("03:00", "", "04:00", "https://legacy.example/hook"); schedule != "04:00" || webhook != "https://legacy.example/hook" {
		t.Fatalf("legacy defaults not applied: schedule=%q webhook=%q", schedule, webhook)
	}
}
