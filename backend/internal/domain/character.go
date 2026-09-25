package domain

import "time"

const CurrentMaxLevel = 70

type Character struct {
	ID             int64
	GuildID        int64
	Name           string
	DisplayName    string
	NormalizedName string
	Realm          string
	RealmSlug      string
	Region         string
	ClassName      string
	SpecName       string
	ClassID        int
	SpecID         int
	Level          int
	GuildRank      int
	ItemLevel      float64
	MythicRating   float64
	BestKeyLevel   int
	SyncedAt       time.Time
}

func (c Character) IsStale(asOf time.Time, maxAge time.Duration) bool {
	return c.SyncedAt.IsZero() || asOf.Sub(c.SyncedAt) > maxAge
}
func (c Character) ActiveCharacter(asOf time.Time, maxAge time.Duration) bool {
	return c.Level >= CurrentMaxLevel && !c.IsStale(asOf, maxAge)
}

var ClassNames = map[int]string{
	1:  "Warrior",
	2:  "Paladin",
	3:  "Hunter",
	4:  "Rogue",
	5:  "Priest",
	6:  "Death Knight",
	7:  "Shaman",
	8:  "Mage",
	9:  "Warlock",
	10: "Monk",
	11: "Druid",
	12: "Demon Hunter",
	13: "Evoker",
}
