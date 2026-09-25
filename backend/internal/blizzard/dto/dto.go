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
	MythicKeystoneProfile struct {
		CurrentPeriod struct {
			ID int `json:"id"`
		} `json:"current_period"`
	} `json:"mythic_keystone_profile"`
}
type Equipment struct {
	EquippedItemLevel float64 `json:"equipped_item_level"`
}
type MythicPlus struct {
	CurrentMythicRating struct {
		Rating float64 `json:"rating"`
	} `json:"current_mythic_rating"`
	BestRuns []struct {
		MythicLevel int `json:"mythic_level"`
		Map         struct {
			Name string `json:"name"`
		} `json:"map"`
		Score float64 `json:"score"`
	} `json:"best_runs"`
}
type Raids struct {
	Expansions []struct {
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
type Token struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
}
type UserProfile struct {
	ID        int64  `json:"id"`
	BattleTag string `json:"battletag"`
}
