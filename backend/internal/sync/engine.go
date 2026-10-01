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
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Engine struct {
	Client blizzard.BlizzardClient
	Stores store.Store
	// Log is optional; when nil the engine stays silent.
	Log     *slog.Logger
	Now     func() time.Time
	Workers int
	// Progress is an optional live-reporting hook; when nil the engine works
	// unchanged. It fires under the run's counter lock, so implementations
	// must not block and must be safe for concurrent use.
	Progress func(phase domain.Phase, updated, failed, total int)
}

// RunOptions keeps operational modes explicit. DryRun fetches the same
// upstream data but suppresses every character, progression and snapshot write.
type RunOptions struct {
	DryRun bool
	Names  map[string]bool
}

// TierAnchor pins a guild sync to the season and expansion that are current
// when the run starts, so every character records the same tier.
type TierAnchor struct {
	SeasonID    int
	SeasonName  string
	ExpansionID int
}

// resolveTierAnchor looks up the current season and expansion once per run.
// Unavailable indexes degrade gracefully: SeasonID 0 skips mythic+ for the
// run and ExpansionID 0 keeps every raid expansion (previous behavior).
func (e Engine) resolveTierAnchor(ctx context.Context) TierAnchor {
	var a TierAnchor
	if seasons, err := e.Client.MythicKeystoneSeasonIndex(ctx); err != nil {
		if e.Log != nil {
			e.Log.Warn("season index unavailable", "error", err)
		}
	} else {
		a.SeasonID = seasons.CurrentSeason.ID
		a.SeasonName = seasons.CurrentSeason.Name
		if a.SeasonID == 0 {
			for _, s := range seasons.Seasons {
				if s.ID > a.SeasonID {
					a.SeasonID = s.ID
				}
			}
		}
	}
	if journal, err := e.Client.JournalExpansionIndex(ctx); err != nil {
		if e.Log != nil {
			e.Log.Warn("expansion index unavailable", "error", err)
		}
	} else {
		// The journal index repeats current content as a pseudo tier named
		// "Current Season"; it shares the real expansion's ID space and must
		// not win the max.
		for _, t := range journal.Tiers {
			if t.Name == "Current Season" {
				continue
			}
			if t.ID > a.ExpansionID {
				a.ExpansionID = t.ID
			}
		}
		if a.ExpansionID == 0 && e.Log != nil {
			e.Log.Warn("expansion index unavailable")
		}
	}
	return a
}

// SeasonDisplayName extracts the season name from the journal-style
// "Mythic+ Dungeons (X)" label, leaving any other name untouched.
func SeasonDisplayName(name string) string {
	const label = "Mythic+ Dungeons ("
	if strings.HasPrefix(name, label) && strings.HasSuffix(name, ")") {
		return strings.TrimSuffix(strings.TrimPrefix(name, label), ")")
	}
	return name
}

func (e Engine) RunGuildSync(ctx context.Context, g domain.Guild, trigger string) (store.SyncRun, error) {
	run, err := e.Stores.SyncRuns.Create(ctx, g.ID, trigger)
	if err != nil {
		return run, err
	}
	return e.RunGuildSyncWithRun(ctx, g, run)
}

// report forwards live run progress to the optional hook; the engine works
// unchanged when Progress is nil.
func (e Engine) report(phase domain.Phase, updated, failed, total int) {
	if e.Progress != nil {
		e.Progress(phase, updated, failed, total)
	}
}

// RunGuildSyncWithRun completes a sync run which has already been created.
func (e Engine) RunGuildSyncWithRun(ctx context.Context, g domain.Guild, run store.SyncRun) (store.SyncRun, error) {
	return e.RunGuildSyncWithOptions(ctx, g, run, RunOptions{})
}

func (e Engine) RunGuildSyncWithOptions(ctx context.Context, g domain.Guild, run store.SyncRun, opt RunOptions) (store.SyncRun, error) {
	roster, err := e.Client.GuildRoster(ctx, g.Realm, g.Name)
	if err != nil {
		run.Status = domain.RunFailed
		if ctx.Err() != nil {
			run.ErrorSummary = "sync cancelled"
		} else {
			run.ErrorSummary = err.Error()
		}
		if e.Log != nil {
			e.Log.Error("guild roster sync failed", "guild", g.Slug, "name", g.Name, "error", err)
		}
		e.report(domain.PhaseFinalizing, 0, 0, 0)
		if ctx.Err() != nil {
			finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			e.finish(finishCtx, run)
			cancel()
		} else {
			e.finish(ctx, run)
		}
		return run, err
	}
	if len(opt.Names) > 0 {
		members := roster.Members[:0]
		for _, m := range roster.Members {
			if opt.Names[strings.ToLower(m.Character.Name)] {
				members = append(members, m)
			}
		}
		roster.Members = members
	}
	run.Total = len(roster.Members)
	e.report(domain.PhaseRoster, 0, 0, run.Total)
	now := time.Now()
	if e.Now != nil {
		now = e.Now()
	}
	e.report(domain.PhaseTierAnchor, 0, 0, run.Total)
	anchor := e.resolveTierAnchor(ctx)
	e.report(domain.PhaseCharacters, 0, 0, run.Total)
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
		c, er := e.syncCharacter(ctx, g, m, anchor, now, opt.DryRun)
		mu.Lock()
		defer mu.Unlock()
		if er != nil {
			run.Failed++
			details[m.Character.Name] = er.Error()
			if e.Log != nil {
				e.Log.Debug("character sync failed", "character", m.Character.Name, "error", er)
			}
		} else {
			_ = c
			run.Updated++
		}
		// Reported inside the counter critical section so the hook always
		// observes a consistent set of counts.
		e.report(domain.PhaseCharacters, run.Updated, run.Failed, run.Total)
	}
	for i := 0; i < workers; i++ {
		go func() {
			for m := range jobs {
				work(m)
			}
		}()
	}
	wg.Add(len(roster.Members))
	sent := 0
