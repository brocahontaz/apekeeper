package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

// The live captured profile summary shape: Blizzard now returns localized
// names as plain strings.
func TestProfileSummaryDecodesStringLocalizedNames(t *testing.T) {
	var p ProfileSummary
	fixture := `{
		"name": "Allcaps",
		"gender": {"type": "MALE", "name": "Male"},
		"faction": {"type": "HORDE", "name": "Horde"},
		"race": {"key": {"href": "https://eu.api.blizzard.com/data/wow/playable-race/2"}, "name": "Orc", "id": 2},
		"character_class": {"key": {"href": "https://eu.api.blizzard.com/data/wow/playable-class/1"}, "name": "Warrior", "id": 1},
		"active_spec": {"key": {"href": "https://eu.api.blizzard.com/data/wow/playable-specialization/72"}, "name": "Fury", "id": 72},
		"realm": {"key": {"href": "https://eu.api.blizzard.com/data/wow/realm/ravencrest"}, "name": "Ravencrest", "id": 554, "slug": "ravencrest"}
	}`
	if err := json.Unmarshal([]byte(fixture), &p); err != nil {
		t.Fatal(err)
	}
	if p.Race.Name != "Orc" || p.Gender.Name != "Male" || p.Gender.Type != "MALE" {
		t.Fatalf("race/gender=%q/%q type=%q, want Orc/Male MALE", p.Race.Name, p.Gender.Name, p.Gender.Type)
	}
}

// The legacy shape kept working before the API change: localized objects.
func TestProfileSummaryDecodesLocalizedObjects(t *testing.T) {
	var p ProfileSummary
	fixture := `{
		"race": {"id": 5, "name": {"en_US": "Dwarf", "es_MX": "Enano"}},
		"gender": {"type": "FEMALE", "name": {"en_US": "Female", "es_MX": "Femenino"}}
	}`
	if err := json.Unmarshal([]byte(fixture), &p); err != nil {
		t.Fatal(err)
	}
	if p.Race.Name != "Dwarf" || p.Gender.Name != "Female" || p.Gender.Type != "FEMALE" {
		t.Fatalf("race/gender=%q/%q type=%q, want Dwarf/Female FEMALE", p.Race.Name, p.Gender.Name, p.Gender.Type)
	}
}

// A response can mix both shapes while Blizzard's rollout is in flight.
func TestProfileSummaryDecodesMixedLocalizedShapes(t *testing.T) {
	var one, two ProfileSummary
	if err := json.Unmarshal([]byte(`{"race":{"id":2,"name":"Orc"},"gender":{"type":"MALE","name":{"en_US":"Male"}}}`), &one); err != nil {
		t.Fatal(err)
	}
	if one.Race.Name != "Orc" || one.Gender.Name != "Male" {
		t.Fatalf("race/gender=%q/%q, want Orc/Male", one.Race.Name, one.Gender.Name)
	}
	if err := json.Unmarshal([]byte(`{"race":{"id":2,"name":{"en_US":"Orc"}},"gender":{"type":"MALE","name":"Male"}}`), &two); err != nil {
		t.Fatal(err)
	}
	if two.Race.Name != "Orc" || two.Gender.Name != "Male" {
		t.Fatalf("race/gender=%q/%q, want Orc/Male", two.Race.Name, two.Gender.Name)
	}
}

// An invalid type is neither a string nor a localized object: it must fail
// the decode faithfully.
func TestLocalizedTextRejectsInvalidTypes(t *testing.T) {
	var p ProfileSummary
	if err := json.Unmarshal([]byte(`{"race":{"id":2,"name":42}}`), &p); err == nil {
		t.Fatal("race name 42 decoded without error, want a failure")
	}
	if err := json.Unmarshal([]byte(`{"gender":{"type":"MALE","name":true}}`), &p); err == nil {
		t.Fatal("gender name true decoded without error, want a failure")
	}
}

// The engine re-marshals the decoded summary into the stored profile JSON, so
// LocalizedText must marshal back as a plain JSON string.
func TestProfileSummaryMarshalsLocalizedNamesAsStrings(t *testing.T) {
	var p ProfileSummary
	fixture := `{
		"name": "Allcaps",
		"race": {"id": 2, "name": {"en_US": "Orc"}},
		"gender": {"type": "MALE", "name": "Male"}
	}`
	if err := json.Unmarshal([]byte(fixture), &p); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var out struct {
		Race struct {
			Name any `json:"name"`
		} `json:"race"`
		Gender struct {
			Name any `json:"name"`
		} `json:"gender"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if s, ok := out.Race.Name.(string); !ok || s != "Orc" {
		t.Fatalf("marshaled race name=%#v, want the plain string \"Orc\"", out.Race.Name)
	}
	if s, ok := out.Gender.Name.(string); !ok || s != "Male" {
		t.Fatalf("marshaled gender name=%#v, want the plain string \"Male\"", out.Gender.Name)
	}
	if !strings.Contains(string(raw), `"name":"Orc"`) {
		t.Fatalf("marshaled profile %s missing the plain race name", raw)
	}
}
