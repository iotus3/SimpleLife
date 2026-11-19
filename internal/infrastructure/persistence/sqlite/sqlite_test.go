package sqlite_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
)

// openFile opens a database in the test's temp directory and closes it before the
// directory is removed (Windows refuses to delete an open file).
func openFile(t *testing.T) *sqlite.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "simplelife.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	})
	return db
}

func openMemory(t *testing.T) *sqlite.DB {
	t.Helper()
	// The name is unique per test so two tests never share the shared-cache file.
	db, err := sqlite.OpenMemory(t.Name())
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestCloseLeavesOneFile pins what a backup relies on: after a clean exit the data folder holds
// simplelife.db alone.
func TestCloseLeavesOneFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "simplelife.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	// A write through the settings repository, so the log has something to replay.
	if err := db.Settings().Set("probe", "1"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(path + suffix); err == nil {
			t.Errorf("%s survived Close, so a copy of the data folder would miss writes", suffix)
		}
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("the database itself is gone: %v", err)
	}
}

func TestMigrationsRunOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deeper", "simplelife.db")

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open must create the directory: %v", err)
	}
	version, err := db.AppSchemaVersion()
	if err != nil {
		t.Fatalf("AppSchemaVersion: %v", err)
	}
	if version != sqlite.SchemaVersion() {
		t.Fatalf("version = %d, want %d", version, sqlite.SchemaVersion())
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening must not replay the migration: a second CREATE TABLE would fail.
	db2, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = db2.Close() })
	if _, err := db2.Tasks().CountOpen(); err != nil {
		t.Fatalf("the schema must be usable after reopening: %v", err)
	}
}

func TestSchemaRejectsAFutureVersion(t *testing.T) {
	// A database written by a newer build must be refused, not silently used: the
	// older binary does not know what the new columns mean.
	path := filepath.Join(t.TempDir(), "future.db")
	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	// Bump the stored version through a second connection.
	raw, err := sqlite.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Settings().Set("probe", "1"); err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	// Simulate the newer build by opening a raw handle and inserting a fake row.
	if err := insertFutureVersion(t, path, sqlite.SchemaVersion()+1); err != nil {
		t.Fatalf("seed future version: %v", err)
	}
	if _, err := sqlite.Open(path); err == nil {
		t.Fatal("opening a newer schema must fail")
	}
}

func TestOpenRejectsAnEmptyPath(t *testing.T) {
	if _, err := sqlite.Open("   "); !shared.IsKind(err, shared.KindInvalid) {
		t.Fatalf("an empty path must be invalid, got %v", err)
	}
}

func TestWithinTxCommitsAndRollsBack(t *testing.T) {
	db := openFile(t)

	// Commit: the write survives.
	err := db.WithinTx(func() error {
		return db.Settings().Set("currency", "USD")
	})
	if err != nil {
		t.Fatalf("WithinTx: %v", err)
	}
	if got, err := db.Settings().Get("currency"); err != nil || got != "USD" {
		t.Fatalf("committed value = %q, %v", got, err)
	}

	// Rollback: everything the callback wrote disappears.
	boom := errors.New("boom")
	err = db.WithinTx(func() error {
		if err := db.Settings().Set("currency", "CNY"); err != nil {
			return err
		}
		if err := db.Settings().Set("language", "en"); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("WithinTx must return the callback error, got %v", err)
	}
	if got, _ := db.Settings().Get("currency"); got != "USD" {
		t.Fatalf("a failed transaction must not write: currency = %q", got)
	}
	if _, err := db.Settings().Get("language"); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a failed transaction must not write: language err = %v", err)
	}
}

func TestWithinTxNests(t *testing.T) {
	db := openFile(t)
	err := db.WithinTx(func() error {
		if err := db.Settings().Set("outer", "1"); err != nil {
			return err
		}
		// A nested block joins the outer one; its writes are part of the same unit.
		if err := db.WithinTx(func() error {
			return db.Settings().Set("inner", "1")
		}); err != nil {
			return err
		}
		return errors.New("abort everything")
	})
	if err == nil {
		t.Fatal("the outer failure must be reported")
	}
	for _, key := range []string{"outer", "inner"} {
		if _, err := db.Settings().Get(key); !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("%s must be rolled back, got %v", key, err)
		}
	}
}

func TestWithinTxSurvivesAPanic(t *testing.T) {
	db := openFile(t)
	func() {
		defer func() { _ = recover() }()
		_ = db.WithinTx(func() error {
			panic("interrupted")
		})
	}()
	// The connection must be usable again: a leaked transaction would block here
	// forever because the pool holds exactly one connection.
	done := make(chan error, 1)
	go func() { done <- db.Settings().Set("after", "panic") }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("write after panic: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a panic left the transaction open and the pool blocked")
	}
}
