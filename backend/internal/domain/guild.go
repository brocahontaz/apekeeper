package domain

import "time"

type Guild struct {
	ID                        int64
	Slug, Name, Realm, Region string
	CreatedAt                 time.Time
	SyncSchedule              string
	DiscordWebhookURL         string
}

type GuildMembership struct {
	Guild  Guild
	UserID int64
	Role   string
}
