package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard"
	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

type logCapture struct {
	mu      sync.Mutex
	records []slog.Record
}

func (c *logCapture) Enabled(context.Context, slog.Level) bool { return true }
func (c *logCapture) Handle(_ context.Context, r slog.Record) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.records = append(c.records, r.Clone())
	return nil
}
func (c *logCapture) WithAttrs([]slog.Attr) slog.Handler { return c }
func (c *logCapture) WithGroup(string) slog.Handler      { return c }

func (c *logCapture) find(level slog.Level, msg string) (slog.Record, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range c.records {
		if r.Level == level && r.Message == msg {
			return r, true
		}
	}
	return slog.Record{}, false
}

func (c *logCapture) attr(level slog.Level, msg, key string) (slog.Value, bool) {
	r, ok := c.find(level, msg)
	if !ok {
		return slog.Value{}, false
	}
	var found slog.Value
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			found = a.Value
			return false
		}
		return true
	})
	return found, true
}

type fakeClient struct {
	mu                    sync.Mutex
	rating, ilvl          float64
	bestLevel             int
	mediaURL              string
	rosterErr, profileErr error
	mythicErr, raidsErr   error
	mediaErr              error
	profileWait           <-chan struct{}
	rosterWait            <-chan struct{}
	seasons               []string
	profileCalls          int
	profileIndex          dto.MythicPlusProfileIndex
	profileIndexErr       error
	seasonIndex           dto.MythicKeystoneSeasonIndex
	seasonIndexErr        error
	journalIndex          dto.JournalExpansionIndex
	journalErr            error
	raidsResponse         dto.Raids
}

func (f *fakeClient) GuildRoster(ctx context.Context, _ string, _ string) (dto.GuildRoster, error) {
	if f.rosterWait != nil {
		select {
		case <-f.rosterWait:
		case <-ctx.Done():
			return dto.GuildRoster{}, ctx.Err()
		}
	}
	if f.rosterErr != nil {
		return dto.GuildRoster{}, f.rosterErr
	}
	var r dto.GuildRoster
	for _, n := range []string{"One", "Two"} {
		m := dto.RosterMember{}
		m.Character.Name = n
		m.Character.Realm.Name = "Area 52"
		m.Character.Realm.Slug = "area-52"
		r.Members = append(r.Members, m)
	}
	return r, nil
}
func (f *fakeClient) CharacterProfileSummary(ctx context.Context, _, n string) (dto.ProfileSummary, error) {
	if f.profileWait != nil {
		select {
		case <-f.profileWait:
		case <-ctx.Done():
			return dto.ProfileSummary{}, ctx.Err()
		}
	}
	if f.profileErr != nil && n == "Two" {
		return dto.ProfileSummary{}, f.profileErr
	}
	p := dto.ProfileSummary{Name: n, Level: 70, EquippedItemLevel: f.ilvl}
	p.Realm = dto.Name{Name: "Area 52", Slug: "area-52"}
	p.CharacterClass.ID = 8
	p.CharacterClass.Name = "Mage"
	p.ActiveSpec.ID = 62
	p.ActiveSpec.Name = "Arcane"
	return p, nil
}
func (f *fakeClient) CharacterMythicPlusSeasonal(_ context.Context, _, _, season string) (dto.MythicPlus, error) {
	f.mu.Lock()
	f.seasons = append(f.seasons, season)
	f.mu.Unlock()
	if f.mythicErr != nil {
		return dto.MythicPlus{}, f.mythicErr
	}
	var m dto.MythicPlus
	// Two runs with distinct levels, scores, and timestamps; the lower level
	// carries the higher score, so score ordering is observable in storage.
	raw := fmt.Sprintf(
		`{"season":{"id":18},"mythic_rating":{"rating":%v},"best_runs":[`+
			`{"keystone_level":%d,"dungeon":{"name":"Voidscar Arena"},"mythic_rating":{"rating":%v},`+
			`"completed_timestamp":1700000001000,"is_completed_within_time":false},`+
			`{"keystone_level":%d,"dungeon":{"name":"Apex Canopy"},"mythic_rating":{"rating":%v},`+
			`"completed_timestamp":1700000000000,"is_completed_within_time":true}]}`,
		f.rating, f.bestLevel, f.rating+10, f.bestLevel/2, f.rating+30,
	)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return dto.MythicPlus{}, err
	}
	return m, nil
}

