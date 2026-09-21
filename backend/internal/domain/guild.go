package domain

import "time"

type Guild struct {
	ID                        int64
	Slug, Name, Realm, Region string
	CreatedAt                 time.Time
}
