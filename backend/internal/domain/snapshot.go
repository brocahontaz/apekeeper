package domain

import (
	"encoding/json"
	"time"
)

type Snapshot struct {
	ID           int64     `json:"id"`
	CharacterID  int64     `json:"characterId"`
	CapturedAt   time.Time `json:"capturedAt"`
	ItemLevel    float64   `json:"itemLevel"`
	MythicRating float64   `json:"mythicRating"`
	BestKeyLevel int       `json:"bestKeyLevel"`
	// RawMessage preserves the jsonb snapshot as JSON on API responses rather
	// than encoding its bytes as a base64 string.
	RaidProgress json.RawMessage `json:"raidProgress"`
}

func SnapshotChanged(prev, next Snapshot) bool {
	return prev.ItemLevel != next.ItemLevel ||
		prev.MythicRating != next.MythicRating ||
		prev.BestKeyLevel != next.BestKeyLevel ||
		string(prev.RaidProgress) != string(next.RaidProgress)
}
