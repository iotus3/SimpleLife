package sqlite_test

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/iotus3/simplelife/app/internal/apptest/harness"
	"github.com/iotus3/simplelife/app/internal/domain/focus"
	"github.com/iotus3/simplelife/app/internal/domain/ledger"
	"github.com/iotus3/simplelife/app/internal/domain/shared"
	task "github.com/iotus3/simplelife/app/internal/domain/task"
	"github.com/iotus3/simplelife/app/internal/domain/template"
)

// insertFutureVersion writes a schema version the current build does not know, to
// prove Open refuses it. It needs a raw handle: the package under test would reject
// the write through its own API.
func insertFutureVersion(t *testing.T, path string, version int) error {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	_, err = db.Exec(`INSERT INTO schema_migration(version, applied_at) VALUES (?, ?)`,
		version, time.Now().UnixMilli())
	return err
}

// idGen mints deterministic identifiers: the tests assert on ordering, so a random
// ULID would make the expectations unreadable.
type idGen struct{ n int }

func (g *idGen) New() shared.ID {
	g.n++
	// 26 Crockford characters: ten for the timestamp, sixteen for the sequence.
	return shared.ID(fmt.Sprintf("0000000000%016d", g.n))
}

func clock() *harness.Clock { return harness.NewClock(harness.At("2026-11-17 09:30:00")) }

// sampleTask builds a valid task with the given title and due date.
func sampleTask(t *testing.T, id shared.ID, title string, due shared.LocalDate, now time.Time) *task.Task {
	t.Helper()
	tk, err := task.New(id, title, now)
	if err != nil {
		t.Fatalf("task.New: %v", err)
	}
	// The title doubles as the sort key so the ordering assertions stay readable;
	// SetSortKey only requires a non-empty string.
	if err := tk.SetSortKey(title, now); err != nil {
		t.Fatalf("SetSortKey: %v", err)
	}
	if !due.IsZero() {
		if err := tk.SetDueDate(due, now); err != nil {
			t.Fatalf("SetDueDate: %v", err)
		}
	}
	if err := tk.SetTags([]string{"work"}, now); err != nil {
		t.Fatalf("SetTags: %v", err)
	}
	return tk
}

// sampleEntry builds a valid entry.
func sampleEntry(t *testing.T, id shared.ID, in ledger.EntryInput, now time.Time) *ledger.Entry {
	t.Helper()
	if in.OccurredAt.IsZero() {
		in.OccurredAt = now
	}
	e, err := ledger.NewEntry(id, now, in)
	if err != nil {
		t.Fatalf("ledger.NewEntry: %v", err)
	}
	return e
}

// sampleTemplate builds a daily template with one task.
func sampleTemplate(t *testing.T, id shared.ID, name string, now time.Time) *template.Template {
	t.Helper()
	tpl, err := template.New(id, name, now)
	if err != nil {
		t.Fatalf("template.New: %v", err)
	}
	tpl.Enabled = true
	if err := tpl.SetTask(template.Task{Title: "写日记", Estimate: 1, FocusMinutes: 30, Tags: []string{"晨间"}}, now); err != nil {
		t.Fatalf("SetTask: %v", err)
	}
	return tpl
}

// sampleSession builds a finished focus session.
func sampleSession(t *testing.T, id shared.ID, taskID shared.ID, start time.Time, planned time.Duration) *focus.Session {
	t.Helper()
	s, err := focus.Start(id, taskID, focus.KindFocus, planned, start)
	if err != nil {
		t.Fatalf("focus.Start: %v", err)
	}
	if err := s.Complete(start.Add(planned)); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return s
}

// dbPath returns a database path inside the test's temp directory.
func dbPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "simplelife.db")
}
