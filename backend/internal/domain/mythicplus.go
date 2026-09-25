package domain

import "time"

type MythicPlus struct {
	CharacterID   int64     `json:"characterId"`
	Season        string    `json:"season"`
	SeasonSlug    string    `json:"seasonSlug"`
	OverallRating float64   `json:"overallRating"`
	BestRunScore  float64   `json:"bestRunScore"`
	BestKeyLevel  int       `json:"bestKeyLevel"`
	Dungeons      []byte    `json:"-"`
	SyncedAt      time.Time `json:"syncedAt"`
}
