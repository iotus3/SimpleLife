package sqlite_test

import (
	"errors"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

func TestSessionRoundTripKeepsTheTimestamps(t *testing.T) {
	db := openFile(t)
	start := clock().Now()
	session, err := focus.Start("0000000000SESS0000000010", "", focus.KindFocus, 25*time.Minute, start)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Sessions().Create(session); err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := db.Sessions().Get(session.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !got.StartedAt.Equal(start) || got.Planned != 25*time.Minute {
		t.Fatalf("session = %+v", got)
	}
	// The timer is derived from timestamps, so a dropped millisecond shows up as a
	// visibly wrong countdown after a restart.
	if got.StartedAt.UnixMilli() != start.UnixMilli() {
		t.Fatalf("start drifted: %v vs %v", got.StartedAt, start)
	}
	if got.TaskID != "" || got.Kind != focus.KindFocus || got.Phase != focus.PhaseRunning {
		t.Fatalf("fields = %+v", got)
	}
}

func TestSessionActiveIsUnique(t *testing.T) {
	db := openFile(t)
	start := clock().Now()
	first, err := focus.Start("0000000000SESS0000000011", "", focus.KindFocus, 25*time.Minute, start)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Sessions().Create(first); err != nil {
		t.Fatal(err)
	}
	second, err := focus.Start("0000000000SESS0000000012", "", focus.KindFocus, 25*time.Minute, start)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Sessions().Create(second); !errors.Is(err, shared.ErrDuplicate) {
		t.Fatalf("a second active session must be refused by the store, got %v", err)
	}

	active, err := db.Sessions().Active()
	if err != nil {
		t.Fatal(err)
	}
	if active == nil || active.ID != first.ID {
		t.Fatalf("active = %+v", active)
	}

	// Settling the first frees the slot.
	if err := first.Complete(start.Add(25 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := db.Sessions().Update(first); err != nil {
		t.Fatal(err)
	}
	if active, err := db.Sessions().Active(); err != nil || active != nil {
		t.Fatalf("active after completing = %+v, %v", active, err)
	}
	if err := db.Sessions().Create(second); err != nil {
		t.Fatalf("the slot must be free again: %v", err)
	}
}

func TestSessionUpdateMissing(t *testing.T) {
	db := openMemory(t)
	start := clock().Now()
	session, err := focus.Start("0000000000SESS0000000013", "", focus.KindFocus, 25*time.Minute, start)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Sessions().Update(session); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("updating a missing session must be not-found, got %v", err)
	}
	if _, err := db.Sessions().Get("0000000000SESS0000000099"); !shared.IsKind(err, shared.KindNotFound) {
		t.Fatalf("a missing session must be not-found, got %v", err)
	}
}

func TestSessionQueries(t *testing.T) {
	db := openFile(t)
	clk := clock()
	base := clk.Now()

	tk := sampleTask(t, "0000000000TASK0000000101", "写方案", shared.LocalDate{}, base)
	if err := db.Tasks().Create(tk); err != nil {
		t.Fatal(err)
	}

	// Three finished focus sessions on the same day, plus one break and one
	// cancelled session that must not be credited.
	morning := sampleSession(t, "0000000000SESS0000000101", tk.ID, base, 25*time.Minute)
	noon := sampleSession(t, "0000000000SESS0000000102", tk.ID, base.Add(2*time.Hour), 25*time.Minute)
	evening := sampleSession(t, "0000000000SESS0000000103", tk.ID, base.Add(10*time.Hour), 25*time.Minute)
	breakSession, err := focus.Start("0000000000SESS0000000104", "", focus.KindShortBreak, 5*time.Minute, base.Add(30*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if err := breakSession.Complete(base.Add(35 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	cancelled, err := focus.Start("0000000000SESS0000000105", tk.ID, focus.KindFocus, 25*time.Minute, base.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if err := cancelled.Cancel(base.Add(4*time.Minute), true); err != nil {
		t.Fatal(err)
	}
	for _, s := range []*focus.Session{morning, noon, evening, breakSession, cancelled} {
		if err := db.Sessions().Create(s); err != nil {
			t.Fatalf("Create %s: %v", s.ID, err)
		}
	}

	from := shared.DateOf(base).Midnight()
	to := from.Add(24 * time.Hour)

	count, err := db.Sessions().FocusCountBetween(from, to)
	if err != nil || count != 3 {
		t.Fatalf("count = %d, %v", count, err)
	}
	minutes, err := db.Sessions().FocusMinutesBetween(from, to)
	if err != nil || minutes != 75 {
		t.Fatalf("minutes = %d, %v", minutes, err)
	}
	if count, err := db.Sessions().FocusCountBetween(from.Add(-48*time.Hour), from); err != nil || count != 0 {
		t.Fatalf("an earlier window must be empty: %d, %v", count, err)
	}
	taskMinutes, err := db.Sessions().TaskFocusMinutes(tk.ID)
	if err != nil || taskMinutes != 75 {
		t.Fatalf("task minutes = %d, %v", taskMinutes, err)
	}

	byRange, err := db.Sessions().ListByRange(from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(byRange) != 5 {
		t.Fatalf("range = %d sessions", len(byRange))
	}
	// Newest first.
	if byRange[0].ID != evening.ID {
		t.Fatalf("first = %s, want the evening session", byRange[0].ID)
	}
	for i := 1; i < len(byRange); i++ {
		if byRange[i].StartedAt.After(byRange[i-1].StartedAt) {
			t.Fatalf("range is not newest-first at %d", i)
		}
	}

	recent, err := db.Sessions().ListRecent(2)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 || recent[0].ID != evening.ID {
		t.Fatalf("recent = %+v", recent)
	}
	all, err := db.Sessions().ListRecent(0)
	if err != nil || len(all) != 5 {
		t.Fatalf("recent(0) = %d, %v", len(all), err)
	}
}
