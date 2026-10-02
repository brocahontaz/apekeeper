package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
)

func TestOfficerWorkflowIsGuildScopedAndIdempotent(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.New(t)
	s := store.New(pool)
	g, _ := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "officer-a", Name: "A", Realm: "Area 52", Region: "us"})
	other, _ := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "officer-b", Name: "B", Realm: "Area 52", Region: "us"})
	actor, err := s.Users.UpsertBattleNetUser(ctx, "officer", "Officer", "", "", time.Now().Add(time.Hour), false)
	if err != nil {
		t.Fatal(err)
	}
	makeCharacter := func(guild int64, name string) domain.Character {
		c, e := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{GuildID: guild, Name: name, DisplayName: name, NormalizedName: name, Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: time.Now()}, []byte("{}"))
		if e != nil {
			t.Fatal(e)
		}
		return c
	}
	one, two := makeCharacter(g.ID, "One"), makeCharacter(other.ID, "Two")
	if err := s.Officer.BulkTags(ctx, g.ID, actor.ID, []int64{one.ID}, []string{"Raider", "raider"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Officer.BulkTags(ctx, g.ID, actor.ID, []int64{one.ID}, []string{"raider"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := s.Officer.BulkTags(ctx, g.ID, actor.ID, []int64{two.ID}, []string{"nope"}, nil); err == nil {
		t.Fatal("cross-guild bulk update succeeded")
	}
	if err := s.Officer.Complete(ctx, g.ID, actor.ID, []int64{one.ID}, nil); err != nil {
		t.Fatal(err)
	}
	var failedRunID int64
	if err := pool.QueryRow(ctx, `INSERT INTO sync_runs(guild_id,trigger,status,characters_failed,started_at) VALUES($1,'manual','partial',1,now()) RETURNING id`, g.ID).Scan(&failedRunID); err != nil {
		t.Fatal(err)
	}
	failures, err := s.Officer.Queue(ctx, g.ID, "recent_sync_failure", time.Now())
	if err != nil || len(failures) != 1 || failures[0].SyncRunID == nil || *failures[0].SyncRunID != failedRunID {
		t.Fatalf("failed sync queue=%+v err=%v", failures, err)
	}
	if err := s.Officer.Complete(ctx, g.ID, actor.ID, nil, []int64{failedRunID}); err != nil {
		t.Fatal(err)
	}
	failures, err = s.Officer.Queue(ctx, g.ID, "recent_sync_failure", time.Now())
	if err != nil || len(failures) != 0 {
		t.Fatalf("completed failed sync remains queued: %+v err=%v", failures, err)
	}
	items, err := s.Officer.Queue(ctx, g.ID, "missing_progression", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].CharacterID != one.ID || len(items[0].Tags) != 1 || items[0].Tags[0] != "raider" {
		t.Fatalf("queue=%+v", items)
	}
	activity, err := s.Officer.Activity(ctx, g.ID, 10)
	if err != nil || len(activity) != 4 {
		t.Fatalf("activity=%+v err=%v", activity, err)
	}
	if err := s.Officer.Update(ctx, g.ID, actor.ID, one.ID, string(make([]byte, domain.OfficerNoteMaxLength+1)), "", nil); err == nil {
		t.Fatal("oversize note accepted")
	}
}

func TestOfficerMetadataValidationAndRollback(t *testing.T) {
	ctx := context.Background()
	pool, _ := testdb.New(t)
	s := store.New(pool)
	g, _ := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "officer-meta", Name: "A", Realm: "Area 52", Region: "us"})
	other, _ := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "officer-meta-other", Name: "B", Realm: "Area 52", Region: "us"})
	actor, err := s.Users.UpsertBattleNetUser(ctx, "author", "Note Author", "", "", time.Now().Add(time.Hour), false)
	if err != nil {
		t.Fatal(err)
	}
	makeCharacter := func(guildID int64, name string) domain.Character {
		c, err := s.Characters.UpsertByGuildIdentity(ctx, domain.Character{GuildID: guildID, Name: name, DisplayName: name, NormalizedName: name, Realm: "Area 52", RealmSlug: "area-52", Region: "us", SyncedAt: time.Now()}, []byte("{}"))
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	character, outsider := makeCharacter(g.ID, "Meta"), makeCharacter(other.ID, "Outsider")
	if err := s.Officer.Update(ctx, g.ID, actor.ID, character.ID, "private note", "active", []string{"Core-Team"}); err != nil {
		t.Fatal(err)
	}
	metadata, err := s.Officer.Character(ctx, g.ID, character.ID)
	if err != nil || metadata.Note != "private note" || metadata.NoteAuthor == nil || *metadata.NoteAuthor != "Note Author" || metadata.LifecycleStatus != "active" || len(metadata.Tags) != 1 || metadata.Tags[0] != "core-team" {
		t.Fatalf("metadata=%+v err=%v", metadata, err)
	}
	if _, err := s.Officer.Character(ctx, g.ID, outsider.ID); err == nil {
		t.Fatal("cross-guild metadata read succeeded")
	}
	for _, tags := range [][]string{{"bad tag"}, {"bad\nname"}, {strings.Repeat("a", domain.OfficerTagMaxLength+1)}} {
		if err := s.Officer.Update(ctx, g.ID, actor.ID, character.ID, "", "", tags); err == nil {
			t.Fatalf("invalid tags accepted: %q", tags)
		}
	}
	tooManyTags := make([]string, domain.OfficerTagsMax+1)
	for i := range tooManyTags {
		tooManyTags[i] = "tag" + string(rune('a'+i))
	}
	if err := s.Officer.Update(ctx, g.ID, actor.ID, character.ID, "", "", tooManyTags); err == nil {
		t.Fatal("too many tags accepted")
	}
	if err := s.Officer.Update(ctx, g.ID, actor.ID, character.ID, "", "invalid", nil); err == nil {
		t.Fatal("invalid status accepted")
	}
	if err := s.Officer.Complete(ctx, g.ID, actor.ID, nil, nil); err == nil {
		t.Fatal("empty batch accepted")
	}
	ids := make([]int64, domain.OfficerBatchMax+1)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	if err := s.Officer.Complete(ctx, g.ID, actor.ID, ids, nil); err == nil {
		t.Fatal("oversize batch accepted")
	}
	if err := s.Officer.BulkTags(ctx, g.ID, 999999, []int64{character.ID}, []string{"rollback"}, nil); err == nil {
		t.Fatal("invalid audit actor accepted")
	}
	metadata, err = s.Officer.Character(ctx, g.ID, character.ID)
	if err != nil || strings.Contains(strings.Join(metadata.Tags, ","), "rollback") {
		t.Fatalf("transaction did not roll back: %+v err=%v", metadata, err)
	}
	activity, err := s.Officer.Activity(ctx, g.ID, 10)
	if err != nil || len(activity) != 1 || activity[0].Changes["noteChanged"] != true {
		t.Fatalf("unsafe or missing activity=%+v err=%v", activity, err)
	}
	statusChanges := activity[0].Changes["lifecycleStatus"].(map[string]any)
	tagChanges := activity[0].Changes["tags"].(map[string]any)
	if statusChanges["before"] != "" || statusChanges["after"] != "active" || len(tagChanges["after"].([]any)) != 1 {
		t.Fatalf("incomplete safe before/after audit: %v", activity[0].Changes)
	}
	if _, containsNote := activity[0].Changes["note"]; containsNote {
		t.Fatalf("note content leaked to audit: %v", activity[0].Changes)
	}
}