func (f *fakeClient) CharacterMythicPlusProfile(context.Context, string, string) (dto.MythicPlusProfileIndex, error) {
	f.mu.Lock()
	f.profileCalls++
	f.mu.Unlock()
	if f.profileIndexErr != nil {
		return dto.MythicPlusProfileIndex{}, f.profileIndexErr
	}
	return f.profileIndex, nil
}

func (f *fakeClient) MythicKeystoneSeasonIndex(context.Context) (dto.MythicKeystoneSeasonIndex, error) {
	if f.seasonIndexErr != nil {
		return dto.MythicKeystoneSeasonIndex{}, f.seasonIndexErr
	}
	idx := f.seasonIndex
	if idx.CurrentSeason.ID == 0 && len(idx.Seasons) == 0 {
		// The season the other fakes imitate, so tests resolve a real anchor
		// unless they configure the index themselves.
		if err := json.Unmarshal([]byte(
			`{"current_season":{"id":18,"name":"Mythic+ Dungeons (Midnight Season 2)"}}`), &idx); err != nil {
			return dto.MythicKeystoneSeasonIndex{}, err
		}
	}
	return idx, nil
}

func (f *fakeClient) JournalExpansionIndex(context.Context) (dto.JournalExpansionIndex, error) {
	return f.journalIndex, f.journalErr
}

func (f *fakeClient) CharacterRaids(_ context.Context, _, n string) (dto.Raids, error) {
	var notFound *blizzard.NotFoundError
	if f.raidsErr != nil && (errors.As(f.raidsErr, &notFound) || n == "Two") {
		return dto.Raids{}, f.raidsErr
	}
	return f.raidsResponse, nil
}

func (f *fakeClient) CharacterMedia(context.Context, string, string) (dto.CharacterMedia, error) {
	if f.mediaErr != nil {
		return dto.CharacterMedia{}, f.mediaErr
	}
	var m dto.CharacterMedia
	if f.mediaURL != "" {
		m.Assets = []dto.MediaAsset{{Key: "avatar", Value: f.mediaURL}}
	}
	return m, nil
}

func TestEngineCapturesProfileWhenProgressionIsNotFound(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{
		ilvl:      600,
		mythicErr: &blizzard.NotFoundError{URL: "mythic"},
		raidsErr:  &blizzard.NotFoundError{URL: "raids"},
	})
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 || r.Failed != 0 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Mythic) != 0 || len(d.Raids) != 0 ||
			len(d.Snapshots) != 1 || d.Snapshots[0].MythicRating != 0 || d.Snapshots[0].BestKeyLevel != 0 {
			t.Fatalf("detail=%+v err=%v", d, err)
		}
	}
}

func TestEngineCapturesProfileOnProgressionFailure(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{ilvl: 600, raidsErr: errors.New("raids unavailable")})
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunPartial || r.Updated != 1 || r.Failed != 1 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Snapshots) != 1 {
			t.Fatalf("detail=%+v err=%v", d, err)
		}
	}
}
func (*fakeClient) ExchangeCode(context.Context, string) (dto.Token, error) {
	return dto.Token{}, nil
}
func (*fakeClient) UserProfile(context.Context, string) (dto.UserProfile, error) {
	return dto.UserProfile{}, nil
}
func (*fakeClient) AuthorizationURL(string, string) string {
	return ""
}

var _ blizzard.BlizzardClient = &fakeClient{}

func testEngine(t *testing.T, f *fakeClient) (Engine, store.Store, domain.Guild) {
	t.Helper()
	p, _ := testdb.New(t)
	s := store.New(p)
	g, err := s.Guilds.EnsureGuild(context.Background(), domain.Guild{
		Slug:   "sync-test",
		Name:   "Ape",
		Realm:  "Area 52",
		Region: "us",
	})
	if err != nil {
		t.Fatal(err)
	}
	return Engine{
		Client:  f,
		Stores:  s,
		Workers: 1,
		Now: func() time.Time {
			return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		},
	}, s, g
}
func TestEnginePersistsSyncAndChangedSnapshots(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{rating: 100, ilvl: 600})
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 || chars[0].ClassName != "Mage" || chars[0].ItemLevel != 600 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	d, err := s.Characters.Detail(context.Background(), g.ID, chars[0].ID, time.Time{})
	if err != nil || len(d.Mythic) != 1 || len(d.Snapshots) != 1 {
		t.Fatalf("detail=%+v err=%v", d, err)
	}
	e.Client = &fakeClient{rating: 200, ilvl: 610}
	e.Now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	r, err = e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Updated != 2 {
		t.Fatalf("second run=%+v err=%v", r, err)
	}
	d, err = s.Characters.Detail(context.Background(), g.ID, chars[0].ID, time.Time{})
	if err != nil || len(d.Snapshots) != 2 {
		t.Fatalf("changed snapshots=%d err=%v", len(d.Snapshots), err)
	}
}

