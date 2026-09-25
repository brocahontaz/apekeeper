package domain

import "time"

type Snapshot struct {
	CharacterID             int64
	CapturedAt              time.Time
	ItemLevel, MythicRating float64
	BestKeyLevel            int
	RaidProgress            []byte
}

func SnapshotChanged(prev, next Snapshot) bool {
	return prev.ItemLevel != next.ItemLevel ||
		prev.MythicRating != next.MythicRating ||
		prev.BestKeyLevel != next.BestKeyLevel ||
		string(prev.RaidProgress) != string(next.RaidProgress)
}
