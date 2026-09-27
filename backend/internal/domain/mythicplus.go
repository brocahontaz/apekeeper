package domain

import "time"

type MythicPlus struct {
	CharacterID   int64       `json:"characterId"`
	Season        string      `json:"season"`
	SeasonSlug    string      `json:"seasonSlug"`
	OverallRating float64     `json:"overallRating"`
	BestRunScore  float64     `json:"bestRunScore"`
	BestKeyLevel  int         `json:"bestKeyLevel"`
	Runs          *MythicRuns `json:"runs,omitempty"`
	SyncedAt      time.Time   `json:"syncedAt"`
}

// MythicRun is one keystone completion; CompletedAt is the unix millisecond
// timestamp Blizzard reports.
type MythicRun struct {
	Dungeon     string  `json:"dungeon"`
	Level       int     `json:"level"`
	Score       float64 `json:"score"`
	Timed       bool    `json:"timed"`
	CompletedAt int64   `json:"completedAt"`
}

type MythicRuns struct {
	Best   []MythicRun `json:"best,omitempty"`
	Recent []MythicRun `json:"recent,omitempty"`
}