func TestEngineDryRunFetchesWithoutWritingGuildData(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{rating: 100, ilvl: 600})
	if run, err := e.RunGuildSync(context.Background(), g, "manual"); err != nil || run.Status != domain.RunSuccess {
		t.Fatalf("seed run=%+v err=%v", run, err)
	}
	before, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(before) != 2 {
		t.Fatalf("seed characters=%+v err=%v", before, err)
	}
	beforeDetails := make([]store.CharacterDetail, len(before))
	for i, c := range before {
		beforeDetails[i], err = s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
	}
	e.Client = &fakeClient{rating: 999, ilvl: 700}
	run, err := e.Stores.SyncRuns.Create(context.Background(), g.ID, "dry-run")
	if err != nil {
		t.Fatal(err)
	}
	run, err = e.RunGuildSyncWithOptions(context.Background(), g, run, RunOptions{DryRun: true})
	if err != nil || run.Status != domain.RunSuccess || run.Updated != 2 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || !reflect.DeepEqual(chars, before) {
		t.Fatalf("dry run changed characters=%+v, want %+v (err=%v)", chars, before, err)
	}
	for i, c := range chars {
		detail, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || !reflect.DeepEqual(detail, beforeDetails[i]) {
			t.Fatalf("dry run changed progression or snapshots: detail=%+v want=%+v err=%v", detail, beforeDetails[i], err)
		}
	}
	history, err := s.SyncRuns.History(context.Background(), g.ID, 1)
	if err != nil || len(history) != 1 || history[0].Trigger != "dry-run" || history[0].Status != domain.RunSuccess {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestEngineCancelledRunSettlesWithoutDispatchingWorkers(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{ilvl: 600})
	run, err := s.SyncRuns.Create(context.Background(), g.ID, "manual")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan store.SyncRun, 1)
	go func() { got, _ := e.RunGuildSyncWithRun(ctx, g, run); done <- got }()
	select {
	case got := <-done:
		if got.Status != domain.RunFailed || got.ErrorSummary != "sync cancelled" {
			t.Fatalf("run=%+v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled sync did not settle")
	}
}

func TestEngineSyncsCurrentSeasonMythicPlus(t *testing.T) {
	f := &fakeClient{rating: 1500, ilvl: 621, bestLevel: 10}
	e, s, g := testEngine(t, f)
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	f.mu.Lock()
	seasons := append([]string(nil), f.seasons...)
	f.mu.Unlock()
	if len(seasons) != 2 || seasons[0] != "18" || seasons[1] != "18" {
		t.Fatalf("seasons requested=%v, want two requests for \"18\"", seasons)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		if c.ItemLevel != 621 {
			t.Fatalf("character %s item level=%v, want 621 from profile", c.Name, c.ItemLevel)
		}
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Mythic) != 1 || len(d.Snapshots) != 1 {
			t.Fatalf("detail=%+v err=%v", d, err)
		}
		m := d.Mythic[0]
		if m.SeasonSlug != "18" || m.Season != "Midnight Season 2" || m.OverallRating != 1500 || m.BestKeyLevel != 10 {
			t.Fatalf("mythic=%+v, want season \"Midnight Season 2\" (slug 18) rating 1500 key 10", m)
		}
		snap := d.Snapshots[0]
		if snap.ItemLevel != 621 || snap.MythicRating != 1500 {
			t.Fatalf("snapshot=%+v, want item level 621 rating 1500", snap)
		}
	}
}

