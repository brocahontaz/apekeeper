package domain

import "time"

type RaidProgression struct {
	CharacterID                    int64
	RaidSlug, RaidName, Difficulty string
	Progress, TotalBosses          int
	Summary                        []byte
	SyncedAt                       time.Time
}
