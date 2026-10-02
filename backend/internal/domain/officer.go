package domain

import "time"

const (
	OfficerNoteMaxLength = 2000
	OfficerTagMaxLength  = 32
	OfficerTagsMax       = 20
	OfficerBatchMax      = 100
)

var LifecycleStatuses = map[string]bool{"applicant": true, "trial": true, "active": true, "inactive": true, "retired": true}

type OfficerCharacter struct {
	Character
	Note            string     `json:"note"`
	LifecycleStatus string     `json:"lifecycleStatus"`
	Tags            []string   `json:"tags"`
	ReviewedAt      *time.Time `json:"reviewedAt"`
	NoteAuthor      *string    `json:"noteAuthor,omitempty"`
}

type OfficerQueueItem struct {
	CharacterID int64     `json:"characterId,omitempty"`
	SyncRunID   *int64    `json:"syncRunId,omitempty"`
	Name        string    `json:"name"`
	Reason      string    `json:"reason"`
	Tags        []string  `json:"tags,omitempty"`
	SyncedAt    time.Time `json:"syncedAt,omitempty"`
}

// BulkValidationError identifies every requested target that cannot be changed.
// Callers must make no changes when this error is returned.
type BulkValidationError struct {
	InvalidCharacterIDs    []int64 `json:"invalidCharacterIds,omitempty"`
	CrossGuildCharacterIDs []int64 `json:"crossGuildCharacterIds,omitempty"`
	InvalidSyncRunIDs      []int64 `json:"invalidSyncRunIds,omitempty"`
	CrossGuildSyncRunIDs   []int64 `json:"crossGuildSyncRunIds,omitempty"`
}

func (e *BulkValidationError) Error() string { return "invalid bulk targets" }

type OfficerActivity struct {
	ID        int64          `json:"id"`
	Actor     string         `json:"actor"`
	Action    string         `json:"action"`
	TargetID  *int64         `json:"targetId,omitempty"`
	CreatedAt time.Time      `json:"createdAt"`
	Changes   map[string]any `json:"changes"`
}
