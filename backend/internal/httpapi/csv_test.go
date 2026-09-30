package httpapi

import (
	"bytes"
	"reflect"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
)

func TestRosterCSVHeader(t *testing.T) {
	want := []string{"Name", "Realm", "Class", "Spec", "Level", "Guild Rank", "Item Level",
		"M+ Rating", "Best Key", "Stale", "Last Seen"}
	if got := rosterCSVHeader(); !reflect.DeepEqual(got, want) {
		t.Fatalf("header=%q, want %q", got, want)
	}
}

func TestRosterCSVEncoding(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	var buf bytes.Buffer
	err := rosterCSV(&buf, []domain.Character{
		{
			DisplayName: `Bravo "Big", Jr`, Realm: "Area 52", ClassName: "Mage", SpecName: "",
			Level: 70, GuildRank: 1, ItemLevel: 620.5, MythicRating: 2500, BestKeyLevel: 12,
			SyncedAt: now.Add(-time.Hour),
		},
		{
			// Never synced: the epoch sentinel exports as stale with no last seen.
			DisplayName: "Gamma", Realm: "Area 52", ClassName: "Warrior", SpecName: "Arms",
			Level: 10, GuildRank: 5, ItemLevel: 100, MythicRating: 0, BestKeyLevel: 0,
			SyncedAt: time.Unix(0, 0),
		},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := `Name,Realm,Class,Spec,Level,Guild Rank,Item Level,M+ Rating,Best Key,Stale,Last Seen
"Bravo ""Big"", Jr",Area 52,Mage,,70,1,620.5,2500,12,false,2026-09-30T11:00:00Z
Gamma,Area 52,Warrior,Arms,10,5,100,0,0,true,
`
	if buf.String() != want {
		t.Fatalf("csv=%q, want %q", buf.String(), want)
	}
}
