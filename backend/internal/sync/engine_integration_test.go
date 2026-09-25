package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
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
	seasons               []string
}

func (f *fakeClient) GuildRoster(context.Context, string, string) (dto.GuildRoster, error) {
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
func (f *fakeClient) CharacterProfileSummary(_ context.Context, _, n string) (dto.ProfileSummary, error) {
	if f.profileWait != nil {
		<-f.profileWait
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
	raw := fmt.Sprintf(
		`{"current_mythic_rating":{"rating":%v},"best_runs":[{"mythic_level":%d}]}`,
		f.rating, f.bestLevel,
	)
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return dto.MythicPlus{}, err
	}
	return m, nil
}

func (f *fakeClient) CharacterRaids(_ context.Context, _, n string) (dto.Raids, error) {
	var notFound *blizzard.NotFoundError
	if f.raidsErr != nil && (errors.As(f.raidsErr, &notFound) || n == "Two") {
		return dto.Raids{}, f.raidsErr
	}
	return dto.Raids{}, nil
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
	if len(seasons) != 2 || seasons[0] != "current" || seasons[1] != "current" {
		t.Fatalf("seasons requested=%v, want two requests for \"current\"", seasons)
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
		if m.SeasonSlug != "current" || m.OverallRating != 1500 || m.BestKeyLevel != 10 {
			t.Fatalf("mythic=%+v, want season \"current\" rating 1500 key 10", m)
		}
		snap := d.Snapshots[0]
		if snap.ItemLevel != 621 || snap.MythicRating != 1500 {
			t.Fatalf("snapshot=%+v, want item level 621 rating 1500", snap)
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
