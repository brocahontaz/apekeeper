package store

import (
	"encoding/json"
	"testing"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
)

// raceGenderFromProfile is the tolerant parser behind the upsert: it must
// accept both localized-name shapes Blizzard has used and yield empty strings
// for anything it cannot parse.
func TestRaceGenderFromProfileShapes(t *testing.T) {
	cases := []struct {
		name, profile, race, gender string
	}{
		{
			name:    "plain strings",
			profile: `{"race":{"id":2,"name":"Orc"},"gender":{"type":"MALE","name":"Male"}}`,
			race:    "Orc", gender: "Male",
		},
		{
			name:    "localized objects",
			profile: `{"race":{"id":5,"name":{"en_US":"Dwarf","es_MX":"Enano"}},"gender":{"type":"FEMALE","name":{"en_US":"Female"}}}`,
			race:    "Dwarf", gender: "Female",
		},
		{
			name:    "mixed shapes",
			profile: `{"race":{"id":2,"name":"Orc"},"gender":{"type":"MALE","name":{"en_US":"Male"}}}`,
			race:    "Orc", gender: "Male",
		},
		{
			name:    "invalid name types",
			profile: `{"race":{"id":2,"name":42},"gender":{"type":"MALE","name":true}}`,
			race:    "", gender: "",
		},
		{
			name:    "unparseable profile",
			profile: `{"race":`,
			race:    "", gender: "",
		},
		{
			name:    "missing fields",
			profile: `{}`,
			race:    "", gender: "",
		},
	}
	for _, c := range cases {
		if race, gender := raceGenderFromProfile([]byte(c.profile)); race != c.race || gender != c.gender {
			t.Errorf("%s: race/gender=%q/%q, want %q/%q", c.name, race, gender, c.race, c.gender)
		}
	}
}

// The engine re-marshals the decoded profile summary into the stored
// profile_json, so a LocalizedText field must marshal back as a plain string
// this parser still reads.
func TestRaceGenderFromProfileRemarshaledSummary(t *testing.T) {
	var p dto.ProfileSummary
	fixture := `{
		"name": "Allcaps",
		"gender": {"type": "MALE", "name": "Male"},
		"race": {"key": {"href": "https://eu.api.blizzard.com/data/wow/playable-race/2"}, "name": "Orc", "id": 2},
		"realm": {"name": "Ravencrest", "id": 554, "slug": "ravencrest"}
	}`
	if err := json.Unmarshal([]byte(fixture), &p); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if race, gender := raceGenderFromProfile(raw); race != "Orc" || gender != "Male" {
		t.Fatalf("race/gender=%q/%q, want Orc/Male from remarshaled profile %s", race, gender, raw)
	}
}