// profileIndexFixture decodes a current_period payload from the keystone
// profile index response shape.
func profileIndexFixture(t *testing.T, runs string) dto.MythicPlusProfileIndex {
	t.Helper()
	var x dto.MythicPlusProfileIndex
	if err := json.Unmarshal([]byte(`{"current_period":{"period":{"id":1082},"best_runs":`+runs+`}}`), &x); err != nil {
		t.Fatal(err)
	}
	return x
}

// The season fake lists its runs out of score order and the index fake lists
// its runs out of timestamp order, so both stored orderings are observable.
func TestEngineStoresMythicRuns(t *testing.T) {
	f := &fakeClient{rating: 1500, ilvl: 621, bestLevel: 10}
	f.profileIndex = profileIndexFixture(t, `[
		{"completed_timestamp":1700000005000,"keystone_level":12,"dungeon":{"name":"Cinderbrew Meadery"},"mythic_rating":{"rating":120},"is_completed_within_time":true},
		{"completed_timestamp":1700000009000,"keystone_level":11,"dungeon":{"name":"Darkflame Cleft"},"mythic_rating":{"rating":95},"is_completed_within_time":false}
	]`)
	e, s, g := testEngine(t, f)
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Mythic) != 1 {
			t.Fatalf("character %s detail=%+v err=%v", c.Name, d, err)
		}
		runs := d.Mythic[0].Runs
		if runs == nil {
			t.Fatalf("character %s runs=nil, want best and recent runs", c.Name)
		}
		wantBest := []domain.MythicRun{
			{Dungeon: "Apex Canopy", Level: 5, Score: 1530, Timed: true, CompletedAt: 1700000000000},
			{Dungeon: "Voidscar Arena", Level: 10, Score: 1510, Timed: false, CompletedAt: 1700000001000},
		}
		if !reflect.DeepEqual(runs.Best, wantBest) {
			t.Fatalf("character %s best=%+v, want %+v", c.Name, runs.Best, wantBest)
		}
		wantRecent := []domain.MythicRun{
			{Dungeon: "Darkflame Cleft", Level: 11, Score: 95, Timed: false, CompletedAt: 1700000009000},
			{Dungeon: "Cinderbrew Meadery", Level: 12, Score: 120, Timed: true, CompletedAt: 1700000005000},
		}
		if !reflect.DeepEqual(runs.Recent, wantRecent) {
			t.Fatalf("character %s recent=%+v, want %+v", c.Name, runs.Recent, wantRecent)
		}
		if d.Mythic[0].BestRunScore != 1530 {
			t.Fatalf("character %s best run score=%v, want 1530", c.Name, d.Mythic[0].BestRunScore)
		}
	}
}

// A season 404 is benign, and an index that still carries current-period runs
// must store them in a zeroed season row rather than losing them.
func TestEngineStoresRecentRunsWhenSeasonIsNotFound(t *testing.T) {
	f := &fakeClient{ilvl: 600, mythicErr: &blizzard.NotFoundError{URL: "mythic"}}
	f.profileIndex = profileIndexFixture(t, `[
		{"completed_timestamp":1700000007000,"keystone_level":12,"dungeon":{"name":"Cinderbrew Meadery"},"mythic_rating":{"rating":110},"is_completed_within_time":true}
	]`)
	e, s, g := testEngine(t, f)
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Failed != 0 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Mythic) != 1 {
			t.Fatalf("character %s detail=%+v err=%v", c.Name, d, err)
		}
		m := d.Mythic[0]
		if m.OverallRating != 0 || m.BestKeyLevel != 0 || m.BestRunScore != 0 {
			t.Fatalf("character %s mythic=%+v, want zeroed season summary", c.Name, m)
		}
		if m.Runs == nil || len(m.Runs.Best) != 0 || len(m.Runs.Recent) != 1 ||
			m.Runs.Recent[0].Dungeon != "Cinderbrew Meadery" || m.Runs.Recent[0].Level != 12 {
			t.Fatalf("character %s runs=%+v, want only the recent run", c.Name, m.Runs)
		}
	}
}

