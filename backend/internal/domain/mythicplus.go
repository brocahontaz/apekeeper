package domain

import "time"

type MythicPlus struct {
	CharacterID                 int64
	Season, SeasonSlug          string
	OverallRating, BestRunScore float64
	BestKeyLevel                int
	Dungeons                    []byte
	SyncedAt                    time.Time
}
