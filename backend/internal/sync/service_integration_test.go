package sync

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/brocahontaz/apekeeper/backend/internal/blizzard/dto"
	"github.com/brocahontaz/apekeeper/backend/internal/domain"
	"github.com/brocahontaz/apekeeper/backend/internal/notify"
	"github.com/brocahontaz/apekeeper/backend/internal/sched"
	"github.com/brocahontaz/apekeeper/backend/internal/store"
	"github.com/brocahontaz/apekeeper/backend/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestServiceStartManualCreatesRunningRunBeforeQueueing(t *testing.T) {
	blocked := make(chan struct{})
	e, s, g := testEngine(t, &fakeClient{ilvl: 600, profileWait: blocked})
	service := Service{Engine: e, Guild: g}
	run, err := service.StartManual(context.Background())
	if err != nil || run.ID == 0 || run.Status != domain.RunRunning {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	history, err := s.SyncRuns.History(context.Background(), g.ID, 1)
	if err != nil || len(history) != 1 || history[0].ID != run.ID || history[0].Status != domain.RunRunning {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if _, err := service.StartManual(context.Background()); err != ErrAlreadyRunning {
		t.Fatalf("concurrent manual sync error=%v", err)
	}
	close(blocked)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		history, err = s.SyncRuns.History(context.Background(), g.ID, 1)
		if err == nil && len(history) == 1 && history[0].Status == domain.RunSuccess {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("run did not complete: history=%+v err=%v", history, err)
}

func TestScheduledGuildServicesUseTheirOwnGuildAndConfiguration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p, _ := testdb.New(t)
	stores := store.New(p)
	firstGuild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "scheduled-one", Name: "One", Realm: "Area 52", Region: "us", SyncSchedule: "23:59"})
	if err != nil {
		t.Fatal(err)
	}
	secondGuild, err := stores.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "scheduled-two", Name: "Two", Realm: "Tichondrius", Region: "eu", SyncSchedule: "23:59"})
	if err != nil {
		t.Fatal(err)
	}
	first := &Service{Engine: Engine{Client: &fakeClient{ilvl: 601}, Stores: stores, Workers: 1}, Guild: firstGuild}
	second := &Service{Engine: Engine{Client: &fakeClient{ilvl: 602}, Stores: stores, Workers: 1}, Guild: secondGuild}
	firstScheduler := sched.New(firstGuild.SyncSchedule, slog.New(slog.DiscardHandler))
	secondScheduler := sched.New(secondGuild.SyncSchedule, slog.New(slog.DiscardHandler))
	go firstScheduler.Run(ctx, func(c context.Context) { _, _ = first.Start(c, "scheduled") })
	go secondScheduler.Run(ctx, func(c context.Context) { _, _ = second.Start(c, "scheduled") })
	firstScheduler.Trigger()
	secondScheduler.Trigger()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		one, oneErr := stores.SyncRuns.History(ctx, firstGuild.ID, 1)
		two, twoErr := stores.SyncRuns.History(ctx, secondGuild.ID, 1)
		if oneErr == nil && twoErr == nil && len(one) == 1 && len(two) == 1 && one[0].Status == domain.RunSuccess && two[0].Status == domain.RunSuccess {
			charsOne, err := stores.Characters.ListByGuild(ctx, firstGuild.ID, store.CharacterFilter{})
			if err != nil {
				t.Fatal(err)
			}
			charsTwo, err := stores.Characters.ListByGuild(ctx, secondGuild.ID, store.CharacterFilter{})
			if err != nil {
				t.Fatal(err)
			}
			if len(charsOne) != 2 || len(charsTwo) != 2 || charsOne[0].ItemLevel != 601 || charsTwo[0].ItemLevel != 602 {
				t.Fatalf("guild services mixed configuration: one=%+v two=%+v", charsOne, charsTwo)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	one, _ := stores.SyncRuns.History(ctx, firstGuild.ID, 1)
	two, _ := stores.SyncRuns.History(ctx, secondGuild.ID, 1)
	t.Fatalf("scheduled guild services did not complete independently: one=%+v two=%+v", one, two)
}

// recordingNotifier captures the summary the service hands to a notifier and
// replies with a configurable outcome; NotifySyncRun runs in its own
// goroutine, so access is locked.
type recordingNotifier struct {
	mu      sync.Mutex
	got     notify.SyncRunSummary
	hits    int
	outcome notify.Outcome
}

type blockingNotifier struct {
	started chan<- struct{}
	release <-chan struct{}
}

func (n *blockingNotifier) NotifySyncRun(notify.SyncRunSummary) notify.Outcome {
	n.started <- struct{}{}
	<-n.release
	return notify.Outcome{Status: notify.OutcomeSent}
}

func (r *recordingNotifier) NotifySyncRun(s notify.SyncRunSummary) notify.Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = s
	r.hits++
	return r.outcome
}