// A later sync whose season endpoint 404s must keep the stored rating summary
// while still landing the current-period runs the index reports.
func TestEngineKeepsStoredSummaryWhenSeasonIsNotFound(t *testing.T) {
	f := &fakeClient{ilvl: 600, mythicErr: &blizzard.NotFoundError{URL: "mythic"}}
	f.profileIndex = profileIndexFixture(t, `[
		{"completed_timestamp":1700000007000,"keystone_level":12,"dungeon":{"name":"Cinderbrew Meadery"},"mythic_rating":{"rating":110},"is_completed_within_time":true}
	]`)
	e, s, g := testEngine(t, f)
	// Seed the row a previous sync stored for roster member "One".
	c, err := s.Characters.UpsertByGuildIdentity(context.Background(), domain.Character{
		GuildID:        g.ID,
		Name:           "One",
		DisplayName:    "One",
		NormalizedName: "one",
		Realm:          "Area 52",
		RealmSlug:      "area-52",
		Region:         "us",
	}, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Progression.UpsertMythicPlus(context.Background(), domain.MythicPlus{
		CharacterID:   c.ID,
		Season:        "Midnight Season 2",
		SeasonSlug:    "18",
		OverallRating: 2500,
		BestKeyLevel:  12,
		BestRunScore:  500,
		SyncedAt:      time.Date(2025, 12, 1, 0, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Failed != 0 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	wantRuns := &domain.MythicRuns{Recent: []domain.MythicRun{
		{Dungeon: "Cinderbrew Meadery", Level: 12, Score: 110, Timed: true, CompletedAt: 1700000007000},
	}}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Mythic) != 1 {
			t.Fatalf("character %s detail=%+v err=%v", c.Name, d, err)
		}
		m := d.Mythic[0]
		if c.Name == "One" {
			// The zeroed season write must not clobber the stored summary.
			if m.OverallRating != 2500 || m.BestKeyLevel != 12 || m.BestRunScore != 500 {
				t.Fatalf("character %s mythic=%+v, want the stored rating summary preserved", c.Name, m)
			}
		} else if m.OverallRating != 0 || m.BestKeyLevel != 0 || m.BestRunScore != 0 {
			t.Fatalf("character %s mythic=%+v, want a zeroed summary on fresh insert", c.Name, m)
		}
		if !reflect.DeepEqual(m.Runs, wantRuns) {
			t.Fatalf("character %s runs=%+v, want the refreshed recent runs", c.Name, m.Runs)
		}
	}
}

// An index 404 is benign: the season row is stored with its best runs only
// and the run still succeeds.
func TestEngineStoresBestRunsWhenIndexIsNotFound(t *testing.T) {
	capture := &logCapture{}
	e, s, g := testEngine(t, &fakeClient{
		rating:          1500,
		ilvl:            621,
		bestLevel:       10,
		profileIndexErr: &blizzard.NotFoundError{URL: "profile index"},
	})
	e.Log = slog.New(capture)
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Failed != 0 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	if _, ok := capture.find(slog.LevelDebug, "mythic keystone profile index not found"); !ok {
		t.Error("missing mythic keystone profile index not found record")
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || len(d.Mythic) != 1 {
			t.Fatalf("character %s detail=%+v err=%v", c.Name, d, err)
		}
		m := d.Mythic[0]
		if m.Runs == nil || len(m.Runs.Best) != 2 || m.Runs.Recent != nil {
			t.Fatalf("character %s runs=%+v, want best runs only", c.Name, m.Runs)
		}
	}
}

func TestSeasonDisplayName(t *testing.T) {
	for name, want := range map[string]string{
		"Mythic+ Dungeons (Midnight Season 2)": "Midnight Season 2",
		"Some Raid Tier":                       "Some Raid Tier",
		"":                                     "",
	} {
		if got := SeasonDisplayName(name); got != want {
			t.Errorf("SeasonDisplayName(%q) = %q, want %q", name, got, want)
		}
	}
}

// raidsFixture decodes an expansions array from the raids response shape.
func raidsFixture(t *testing.T, expansions string) dto.Raids {
	t.Helper()
	var r dto.Raids
	if err := json.Unmarshal([]byte(`{"expansions":`+expansions+`}`), &r); err != nil {
		t.Fatal(err)
	}
	return r
}

// journalFixture decodes a tiers array from the journal expansion index shape.
func journalFixture(t *testing.T, tiers string) dto.JournalExpansionIndex {
	t.Helper()
	var j dto.JournalExpansionIndex
	if err := json.Unmarshal([]byte(`{"tiers":`+tiers+`}`), &j); err != nil {
		t.Fatal(err)
	}
	return j
}

// raidRows renders stored progression rows as a sorted, comparable form.
func raidRows(rows []domain.RaidProgression) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.RaidSlug+"|"+r.RaidName+"|"+r.Difficulty+"|"+
			strconv.Itoa(r.Progress)+"/"+strconv.Itoa(r.TotalBosses))
	}
	sort.Strings(out)
	return out
}

