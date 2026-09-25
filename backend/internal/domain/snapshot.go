package domain

import "time"

type Snapshot struct {
	CharacterID  int64     `json:"characterId"`
	CapturedAt   time.Time `json:"capturedAt"`
	ItemLevel    float64   `json:"itemLevel"`
	MythicRating float64   `json:"mythicRating"`
	BestKeyLevel int       `json:"bestKeyLevel"`
	RaidProgress []byte    `json:"-"`
}

func SnapshotChanged(prev, next Snapshot) bool {
	return prev.ItemLevel != next.ItemLevel ||
		prev.MythicRating != next.MythicRating ||
		prev.BestKeyLevel != next.BestKeyLevel ||
		string(prev.RaidProgress) != string(next.RaidProgress)
}