func (r *recordingNotifier) summary() (notify.SyncRunSummary, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.got, r.hits
}

// closingNotifier tears the engine's pool down mid-notification, forcing the
// outcome persistence that follows to fail against a dead store.
type closingNotifier struct {
	pool *pgxpool.Pool
}

func (c *closingNotifier) NotifySyncRun(notify.SyncRunSummary) notify.Outcome {
	c.pool.Close()
	return notify.Outcome{Status: notify.OutcomeSent}
}

// assertNotifyStatus polls until the run's persisted notify status matches
// want; the fan-out goroutine lands it asynchronously after Start returns.
func assertNotifyStatus(t *testing.T, s store.Store, guildID, runID int64, want string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		history, err := s.SyncRuns.History(context.Background(), guildID, 10)
		if err == nil {
			for _, r := range history {
				if r.ID == runID && r.NotifyStatus != nil && *r.NotifyStatus == want {
					return
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("notify status %q never landed for run %d", want, runID)
}

// Every finished run — manual or scheduled — must fire one notification
// summarizing the outcome; without a notifier nothing fires at all.
func TestServiceNotifiesAfterEveryRunCompletes(t *testing.T) {
	e, _, g := testEngine(t, &fakeClient{ilvl: 600})
	notifier := &recordingNotifier{}
	service := Service{Engine: e, Guild: g, Notifier: notifier}
	run, err := service.Start(context.Background(), "manual")
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		got, hits := notifier.summary()
		if hits > 1 {
			t.Fatalf("notification fired %d times, want one per run", hits)
		}
		if hits == 1 {
			if got.Trigger != "manual" || got.Status != string(domain.RunSuccess) ||
				got.Total != run.Total || got.Updated != run.Updated || got.Failed != 0 ||
				got.Guild != g.Name || got.Duration <= 0 || len(got.Reasons) != 0 {
				t.Fatalf("summary=%+v run=%+v", got, run)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("no notification arrived; summary=%+v", notifier.got)
}

// runFailureReasons flattens the stored per-character detail into a sorted
// reason list and falls back to the overall error summary.
func TestRunFailureReasons(t *testing.T) {
	run := store.SyncRun{
		ErrorSummary: "2 of 3 characters failed",
		Detail:       []byte(`{"Alpha":"profile unavailable","Beta":"raids unavailable"}`),
	}
	reasons := runFailureReasons(run)
	if len(reasons) != 2 || reasons[0] != "Alpha: profile unavailable" || reasons[1] != "Beta: raids unavailable" {
		t.Fatalf("reasons=%q, want the sorted detail entries", reasons)
	}
	empty := runFailureReasons(store.SyncRun{})
	if len(empty) != 0 {
		t.Fatalf("reasons=%q, want none", empty)
	}
	overall := runFailureReasons(store.SyncRun{ErrorSummary: "roster unavailable"})
	if len(overall) != 1 || overall[0] != "roster unavailable" {
		t.Fatalf("reasons=%q, want the overall summary", overall)
	}
}

// The notification outcome is pure bookkeeping: each returned outcome must
// land in sync_runs and surface through History without ever touching the
// run's own status or result.
func TestServicePersistsNotifyOutcome(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{ilvl: 600})
	notifier := &recordingNotifier{outcome: notify.Outcome{Status: notify.OutcomeSent}}
	service := Service{Engine: e, Guild: g, Notifier: notifier}
	first, err := service.Start(context.Background(), "manual")
	if err != nil || first.Status != domain.RunSuccess {
		t.Fatalf("run=%+v err=%v", first, err)
	}
	assertNotifyStatus(t, s, g.ID, first.ID, notify.OutcomeSent)
	history, err := s.SyncRuns.History(context.Background(), g.ID, 10)
	if err != nil || len(history) != 1 ||
		history[0].ID != first.ID || history[0].Status != domain.RunSuccess ||
		history[0].NotifyStatus == nil || *history[0].NotifyStatus != notify.OutcomeSent {
		t.Fatalf("history=%+v err=%v", history, err)
	}

	// A failed delivery outcome is recorded the same way, and the run that
	// triggered it still finished successfully. The engine clock advances so
	// the second run's snapshots get fresh capture timestamps.
	service.Engine.Now = func() time.Time { return time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) }
	notifier.mu.Lock()
	notifier.outcome = notify.Outcome{Status: notify.OutcomeFailed, Detail: "timeout"}
	notifier.mu.Unlock()
	second, err := service.Start(context.Background(), "manual")
	if err != nil || second.Status != domain.RunSuccess {
		t.Fatalf("second run=%+v err=%v", second, err)
	}
	assertNotifyStatus(t, s, g.ID, second.ID, notify.OutcomeFailed)
	history, err = s.SyncRuns.History(context.Background(), g.ID, 10)
	if err != nil || len(history) != 2 {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if got := history[0].NotifyStatus; got == nil || *got != notify.OutcomeFailed {
		t.Fatalf("newest run notifyStatus=%v, want failed", got)
	}
	if got := history[1].NotifyStatus; got == nil || *got != notify.OutcomeSent {
		t.Fatalf("older run notifyStatus=%v, want sent", got)
	}
}

// Failing notification bookkeeping must stay a logged warning: the run keeps
// its stored result, the notify status stays absent, and nothing panics.
func TestServiceNotifyPersistFailureIsLoggedNotFatal(t *testing.T) {
	ctx := context.Background()
	pool, dbURL := testdb.New(t)
	s := store.New(pool)
	g, err := s.Guilds.EnsureGuild(ctx, domain.Guild{Slug: "notify-persist", Name: "Ape", Realm: "Area 52", Region: "us"})
	if err != nil {
		t.Fatal(err)
	}
	// A second pool outlives the engine's so the assertions survive the
	// forced store failure.
	watch, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(watch.Close)

	capture := &logCapture{}
	service := Service{
		Engine: Engine{Client: &fakeClient{ilvl: 600}, Stores: s, Workers: 1},
		Guild:  g,
		Log:    slog.New(capture),
		// The notifier closes the pool before its outcome is persisted.
		Notifier: &closingNotifier{pool: pool},
	}
	run, err := service.Start(ctx, "manual")
	if err != nil || run.Status != domain.RunSuccess {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, ok := capture.find(slog.LevelWarn, "sync notification status persist failed"); ok {
			history, err := store.New(watch).SyncRuns.History(ctx, g.ID, 10)
			if err != nil || len(history) != 1 ||
				history[0].ID != run.ID || history[0].Status != domain.RunSuccess || history[0].NotifyStatus != nil {
				t.Fatalf("history=%+v err=%v, want the run result untouched with no notify status", history, err)
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("missing sync notification status persist failed record")
}

// The live progress snapshot starts queued while the engine is still
// reaching the roster, tracks the characters phase with the full total while
// a run is blocked, and settles into a terminal, inactive snapshot that
// keeps the final counts visible for the next poll.
func TestServiceReportsLiveProgress(t *testing.T) {
	ctx := context.Background()
	blockedRoster := make(chan struct{})
	blockedProfile := make(chan struct{})
	e, _, g := testEngine(t, &fakeClient{
		ilvl:        600,
		rosterWait:  blockedRoster,
		profileWait: blockedProfile,
		profileErr:  errors.New("profile unavailable"),
	})
	service := Service{Engine: e, Guild: g}
	service.Engine.Progress = service.ReportProgress

	run, err := service.StartManual(ctx)
	if err != nil || run.ID == 0 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	p := service.Progress()
	if !p.Active || p.Status != domain.RunRunning || p.Phase != domain.PhaseQueued ||
		p.RunID != run.ID || p.StartedAt.IsZero() || p.Total != 0 {
		t.Fatalf("queued progress=%+v", p)
	}

	close(blockedRoster)
	// Once the roster lands the run blocks on the first profile: characters
	// phase with the whole crew still unprocessed.
	deadline := time.Now().Add(time.Second)
	for {
		p = service.Progress()
		if p.Phase == domain.PhaseCharacters && p.Total == 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("characters phase never observed; progress=%+v", p)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !p.Active || p.Status != domain.RunRunning || p.Updated != 0 || p.Failed != 0 {
		t.Fatalf("blocked progress=%+v", p)
	}

	close(blockedProfile)
	deadline = time.Now().Add(time.Second)
	for {
		p = service.Progress()
		if !p.Active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run never went terminal; progress=%+v", p)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if p.Status != domain.RunPartial || p.Phase != domain.PhaseFinalizing ||
		p.Updated != 1 || p.Failed != 1 || p.Total != 2 || p.RunID != run.ID {
		t.Fatalf("terminal progress=%+v, want partial 1 updated 1 failed of 2", p)
	}
}

// A second trigger while the first is still blocked must stay
// ErrAlreadyRunning and must not disturb the live snapshot the first run is
// reporting.
func TestServiceProgressRejectsConcurrentRun(t *testing.T) {
	ctx := context.Background()
	blockedProfile := make(chan struct{})
	e, _, g := testEngine(t, &fakeClient{ilvl: 600, profileWait: blockedProfile})
	service := Service{Engine: e, Guild: g}
	service.Engine.Progress = service.ReportProgress
	run, err := service.StartManual(ctx)
	if err != nil || run.ID == 0 {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	if _, err := service.Start(ctx, "scheduled"); err != ErrAlreadyRunning {
		t.Fatalf("concurrent start error=%v, want ErrAlreadyRunning", err)
	}
	p := service.Progress()
	if !p.Active || p.RunID != run.ID {
		t.Fatalf("progress=%+v, want the first run still live", p)
	}
	close(blockedProfile)
}

// A scheduled run is synchronous for the scheduler, but it is still an
// admitted service run: cancellation must reach its engine context and settle
// the ledger rather than only working for HTTP-started asynchronous runs.
func TestServiceCancelStopsScheduledRun(t *testing.T) {
	blockedRoster := make(chan struct{})
	e, s, g := testEngine(t, &fakeClient{ilvl: 600, rosterWait: blockedRoster})
	service := Service{Engine: e, Guild: g}
	result := make(chan struct {
		run store.SyncRun
		err error
	}, 1)
	go func() {
		run, err := service.Start(context.Background(), "scheduled")
		result <- struct {
			run store.SyncRun
			err error
		}{run, err}
	}()

	deadline := time.Now().Add(time.Second)
	for !service.Progress().Active {
		if time.Now().After(deadline) {
			t.Fatal("scheduled run was not admitted")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if err := service.Cancel(); err != nil {
		t.Fatalf("Cancel() error=%v", err)
	}
	select {
	case got := <-result:
		if got.run.Status != domain.RunFailed || got.run.ErrorSummary != "sync cancelled" || got.err == nil {
			t.Fatalf("cancelled scheduled run=%+v err=%v", got.run, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("scheduled run did not stop after cancellation")
	}
	history, err := s.SyncRuns.History(context.Background(), g.ID, 1)
	if err != nil || len(history) != 1 || history[0].Status != domain.RunFailed || history[0].ErrorSummary != "sync cancelled" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

// uncooperativeClient simulates an upstream call that does not honor context
// cancellation, proving Shutdown waits for the service's engine goroutine.
type uncooperativeClient struct {
	*fakeClient
	release <-chan struct{}
}

func (c *uncooperativeClient) GuildRoster(context.Context, string, string) (dto.GuildRoster, error) {
	<-c.release
	return c.fakeClient.GuildRoster(context.Background(), "", "")
}

func TestServiceShutdownWaitsForActiveRun(t *testing.T) {
	release := make(chan struct{})
	f := &fakeClient{ilvl: 600}
	e, s, g := testEngine(t, f)
	e.Client = &uncooperativeClient{fakeClient: f, release: release}
	service := Service{Engine: e, Guild: g}
	run, err := service.StartManual(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	shutdown := make(chan error, 1)
	go func() { shutdown <- service.Shutdown(context.Background()) }()
	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned before the engine stopped: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatalf("Shutdown() error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not wait for the engine to settle")
	}
	history, err := s.SyncRuns.History(context.Background(), g.ID, 1)
	if err != nil || len(history) != 1 || history[0].ID != run.ID || history[0].Status != domain.RunFailed {
		t.Fatalf("history=%+v err=%v", history, err)
	}
}

func TestServiceShutdownWaitsForInFlightNotification(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	e, _, g := testEngine(t, &fakeClient{ilvl: 600})
	service := Service{
		Engine:   e,
		Guild:    g,
		Notifier: &blockingNotifier{started: started, release: release},
	}
	if _, err := service.Start(context.Background(), "manual"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("notification did not start")
	}

	shutdown := make(chan error, 1)
	go func() { shutdown <- service.Shutdown(context.Background()) }()
	select {
	case err := <-shutdown:
		t.Fatalf("Shutdown returned before notification delivery finished: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatalf("Shutdown() error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Shutdown did not wait for notification delivery")
	}
}

func TestServiceRetryRecordsRepeatedCharacterFailure(t *testing.T) {
	e, s, g := testEngine(t, &fakeClient{ilvl: 600, profileErr: errors.New("profile unavailable")})
	service := Service{Engine: e, Guild: g}
	source, err := service.Start(context.Background(), "manual")
	if err != nil || source.Status != domain.RunPartial || source.Failed != 1 {
		t.Fatalf("source=%+v err=%v", source, err)
	}
	service.Engine.Client = &fakeClient{ilvl: 600, profileErr: errors.New("profile unavailable")}
	retry, err := service.StartRetry(context.Background(), source.ID)
	if err != nil || retry.Trigger != "retry" || retry.Status != domain.RunRunning {
		t.Fatalf("retry=%+v err=%v", retry, err)
	}
	deadline := time.Now().Add(time.Second)
	var history []store.SyncRun
	for {
		history, err = s.SyncRuns.History(context.Background(), g.ID, 2)
		if err == nil && len(history) == 2 && history[0].ID == retry.ID && history[0].Status != domain.RunRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("retry did not settle: history=%+v err=%v", history, err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if history[0].ID != retry.ID || history[0].Trigger != "retry" || history[0].Status != domain.RunFailed ||
		history[0].Total != 1 || history[0].Updated != 0 || history[0].Failed != 1 || len(history[0].Detail) == 0 {
		t.Fatalf("retry history=%+v", history[0])
	}
	if history[1].ID != source.ID || history[1].Status != domain.RunPartial || history[1].Failed != 1 {
		t.Fatalf("source history=%+v", history[1])
	}
}