func TestEngineSyncsOnlyCurrentExpansionRaids(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{
		ilvl: 600,
		raidsResponse: raidsFixture(t, `[
			{"id":395,"instances":[{"instance":{"name":"Emerald Nightmare","slug":"emerald-nightmare"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":7,"total_count":7}}]}]},
			{"id":514,"instances":[{"instance":{"name":"Nerub-ar Palace","slug":"nerubar-palace"},"modes":[{"difficulty":{"name":"Heroic"},"progress":{"completed_count":8,"total_count":8}}]}]},
			{"id":516,"instances":[{"instance":{"name":"Midnight Raid One","slug":"midnight-raid-one"},"modes":[{"difficulty":{"name":"Raid Finder"},"progress":{"completed_count":1,"total_count":4}}]},{"instance":{"name":"Midnight Raid Two","slug":"midnight-raid-two"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":2,"total_count":5}},{"difficulty":{"name":"Heroic"},"progress":{"completed_count":1,"total_count":5}}]}]}
		]`),
		journalIndex: journalFixture(t, `[
			{"id":74,"name":"Mists of Pandaria"},
			{"id":505,"name":"Current Season"},
			{"id":514,"name":"The War Within"},
			{"id":516,"name":"Midnight"}
		]`),
	})
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	want := []string{
		"midnight-raid-one|Midnight Raid One|Raid Finder|1/4",
		"midnight-raid-two|Midnight Raid Two|Heroic|1/5",
		"midnight-raid-two|Midnight Raid Two|Normal|2/5",
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if got := raidRows(d.Raids); !reflect.DeepEqual(got, want) {
			t.Fatalf("character %s raids=%v, want only the Midnight rows %v", c.Name, got, want)
		}
	}
}

func TestEngineReplacesStaleRaidRows(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{
		ilvl: 600,
		raidsResponse: raidsFixture(t, `[
			{"id":516,"instances":[{"instance":{"name":"First Current Raid","slug":"first-current-raid"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":1,"total_count":4}}]}]}
		]`),
		journalIndex: journalFixture(t, `[{"id":516,"name":"Midnight"}]`),
	})
	if r, err := e.RunGuildSync(context.Background(), g, "manual"); err != nil || r.Status != domain.RunSuccess {
		t.Fatalf("first run=%+v err=%v", r, err)
	}
	// A later sync sees different current-tier instances plus a legacy tier.
	e.Now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	e.Client = &fakeClient{
		ilvl: 601,
		raidsResponse: raidsFixture(t, `[
			{"id":395,"instances":[{"instance":{"name":"Emerald Nightmare","slug":"emerald-nightmare"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":7,"total_count":7}}]}]},
			{"id":516,"instances":[{"instance":{"name":"Second Current Raid","slug":"second-current-raid"},"modes":[{"difficulty":{"name":"Heroic"},"progress":{"completed_count":2,"total_count":4}}]}]}
		]`),
		journalIndex: journalFixture(t, `[{"id":516,"name":"Midnight"}]`),
	}
	if r, err := e.RunGuildSync(context.Background(), g, "manual"); err != nil || r.Status != domain.RunSuccess {
		t.Fatalf("second run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	want := []string{"second-current-raid|Second Current Raid|Heroic|2/4"}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if got := raidRows(d.Raids); !reflect.DeepEqual(got, want) {
			t.Fatalf("character %s raids=%v, want exactly the replacement set %v", c.Name, got, want)
		}
	}
}

