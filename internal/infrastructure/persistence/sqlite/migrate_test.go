package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/iotus3/simplelife/app/internal/infrastructure/persistence/sqlite"
)

// seedV1Template writes a version 1 database holding a template with two items, so the upgrade
// is tested against real old rows. The column list is the version 1 DDL verbatim.
func seedV1Template(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer func() { _ = db.Close() }()

	stmts := []string{
		`CREATE TABLE schema_migration (version INTEGER PRIMARY KEY, applied_at INTEGER NOT NULL)`,
		`CREATE TABLE template (
			id           TEXT    PRIMARY KEY,
			name         TEXT    NOT NULL,
			enabled      INTEGER NOT NULL DEFAULT 1,
			kind         INTEGER NOT NULL,
			weekdays     TEXT    NOT NULL DEFAULT '[]',
			day_of_month INTEGER NOT NULL DEFAULT 0,
			dates        TEXT    NOT NULL DEFAULT '[]',
			every_n_days INTEGER NOT NULL DEFAULT 0,
			anchor       TEXT    NOT NULL DEFAULT '',
			created_at   INTEGER NOT NULL,
			updated_at   INTEGER NOT NULL
		)`,
		`CREATE TABLE template_item (
			template_id     TEXT    NOT NULL REFERENCES template(id) ON DELETE CASCADE,
			seq             INTEGER NOT NULL,
			title           TEXT    NOT NULL,
			notes           TEXT    NOT NULL DEFAULT '',
			priority        INTEGER NOT NULL DEFAULT 0,
			estimate        INTEGER NOT NULL DEFAULT 0,
			due_offset_days INTEGER NOT NULL DEFAULT 0,
			tags            TEXT    NOT NULL DEFAULT '[]',
			PRIMARY KEY (template_id, seq)
		)`,
		// Every later step that alters a table needs that table in the fixture, so
		// the ones the migrations touch are all here, in their version 1 shape.
		`CREATE TABLE task (
			id                 TEXT    PRIMARY KEY,
			title              TEXT    NOT NULL,
			notes              TEXT    NOT NULL DEFAULT '',
			status             INTEGER NOT NULL,
			priority           INTEGER NOT NULL DEFAULT 0,
			parent_id          TEXT    REFERENCES task(id) ON DELETE CASCADE,
			tags               TEXT    NOT NULL DEFAULT '[]',
			due_at             INTEGER NOT NULL DEFAULT 0,
			estimate_pomodoros INTEGER NOT NULL DEFAULT 0,
			done_pomodoros     INTEGER NOT NULL DEFAULT 0,
			sort_key           TEXT    NOT NULL,
			created_at         INTEGER NOT NULL,
			updated_at         INTEGER NOT NULL,
			completed_at       INTEGER NOT NULL DEFAULT 0,
			origin_template_id TEXT    NOT NULL DEFAULT '',
			origin_date        TEXT    NOT NULL DEFAULT '',
			origin_item_seq    INTEGER NOT NULL DEFAULT -1
		)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture schema: %v", err)
		}
	}

	now := time.Now().UnixMilli()
	if _, err := db.Exec(`INSERT INTO task(id, title, status, sort_key, created_at, updated_at)
		VALUES ('0000000000TASK00000000A1', '旧版本留下的待办', 1, 'n', ?, ?)`, now, now); err != nil {
		t.Fatalf("fixture task: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO schema_migration(version, applied_at) VALUES (1, ?)`, now); err != nil {
		t.Fatalf("fixture version: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO template(id, name, enabled, kind, created_at, updated_at)
		VALUES ('0000000000TMPL00000000A1', '有事项的模板', 1, 0, ?, ?)`, now, now); err != nil {
		t.Fatalf("fixture template: %v", err)
	}
	items := []struct {
		seq   int
		title string
		notes string
		est   int
	}{
		{0, "第一条", "先写入库的那条", 2},
		{1, "第二条", "会被丢弃", 5},
	}
	for _, item := range items {
		if _, err := db.Exec(`INSERT INTO template_item(template_id, seq, title, notes, priority, estimate, due_offset_days, tags)
			VALUES ('0000000000TMPL00000000A1', ?, ?, ?, 1, ?, 3, '["晨间"]')`,
			item.seq, item.title, item.notes, item.est); err != nil {
			t.Fatalf("fixture item: %v", err)
		}
	}
}

func TestMigrationV1ItemsBecomeOneTask(t *testing.T) {
	// Version 2 collapsed a template's item list into exactly one task. An old row
	// must keep producing something after the upgrade, and the first item wins
	// because it was the one the user saw at the top of the list.
	path := filepath.Join(t.TempDir(), "v1.db")
	seedV1Template(t, path)

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open must upgrade a version 1 file: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	version, err := db.AppSchemaVersion()
	if err != nil {
		t.Fatalf("AppSchemaVersion: %v", err)
	}
	if version != sqlite.SchemaVersion() {
		t.Fatalf("version = %d, want %d", version, sqlite.SchemaVersion())
	}
	tpl, err := db.Templates().Get("0000000000TMPL00000000A1")
	if err != nil {
		t.Fatalf("the template must survive the upgrade: %v", err)
	}
	if tpl.Name != "有事项的模板" || !tpl.Enabled {
		t.Fatalf("template = %+v", tpl)
	}
	if tpl.Task.Title != "第一条" {
		t.Fatalf("task title = %q, want the first item's", tpl.Task.Title)
	}
	if tpl.Task.Notes != "先写入库的那条" || tpl.Task.Priority != 1 ||
		tpl.Task.Estimate != 2 || tpl.Task.DueOffsetDays != 3 {
		t.Fatalf("task fields must come from the same item: %+v", tpl.Task)
	}
	if len(tpl.Task.Tags) != 1 || tpl.Task.Tags[0] != "晨间" {
		t.Fatalf("task tags = %v", tpl.Task.Tags)
	}

	// The item table is gone: it must not be readable through the new code path.
	var remaining int
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	defer func() { _ = raw.Close() }()
	if err := raw.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'template_item'`).
		Scan(&remaining); err != nil {
		t.Fatalf("inspect schema: %v", err)
	}
	if remaining != 0 {
		t.Fatal("template_item must be dropped by the version 2 migration")
	}
}

func TestMigrationV1TemplateWithoutItemsKeepsItsName(t *testing.T) {
	// A template whose items were never written must still produce a titled todo,
	// so the upgrade falls back to the template's name rather than an empty title
	// the task validator would reject on every run.
	path := filepath.Join(t.TempDir(), "v1-empty.db")
	seedV1Template(t, path)

	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("raw open: %v", err)
	}
	if _, err := raw.Exec(`DELETE FROM template_item`); err != nil {
		t.Fatalf("clear items: %v", err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sqlite.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tpl, err := db.Templates().Get("0000000000TMPL00000000A1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if tpl.Task.Title != "有事项的模板" {
		t.Fatalf("task title = %q, want the template name as fallback", tpl.Task.Title)
	}
	if err := tpl.Task.Validate(); err != nil {
		t.Fatalf("an upgraded template must be valid, or expansion would fail forever: %v", err)
	}
}
