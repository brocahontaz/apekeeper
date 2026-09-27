package dto

type Name struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}
type GuildRoster struct {
	Members []RosterMember `json:"members"`
}
type RosterMember struct {
	Character Character `json:"character"`
	Rank      int       `json:"rank"`
}
type Character struct {
	Name          string `json:"name"`
	Realm         Name   `json:"realm"`
	Level         int    `json:"level"`
	PlayableClass struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"playable_class"`
}
type ProfileSummary struct {
	Name       string `json:"name"`
	Realm      Name   `json:"realm"`
	Level      int    `json:"level"`
	ActiveSpec struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"active_spec"`
	CharacterClass struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"character_class"`
	EquippedItemLevel float64 `json:"equipped_item_level"`
	AverageItemLevel  float64 `json:"average_item_level"`
}

// MythicRunEntry is the run shape shared by the season profile and the
// keystone profile index endpoints.
type MythicRunEntry struct {
	CompletedTimestamp int64 `json:"completed_timestamp"`
	KeystoneLevel      int   `json:"keystone_level"`
	Dungeon            struct {
		Name string `json:"name"`
	} `json:"dungeon"`
	MythicRating struct {
		Rating float64 `json:"rating"`
	} `json:"mythic_rating"`
	Completed bool `json:"is_completed_within_time"`
}
type MythicPlus struct {
	Season struct {
		ID int `json:"id"`
	} `json:"season"`
	MythicRating struct {
		Rating float64 `json:"rating"`
	} `json:"mythic_rating"`
	BestRuns []MythicRunEntry `json:"best_runs"`
}
type MythicPlusProfileIndex struct {
	CurrentPeriod struct {
		Period struct {
			ID int `json:"id"`
		} `json:"period"`
		BestRuns []MythicRunEntry `json:"best_runs"`
	} `json:"current_period"`
}
type MythicKeystoneSeasonIndex struct {
	CurrentSeason struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"current_season"`
	Seasons []struct {
		ID int `json:"id"`
	} `json:"seasons"`
}
type JournalExpansionIndex struct {
	Tiers []struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	} `json:"tiers"`
}
type Raids struct {
	Expansions []struct {
		ID        int `json:"id"`
		Instances []struct {
			Instance Name `json:"instance"`
			Modes    []struct {
				Difficulty struct {
					Name string `json:"name"`
				} `json:"difficulty"`
				Progress struct {
					CompletedCount int `json:"completed_count"`
					TotalCount     int `json:"total_count"`
				} `json:"progress"`
			} `json:"modes"`
		} `json:"instances"`
	} `json:"expansions"`
}
type MediaAsset struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type CharacterMedia struct {
	Assets []MediaAsset `json:"assets"`
}
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}
type UserProfile struct {
	ID        int64  `json:"id"`
	BattleTag string `json:"battletag"`
}
