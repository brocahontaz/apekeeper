package domain

import (
	"testing"
	"time"
)

func TestNormalizeAndSlugs(t *testing.T) {
	if NormalizeCharacterName(" O'Neil ") != "oneil" {
		t.Fatal("normalization")
	}
	if RealmSlug("Area 52") != "area-52" || GuildSlug("Ape Enclosure", "Area 52", "us") != "us-area-52-ape-enclosure" {
		t.Fatal("slug")
	}
}
func TestCharacterRules(t *testing.T) {
	now := time.Now()
	c := Character{Level: 70, SyncedAt: now.Add(-time.Hour)}
	if !c.ActiveCharacter(now, 2*time.Hour) || c.ActiveCharacter(now, time.Minute) {
		t.Fatal("active/stale")
	}
}
func TestSnapshotsAndTransitions(t *testing.T) {
	a := Snapshot{ItemLevel: 1}
	if SnapshotChanged(a, a) || !SnapshotChanged(a, Snapshot{ItemLevel: 2}) {
		t.Fatal("snapshot")
	}
	if !Transition(RunRunning, RunSuccess) || Transition(RunSuccess, RunFailed) || Outcome(1, 1) != RunPartial || Outcome(0, 1) != RunFailed {
		t.Fatal("transition")
	}
}
