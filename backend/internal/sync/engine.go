package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"sync"
	"time"
)

const DefaultSeasonSlug = "current"

type Engine struct {
	Client  blizzard.BlizzardClient
	Stores  store.Store
	Now     func() time.Time
	Workers int
}

func (e Engine) RunGuildSync(ctx context.Context, g domain.Guild, trigger string) (store.SyncRun, error) {
	run, err := e.Stores.SyncRuns.Create(ctx, g.ID, trigger)
	if err != nil {
		return run, err
	}
	return e.RunGuildSyncWithRun(ctx, g, run)
}

// RunGuildSyncWithRun completes a sync run which has already been created.
func (e Engine) RunGuildSyncWithRun(ctx context.Context, g domain.Guild, run store.SyncRun) (store.SyncRun, error) {
	roster, err := e.Client.GuildRoster(ctx, g.Realm, g.Name)
	if err != nil {
		run.Status = domain.RunFailed
		run.ErrorSummary = err.Error()
		_ = e.Stores.SyncRuns.Finish(ctx, run)
		return run, err
	}
	run.Total = len(roster.Members)
	now := time.Now()
	if e.Now != nil {
		now = e.Now()
	}
	workers := e.Workers
	if workers <= 0 {
		workers = 8
	}
	jobs := make(chan dto.RosterMember)
	var mu sync.Mutex
	details := map[string]string{}
	var wg sync.WaitGroup
	work := func(m dto.RosterMember) {
		defer wg.Done()
		c, er := e.syncCharacter(ctx, g, m, now)
		mu.Lock()
		defer mu.Unlock()
		if er != nil {
			run.Failed++
			details[m.Character.Name] = er.Error()
		} else {
			_ = c
			run.Updated++
		}
	}
	for i := 0; i < workers; i++ {
		go func() {
			for m := range jobs {
				work(m)
			}
		}()
	}
	wg.Add(len(roster.Members))
	for _, m := range roster.Members {
		jobs <- m
	}
	close(jobs)
	wg.Wait()
	run.Status = domain.Outcome(run.Updated, run.Failed)
	if run.Failed > 0 {
		run.ErrorSummary = fmt.Sprintf("%d of %d characters failed", run.Failed, run.Total)
		run.Detail, _ = json.Marshal(details)
	}
	err = e.Stores.SyncRuns.Finish(ctx, run)
	return run, err
}
func (e Engine) syncCharacter(
	ctx context.Context,
	g domain.Guild,
	m dto.RosterMember,
	now time.Time,
) (domain.Character, error) {
	p, err := e.Client.CharacterProfileSummary(ctx, m.Character.Realm.Slug, m.Character.Name)
	if err != nil {
		return domain.Character{}, err
	}
	eq, err := e.Client.CharacterEquipment(ctx, m.Character.Realm.Slug, m.Character.Name)
	if err != nil {
		return domain.Character{}, err
	}
	c := domain.Character{
		GuildID:        g.ID,
		Name:           p.Name,
		DisplayName:    p.Name,
		NormalizedName: domain.NormalizeCharacterName(p.Name),
		Realm:          p.Realm.Name,
		RealmSlug:      p.Realm.Slug,
		Region:         g.Region,
		ClassID:        p.CharacterClass.ID,
		ClassName:      p.CharacterClass.Name,
		SpecID:         p.ActiveSpec.ID,
		SpecName:       p.ActiveSpec.Name,
		Level:          p.Level,
		ItemLevel:      eq.EquippedItemLevel,
		GuildRank:      m.Rank,
		SyncedAt:       now,
	}
	raw, _ := json.Marshal(p)
	c, err = e.Stores.Characters.UpsertByGuildIdentity(ctx, c, raw)
	if err != nil {
		return c, err
	}
	season := DefaultSeasonSlug
	if p.MythicKeystoneProfile.CurrentPeriod.ID > 0 {
		season = fmt.Sprint(p.MythicKeystoneProfile.CurrentPeriod.ID)
	}
	mp, err := e.Client.CharacterMythicPlusSeasonal(ctx, m.Character.Realm.Slug, m.Character.Name, season)
	best := 0
	var progressionErr error
	if err != nil {
		if !isNotFound(err) {
			progressionErr = err
		}
	} else {
		for _, r := range mp.BestRuns {
			if r.MythicLevel > best {
				best = r.MythicLevel
			}
		}
		if err = e.Stores.Progression.UpsertMythicPlus(ctx, domain.MythicPlus{
			CharacterID:   c.ID,
			SeasonSlug:    season,
			OverallRating: mp.CurrentMythicRating.Rating,
			BestKeyLevel:  best,
			SyncedAt:      now,
		}); err != nil {
			progressionErr = err
		}
	}
	raids, err := e.Client.CharacterRaids(ctx, m.Character.Realm.Slug, m.Character.Name)
	var raidJSON = []byte("[]")
	if err != nil {
		if !isNotFound(err) && progressionErr == nil {
			progressionErr = err
		}
	} else {
		raidJSON, _ = json.Marshal(raids)
		for _, ex := range raids.Expansions {
			for _, r := range ex.Instances {
				for _, mode := range r.Modes {
					if err = e.Stores.Progression.UpsertRaid(ctx, domain.RaidProgression{
						CharacterID: c.ID,
						RaidSlug:    r.Instance.Slug,
						RaidName:    r.Instance.Name,
						Difficulty:  mode.Difficulty.Name,
						Progress:    mode.Progress.CompletedCount,
						TotalBosses: mode.Progress.TotalCount,
						Summary:     raidJSON,
						SyncedAt:    now,
					}); err != nil && progressionErr == nil {
						progressionErr = err
					}
				}
			}
		}
	}
	// Once the character is upserted, progression failures remain per-character
	// failures but do not discard the profile data that a snapshot can preserve.
	if err = e.capture(ctx, c, domain.Snapshot{
		CharacterID:  c.ID,
		CapturedAt:   now,
		ItemLevel:    c.ItemLevel,
		MythicRating: mp.CurrentMythicRating.Rating,
		BestKeyLevel: best,
		RaidProgress: raidJSON,
	}); err != nil {
		return c, err
	}
	return c, progressionErr
}

func isNotFound(err error) bool {
	var notFound *blizzard.NotFoundError
	return errors.As(err, &notFound)
}
