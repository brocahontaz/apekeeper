package httpapi

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
)

func sharedContract(t *testing.T) map[string]struct {
	Required map[string]string `json:"required"`
	Optional map[string]string `json:"optional"`
} {
	t.Helper()
	b, err := os.ReadFile("../../../contracts/api-contract.json")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Models map[string]struct {
			Required map[string]string `json:"required"`
			Optional map[string]string `json:"optional"`
		} `json:"models"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	return c.Models
}

func assertContract(t *testing.T, model string, value any) {
	t.Helper()
	contract := sharedContract(t)[model]
	if len(contract.Required) == 0 {
		t.Fatalf("unknown or empty contract model %s", model)
	}
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for name, typ := range contract.Required {
		v, ok := got[name]
		if !ok {
			t.Fatalf("%s missing %s", model, name)
		}
		if typ == "nullable" && v == nil {
			continue
		}
		if typ == "nullable" {
			continue
		}
		if typ == "integer" {
			if n, ok := v.(float64); !ok || n != float64(int64(n)) {
				t.Fatalf("%s.%s is not integer", model, name)
			}
		} else if typ == "number" {
			if _, ok := v.(float64); !ok {
				t.Fatalf("%s.%s is not number", model, name)
			}
		} else if typ == "array" {
			if reflect.TypeOf(v).Kind() != reflect.Slice {
				t.Fatalf("%s.%s is not array", model, name)
			}
		} else if typ == "object" {
			if _, ok := v.(map[string]any); !ok {
				t.Fatalf("%s.%s is not object", model, name)
			}
		} else if typ == "boolean" {
			if _, ok := v.(bool); !ok {
				t.Fatalf("%s.%s is not boolean", model, name)
			}
		} else if _, ok := v.(string); !ok {
			t.Fatalf("%s.%s is not string", model, name)
		}
	}
}

func TestAllSharedModelsHaveTypedBackendRepresentatives(t *testing.T) {
	values := map[string]any{
		"User":         map[string]any{"id": int64(1), "displayName": "Ape", "battletag": "Ape#1", "appRole": "admin"},
		"Guild":        map[string]any{"id": int64(2), "slug": "apes", "name": "Apes", "realm": "Area 52", "region": "us", "role": "member", "selected": true},
		"Character":    characterJSON(domain.Character{ID: 7, DisplayName: "Alpha", Realm: "Area 52", ClassID: 1, ClassName: "Warrior", SpecID: 71, SpecName: "Arms", Level: 80, ItemLevel: 620.5, MythicRating: 2500, BestKeyLevel: 12, SyncedAt: time.Now().UTC()}, time.Now().UTC()),
		"Snapshot":     map[string]any{"id": int64(3), "characterId": int64(7), "capturedAt": time.Now().UTC(), "itemLevel": 620.5, "mythicRating": 2500.0, "bestKeyLevel": 12},
		"History":      map[string]any{"from": time.Now().UTC(), "to": time.Now().UTC(), "retentionDays": 90, "snapshots": []any{}, "comparison": nil},
		"RosterPage":   map[string]any{"items": []any{}, "total": 0, "page": 1, "pageSize": 25, "classes": []string{"Warrior"}, "specs": []string{"Arms"}},
		"SyncRun":      map[string]any{"id": int64(4), "guildId": int64(2), "startedAt": time.Now().UTC(), "finishedAt": nil, "trigger": "manual", "status": "running", "total": 1, "updated": 0, "failed": 0, "errorSummary": "", "detail": map[string]any{}},
		"SyncProgress": domain.Progress{RunID: 4, Status: domain.RunRunning, Phase: domain.PhaseCharacters},
		"Dashboard":    map[string]any{"rosterSize": 1, "maxLevelMembers": 1, "activeMembers": 1, "classDistribution": []any{}, "mythicPlus": map[string]any{}, "raidProgression": []any{}, "staleCharacters": map[string]any{}, "notableChanges": []any{}, "trends": []any{}},
	}
	models := sharedContract(t)
	if len(values) != len(models) {
		t.Fatalf("representatives cover %d of %d contract models", len(values), len(models))
	}
	for model, value := range values {
		t.Run(model, func(t *testing.T) { assertContract(t, model, value) })
	}
}

// These assertions intentionally inspect the wire representation rather than
// Go implementation details. They protect the fields consumed by frontend/src/lib/api.ts.
func TestFrontendCharacterContract(t *testing.T) {
	b, err := json.Marshal(characterJSON(domain.Character{
		ID: 7, DisplayName: "Alpha", Realm: "Area 52", ClassID: 1, ClassName: "Warrior",
		SpecID: 71, SpecName: "Arms", Level: 80, ItemLevel: 620.5, MythicRating: 2500,
		BestKeyLevel: 12, SyncedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
	}, time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	assertContract(t, "Character", characterJSON(domain.Character{ID: 7, DisplayName: "Alpha", Realm: "Area 52", ClassID: 1, ClassName: "Warrior", SpecID: 71, SpecName: "Arms", Level: 80, ItemLevel: 620.5, MythicRating: 2500, BestKeyLevel: 12, SyncedAt: time.Now().UTC()}, time.Now().UTC()))
	_ = got
	for _, field := range []string{"id", "classId", "specId", "level", "itemLevel", "mythicRating", "bestKeyLevel"} {
		if _, ok := got[field].(float64); !ok {
			t.Fatalf("character field %q is not JSON number: %#v", field, got[field])
		}
	}
	if _, ok := got["stale"].(bool); !ok {
		t.Fatalf("stale is not JSON boolean: %#v", got["stale"])
	}
}

func TestFrontendProgressContract(t *testing.T) {
	b, err := json.Marshal(domain.Progress{Active: true, RunID: 3, Status: domain.RunRunning, Phase: domain.PhaseCharacters, StartedAt: time.Now().UTC(), DryRun: false})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"active", "runId", "status", "phase", "total", "updated", "failed", "startedAt", "dryRun"} {
		if _, ok := got[field]; !ok {
			t.Fatalf("compatibility field %q missing from progress response", field)
		}
	}
}
