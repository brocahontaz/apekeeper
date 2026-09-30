package httpapi

import (
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
)

// rosterExport streams the whole roster as a CSV download. It carries the
// same auth guard as the roster page and reuses the roster list query without
// pagination, so every row is exported.
func (a API) rosterExport(w http.ResponseWriter, r *http.Request) {
	g, e := a.guild(r.Context())
	if e != nil {
		fail(w, 500, "guild lookup failed")
		return
	}
	chars, e := a.Stores.Characters.ListByGuild(r.Context(), g.ID, store.CharacterFilter{})
	if e != nil {
		fail(w, 500, "roster lookup failed")
		return
	}
	name := fmt.Sprintf("roster-%s-%s.csv", g.Slug, a.now().UTC().Format("20060102"))
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, name))
	// Response errors cannot change the status once the CSV headers are sent.
	_ = rosterCSV(w, chars, a.now())
}

func rosterCSVHeader() []string {
	return []string{"Name", "Realm", "Class", "Spec", "Level", "Guild Rank", "Item Level",
		"M+ Rating", "Best Key", "Stale", "Last Seen"}
}

// rosterCSVRow renders one character as a CSV record. An unset spec and a
// never-synced last seen become empty fields; encoding/csv quotes fields
// containing separators, quotes, or newlines.
func rosterCSVRow(c domain.Character, now time.Time) []string {
	lastSeen := ""
	// The list queries coalesce a missing synced_at to the Unix epoch, which
	// means "never seen" and exports as an empty field.
	if c.SyncedAt.After(time.Unix(0, 0)) {
		lastSeen = c.SyncedAt.Format(time.RFC3339)
	}
	return []string{
		c.DisplayName,
		c.Realm,
		c.ClassName,
		c.SpecName,
		strconv.Itoa(c.Level),
		strconv.Itoa(c.GuildRank),
		strconv.FormatFloat(c.ItemLevel, 'f', -1, 64),
		strconv.FormatFloat(c.MythicRating, 'f', -1, 64),
		strconv.Itoa(c.BestKeyLevel),
		strconv.FormatBool(c.IsStale(now, 7*24*time.Hour)),
		lastSeen,
	}
}

func rosterCSV(w io.Writer, chars []domain.Character, now time.Time) error {
	out := csv.NewWriter(w)
	if err := out.Write(rosterCSVHeader()); err != nil {
		return err
	}
	for _, c := range chars {
		if err := out.Write(rosterCSVRow(c, now)); err != nil {
			return err
		}
	}
	out.Flush()
	return out.Error()
}
