package domain

import "time"

type RaidProgression struct {
	CharacterID int64     `json:"characterId"`
	RaidSlug    string    `json:"raidSlug"`
	RaidName    string    `json:"raidName"`
	Difficulty  string    `json:"difficulty"`
	Progress    int       `json:"progress"`
	TotalBosses int       `json:"totalBosses"`
	Summary     []byte    `json:"-"`
	SyncedAt    time.Time `json:"syncedAt"`
}