// A failed raids lookup, including the benign 404, must never clear rows a
// previous sync stored: ReplaceRaids only runs on a successful response.
func TestEngineRaidFailurePreservesStoredRows(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{
		ilvl: 600,
		raidsResponse: raidsFixture(t, `[
			{"id":516,"instances":[{"instance":{"name":"Midnight Raid","slug":"midnight-raid"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":1,"total_count":4}}]}]}
		]`),
		journalIndex: journalFixture(t, `[{"id":516,"name":"Midnight"}]`),
	})
	if r, err := e.RunGuildSync(context.Background(), g, "manual"); err != nil || r.Status != domain.RunSuccess {
		t.Fatalf("first run=%+v err=%v", r, err)
	}
	// A later sync cannot see any raid data: the lookup is not found.
	e.Now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	e.Client = &fakeClient{
		ilvl:     600,
		raidsErr: &blizzard.NotFoundError{URL: "raids"},
	}
	if r, err := e.RunGuildSync(context.Background(), g, "manual"); err != nil || r.Status != domain.RunSuccess {
		t.Fatalf("second run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	want := []string{"midnight-raid|Midnight Raid|Normal|1/4"}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if got := raidRows(d.Raids); !reflect.DeepEqual(got, want) {
			t.Fatalf("character %s raids=%v, want the stored rows preserved %v", c.Name, got, want)
		}
	}
}

func TestEngineAnchorFailureKeepsRaidsAndSkipsMythic(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{
		ilvl:           600,
		seasonIndexErr: errors.New("season index down"),
		journalErr:     errors.New("journal index down"),
		raidsResponse: raidsFixture(t, `[
			{"id":395,"instances":[{"instance":{"name":"Emerald Nightmare","slug":"emerald-nightmare"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":7,"total_count":7}}]}]},
			{"id":516,"instances":[{"instance":{"name":"Midnight Raid","slug":"midnight-raid"},"modes":[{"difficulty":{"name":"Normal"},"progress":{"completed_count":1,"total_count":4}}]}]}
		]`),
	})
	capture := &logCapture{}
	e.Log = slog.New(capture)
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	if _, ok := capture.find(slog.LevelWarn, "season index unavailable"); !ok {
		t.Error("missing season index unavailable record")
	}
	if _, ok := capture.find(slog.LevelWarn, "expansion index unavailable"); !ok {
		t.Error("missing expansion index unavailable record")
	}
	f := e.Client.(*fakeClient)
	f.mu.Lock()
	seasons := append([]string(nil), f.seasons...)
	profileCalls := f.profileCalls
	f.mu.Unlock()
	if len(seasons) != 0 {
		t.Fatalf("seasons requested=%v, want none when the season index fails", seasons)
	}
	if profileCalls != 0 {
		t.Fatalf("profile index calls=%d, want none when the season index fails", profileCalls)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	want := []string{
		"emerald-nightmare|Emerald Nightmare|Normal|7/7",
		"midnight-raid|Midnight Raid|Normal|1/4",
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if len(d.Mythic) != 0 {
			t.Fatalf("character %s mythic=%+v, want no mythic row without a season anchor", c.Name, d.Mythic)
		}
		if got := raidRows(d.Raids); !reflect.DeepEqual(got, want) {
			t.Fatalf("character %s raids=%v, want every expansion %v", c.Name, got, want)
		}
	}
}
func TestEngineSyncsCharacterAvatar(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{
		rating:   100,
		ilvl:     600,
		mediaURL: "https://render.worldofwarcraft.com/us/ape-avatar.jpg",
	})
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 || r.Failed != 0 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	chars, err := s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if d.AvatarURL != "https://render.worldofwarcraft.com/us/ape-avatar.jpg" {
			t.Fatalf("character %s avatar=%q, want the media URL", c.Name, d.AvatarURL)
		}
	}

	// Media failures are auxiliary: the run still succeeds and no stale
	// portrait is written.
	e, s, g = testEngine(t, &fakeClient{
		rating:   100,
		ilvl:     600,
		mediaErr: &blizzard.NotFoundError{URL: "media"},
	})
	r, err = e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 || r.Failed != 0 {
		t.Fatalf("failed-media run=%+v err=%v", r, err)
	}
	chars, err = s.Characters.ListByGuild(context.Background(), g.ID, store.CharacterFilter{})
	if err != nil || len(chars) != 2 {
		t.Fatalf("characters=%+v err=%v", chars, err)
	}
	for _, c := range chars {
		d, err := s.Characters.Detail(context.Background(), g.ID, c.ID, time.Time{})
		if err != nil || d.AvatarURL != "" {
			t.Fatalf("character %s avatar=%q err=%v, want empty", c.Name, d.AvatarURL, err)
		}
	}
}
func TestEngineRecordsFailures(t *testing.T) {
	e, _, g := testEngine(t, &fakeClient{profileErr: &blizzard.NotFoundError{URL: "test"}, ilvl: 600})
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunPartial || r.Failed != 1 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	e, _, g = testEngine(t, &fakeClient{rosterErr: os.ErrNotExist})
	r, err = e.RunGuildSync(context.Background(), g, "manual")
	if err == nil || r.Status != domain.RunFailed || r.ErrorSummary == "" {
		t.Fatalf("run=%+v err=%v", r, err)
	}
}