dispatch:
	for _, m := range roster.Members {
		select {
		case jobs <- m:
			sent++
		case <-ctx.Done():
			break dispatch
		}
	}
	// Jobs that were never handed to a worker still own a WaitGroup count.
	// Release them here before closing the queue so cancellation cannot deadlock.
	for i := sent; i < len(roster.Members); i++ {
		wg.Done()
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		run.Status = domain.RunFailed
		run.ErrorSummary = "sync cancelled"
	} else {
		run.Status = domain.Outcome(run.Updated, run.Failed)
	}
	if run.Failed > 0 && ctx.Err() == nil {
		run.ErrorSummary = fmt.Sprintf("%d of %d characters failed", run.Failed, run.Total)
		run.Detail, _ = json.Marshal(details)
	}
	e.report(domain.PhaseFinalizing, run.Updated, run.Failed, run.Total)
	if ctx.Err() != nil {
		finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		err = e.finish(finishCtx, run)
	} else {
		err = e.finish(ctx, run)
	}
	return run, err
}

// finish persists the run outcome and surfaces a discarded store error at Warn.
func (e Engine) finish(ctx context.Context, run store.SyncRun) error {
	err := e.Stores.SyncRuns.Finish(ctx, run)
	if err != nil && e.Log != nil {
		e.Log.Warn("sync run finish failed", "runId", run.ID, "error", err)
	}
	return err
}
func (e Engine) syncCharacter(
	ctx context.Context,
	g domain.Guild,
	m dto.RosterMember,
	anchor TierAnchor,
	now time.Time,
	dryRun bool,
) (domain.Character, error) {
	p, err := e.Client.CharacterProfileSummary(ctx, m.Character.Realm.Slug, m.Character.Name)
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
		ItemLevel:      p.EquippedItemLevel,
		GuildRank:      m.Rank,
		SyncedAt:       now,
	}
	raw, _ := json.Marshal(p)
	if !dryRun {
		c, err = e.Stores.Characters.UpsertByGuildIdentity(ctx, c, raw)
		if err != nil {
			return c, err
		}
	}
	// The avatar is auxiliary: failures never fail the character and never
	// clear a portrait that was stored by an earlier sync.
	media, err := e.Client.CharacterMedia(ctx, m.Character.Realm.Slug, m.Character.Name)
	if err != nil {
		if e.Log != nil {
			e.Log.Debug("character media unavailable", "character", c.Name, "error", err)
		}
	} else if !dryRun {
		for _, a := range media.Assets {
			if a.Key == "avatar" && a.Value != "" {
				if err = e.Stores.Characters.UpdateAvatar(ctx, c.ID, a.Value); err != nil {
					if e.Log != nil {
						e.Log.Debug("character avatar update failed", "character", c.Name, "error", err)
					}
				}
				break
			}
		}
	}
	var mp dto.MythicPlus
	best := 0
	var bestScore float64
	var bestRuns, recentRuns []domain.MythicRun
	var progressionErr error
	if anchor.SeasonID > 0 {
		season := strconv.Itoa(anchor.SeasonID)
		seasonFound, seasonMissing := false, false
		response, err := e.Client.CharacterMythicPlusSeasonal(ctx, m.Character.Realm.Slug, m.Character.Name, season)
		if err != nil {
			if !isNotFound(err) {
				progressionErr = err
			} else {
				seasonMissing = true
				if e.Log != nil {
					e.Log.Debug("mythic keystone profile not found", "character", c.Name)
				}
			}
		} else {
			seasonFound = true
			mp = response
			for _, r := range response.BestRuns {
				bestRuns = append(bestRuns, domain.MythicRun{
					Dungeon:     r.Dungeon.Name,
					Level:       r.KeystoneLevel,
					Score:       r.MythicRating.Rating,
					Timed:       r.Completed,
					CompletedAt: r.CompletedTimestamp,
				})
				if r.KeystoneLevel > best {
					best = r.KeystoneLevel
				}
				if r.MythicRating.Rating > bestScore {
					bestScore = r.MythicRating.Rating
				}
			}
			sort.Slice(bestRuns, func(i, j int) bool {
				if bestRuns[i].Score != bestRuns[j].Score {
					return bestRuns[i].Score > bestRuns[j].Score
				}
				return bestRuns[i].Level > bestRuns[j].Level
			})
		}
		// The index can still carry current-period runs when the season
		// profile 404s, so it is fetched regardless of the season outcome.
		index, err := e.Client.CharacterMythicPlusProfile(ctx, m.Character.Realm.Slug, m.Character.Name)
		if err != nil {
			if !isNotFound(err) {
				if progressionErr == nil {
					progressionErr = err
				}
			} else if e.Log != nil {
				e.Log.Debug("mythic keystone profile index not found", "character", c.Name)
			}
		} else {
			for _, r := range index.CurrentPeriod.BestRuns {
				recentRuns = append(recentRuns, domain.MythicRun{
					Dungeon:     r.Dungeon.Name,
					Level:       r.KeystoneLevel,
					Score:       r.MythicRating.Rating,
					Timed:       r.Completed,
					CompletedAt: r.CompletedTimestamp,
				})
			}
			sort.Slice(recentRuns, func(i, j int) bool {
				if recentRuns[i].CompletedAt != recentRuns[j].CompletedAt {
					return recentRuns[i].CompletedAt > recentRuns[j].CompletedAt
				}
				return recentRuns[i].Level > recentRuns[j].Level
			})
		}
		// A season miss without index runs stores no row, and a failed season
		// fetch must never wipe stored data with a zeroed row. Without season
		// data the rating summary is unknown, so only the run lists are
		// refreshed and a stored row keeps its real rating summary.
		if seasonFound || (seasonMissing && len(recentRuns) > 0) {
			var runs *domain.MythicRuns
			if len(bestRuns) > 0 || len(recentRuns) > 0 {
				runs = &domain.MythicRuns{Best: bestRuns, Recent: recentRuns}
			}
			upsert := domain.MythicPlus{
				CharacterID:   c.ID,
				Season:        SeasonDisplayName(anchor.SeasonName),
				SeasonSlug:    season,
				OverallRating: mp.MythicRating.Rating,
				BestRunScore:  bestScore,
				BestKeyLevel:  best,
				Runs:          runs,
				SyncedAt:      now,
			}
			var err error
			if seasonFound {
				if !dryRun {
					err = e.Stores.Progression.UpsertMythicPlus(ctx, upsert)
				}
			} else {
				if !dryRun {
					err = e.Stores.Progression.UpsertMythicPlusRuns(ctx, upsert)
				}
			}
			if err != nil {
				progressionErr = err
			}
		}
	}
	raids, err := e.Client.CharacterRaids(ctx, m.Character.Realm.Slug, m.Character.Name)
	var raidJSON = []byte("[]")
	if err != nil {
		if !isNotFound(err) {
			if progressionErr == nil {
				progressionErr = err
			}
		} else if e.Log != nil {
			e.Log.Debug("character raids not found", "character", c.Name)
		}
	} else {
		raidJSON, _ = json.Marshal(raids)
		var rows []domain.RaidProgression
		for _, ex := range raids.Expansions {
			if anchor.ExpansionID > 0 && ex.ID != anchor.ExpansionID {
				continue
			}
			for _, r := range ex.Instances {
				for _, mode := range r.Modes {
					rows = append(rows, domain.RaidProgression{
						CharacterID: c.ID,
						RaidSlug:    r.Instance.Slug,
						RaidName:    r.Instance.Name,
						Difficulty:  mode.Difficulty.Name,
						Progress:    mode.Progress.CompletedCount,
						TotalBosses: mode.Progress.TotalCount,
						Summary:     raidJSON,
						SyncedAt:    now,
					})
				}
			}
		}
		if !dryRun {
			err = e.Stores.Progression.ReplaceRaids(ctx, c.ID, rows)
		}
		if err != nil && progressionErr == nil {
			progressionErr = err
		}
	}
	// Once the character is upserted, progression failures remain per-character
	// failures but do not discard the profile data that a snapshot can preserve.
	if !dryRun {
		if err = e.capture(ctx, c, domain.Snapshot{
			CharacterID:  c.ID,
			CapturedAt:   now,
			ItemLevel:    c.ItemLevel,
			MythicRating: mp.MythicRating.Rating,
			BestKeyLevel: best,
			RaidProgress: raidJSON,
		}); err != nil {
			return c, err
		}
	}
	return c, progressionErr
}

func isNotFound(err error) bool {
	var notFound *blizzard.NotFoundError
	return errors.As(err, &notFound)
}