// progressEvent is one optional-hook observation: the phase plus the counts
// the run had settled at when it fired.
type progressEvent struct {
	phase                  domain.Phase
	updated, failed, total int
}

// progressCapture records hook calls from every worker goroutine under a
// lock, so the reported order and counts stay comparable after the run.
type progressCapture struct {
	mu     sync.Mutex
	events []progressEvent
}

func (c *progressCapture) add(phase domain.Phase, updated, failed, total int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, progressEvent{phase, updated, failed, total})
}

func (c *progressCapture) got() []progressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]progressEvent(nil), c.events...)
}

// The optional progress hook observes the run's phases in order with the
// roster total and per-character counts; leaving it nil keeps the engine
// unchanged, which every other test in this package exercises.
func TestEngineReportsProgressPhases(t *testing.T) {
	e, _, g := testEngine(t, &fakeClient{ilvl: 600})
	capture := &progressCapture{}
	e.Progress = capture.add
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err != nil || r.Status != domain.RunSuccess || r.Updated != 2 {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	events := capture.got()
	phases := make([]domain.Phase, 0, len(events))
	for _, ev := range events {
		phases = append(phases, ev.phase)
	}
	// One worker, two characters: the anchor resolves once and each character
	// completion reports under the run's counter lock.
	want := []domain.Phase{
		domain.PhaseRoster,
		domain.PhaseTierAnchor,
		domain.PhaseCharacters,
		domain.PhaseCharacters,
		domain.PhaseCharacters,
		domain.PhaseFinalizing,
	}
	if !reflect.DeepEqual(phases, want) {
		t.Fatalf("phases=%v, want %v", phases, want)
	}
	if events[0].total != 2 || events[0].updated != 0 || events[0].failed != 0 {
		t.Fatalf("roster event=%+v, want the full total with no counts", events[0])
	}
	last := events[len(events)-1]
	if last.updated != 2 || last.failed != 0 || last.total != 2 {
		t.Fatalf("finalizing event=%+v, want the settled outcome", last)
	}
}

func TestServiceLogsRunStartAndCompletion(t *testing.T) {
	e, _, g := testEngine(t, &fakeClient{ilvl: 600})
	capture := &logCapture{}
	e.Log = slog.New(capture)
	service := Service{Engine: e, Guild: g, Log: slog.New(capture)}
	r, err := service.Start(context.Background(), "manual")
	if err != nil || r.Status != domain.RunSuccess {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	if _, ok := capture.find(slog.LevelInfo, "sync run started"); !ok {
		t.Error("missing sync run started record")
	}
	if _, ok := capture.find(slog.LevelInfo, "sync run completed"); !ok {
		t.Fatal("missing sync run completed record")
	}
	if status, ok := capture.attr(slog.LevelInfo, "sync run completed", "status"); !ok || status.String() != string(domain.RunSuccess) {
		t.Errorf("completed record status = %v, want %q", status, domain.RunSuccess)
	}
	if runID, ok := capture.attr(slog.LevelInfo, "sync run completed", "runId"); !ok || runID.Int64() != r.ID {
		t.Errorf("completed record runId = %v, want %d", runID, r.ID)
	}
}

func TestEngineLogsRosterFailure(t *testing.T) {
	e, _, g := testEngine(t, &fakeClient{rosterErr: os.ErrNotExist})
	capture := &logCapture{}
	e.Log = slog.New(capture)
	r, err := e.RunGuildSync(context.Background(), g, "manual")
	if err == nil || r.Status != domain.RunFailed {
		t.Fatalf("run=%+v err=%v", r, err)
	}
	if _, ok := capture.find(slog.LevelError, "guild roster sync failed"); !ok {
		t.Error("missing guild roster sync failed record")
	}
	if _, ok := capture.find(slog.LevelWarn, "sync run finish failed"); ok {
		t.Error("unexpected sync run finish failed record")
	}
}
