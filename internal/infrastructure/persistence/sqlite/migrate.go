package sqlite

import (
	"fmt"
	"time"
)

// migration is one schema step. Steps are applied in order and never edited after
// they have shipped: a user's database already ran the old text, so changing it
// would make two installations disagree about what version 1 means.
type migration struct {
	version int
	name    string
	stmts   []string
}

// migrations is the schema history. It mirrors docs/schema.md (§5.9) and may only
// add tables, columns and indexes — dropping a column in SQLite means rebuilding
// the table, which is the one migration this project does not want to write.
var migrations = []migration{
	{
		version: 1,
		name:    "initial schema",
		stmts: []string{
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
			`CREATE INDEX idx_task_status_due ON task(status, due_at)`,
			`CREATE INDEX idx_task_parent     ON task(parent_id, sort_key)`,
			`CREATE INDEX idx_task_sort       ON task(status, sort_key)`,
			`CREATE INDEX idx_task_due_sort   ON task(due_at, sort_key)`,
			// The expander's correctness rests here, not on its existence check.
			`CREATE UNIQUE INDEX uq_task_template_occurrence
				ON task(origin_template_id, origin_date, origin_item_seq)
				WHERE origin_template_id <> ''`,

			`CREATE TABLE focus_session (
				id              TEXT    PRIMARY KEY,
				task_id         TEXT    REFERENCES task(id) ON DELETE SET NULL,
				predecessor_id  TEXT    REFERENCES focus_session(id) ON DELETE SET NULL,
				kind            INTEGER NOT NULL,
				phase           INTEGER NOT NULL,
				planned_ms      INTEGER NOT NULL DEFAULT 0,
				started_at      INTEGER NOT NULL,
				ended_at        INTEGER NOT NULL DEFAULT 0,
				paused_total_ms INTEGER NOT NULL DEFAULT 0,
				paused_at       INTEGER NOT NULL DEFAULT 0,
				interruptions   INTEGER NOT NULL DEFAULT 0,
				note            TEXT    NOT NULL DEFAULT ''
			)`,
			`CREATE INDEX idx_session_started ON focus_session(started_at)`,
			// At most one running or paused session, enforced by the store as well.
			`CREATE UNIQUE INDEX uq_session_active
				ON focus_session(phase) WHERE phase IN (1, 2)`,

			`CREATE TABLE account (
				id            TEXT    PRIMARY KEY,
				name          TEXT    NOT NULL,
				builtin_key   TEXT    NOT NULL DEFAULT '',
				kind          INTEGER NOT NULL,
				initial_minor INTEGER NOT NULL DEFAULT 0,
				currency      TEXT    NOT NULL DEFAULT 'CNY',
				archived      INTEGER NOT NULL DEFAULT 0,
				sort_order    INTEGER NOT NULL DEFAULT 0
			)`,
			`CREATE UNIQUE INDEX uq_account_builtin ON account(builtin_key) WHERE builtin_key <> ''`,

			`CREATE TABLE category (
				id          TEXT    PRIMARY KEY,
				name        TEXT    NOT NULL,
				builtin_key TEXT    NOT NULL DEFAULT '',
				direction   INTEGER NOT NULL,
				parent_id   TEXT    REFERENCES category(id) ON DELETE RESTRICT,
				sort_order  INTEGER NOT NULL DEFAULT 0
			)`,
			`CREATE UNIQUE INDEX uq_category_builtin ON category(builtin_key) WHERE builtin_key <> ''`,

			`CREATE TABLE entry (
				id               TEXT    PRIMARY KEY,
				kind             INTEGER NOT NULL,
				amount_minor     INTEGER NOT NULL CHECK (amount_minor > 0),
				currency         TEXT    NOT NULL DEFAULT 'CNY',
				account_id       TEXT    NOT NULL REFERENCES account(id) ON DELETE RESTRICT,
				to_account_id    TEXT    REFERENCES account(id) ON DELETE RESTRICT,
				category_id      TEXT    REFERENCES category(id) ON DELETE RESTRICT,
				occurred_at      INTEGER NOT NULL,
				note             TEXT    NOT NULL DEFAULT '',
				tags             TEXT    NOT NULL DEFAULT '[]',
				origin           INTEGER NOT NULL DEFAULT 1,
				focus_session_id TEXT    REFERENCES focus_session(id) ON DELETE SET NULL,
				created_at       INTEGER NOT NULL
			)`,
			`CREATE INDEX idx_entry_occurred ON entry(occurred_at DESC)`,
			`CREATE INDEX idx_entry_account  ON entry(account_id, occurred_at DESC)`,
			`CREATE INDEX idx_entry_category ON entry(category_id, occurred_at DESC)`,
			// One session books at most one entry (ADR-012). The row keeps its
			// session id even after the user edits it into a manual entry.
			`CREATE UNIQUE INDEX uq_entry_focus_session
				ON entry(focus_session_id) WHERE focus_session_id <> ''`,

			`CREATE TABLE budget (
				id          TEXT    PRIMARY KEY,
				category_id TEXT    NOT NULL REFERENCES category(id) ON DELETE CASCADE,
				month       TEXT    NOT NULL,
				limit_minor INTEGER NOT NULL,
				currency    TEXT    NOT NULL DEFAULT 'CNY',
				UNIQUE (category_id, month)
			)`,

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

			`CREATE TABLE template_log (
				id          INTEGER PRIMARY KEY AUTOINCREMENT,
				template_id TEXT    NOT NULL REFERENCES template(id) ON DELETE CASCADE,
				expand_date TEXT    NOT NULL,
				created     INTEGER NOT NULL DEFAULT 0,
				skipped     INTEGER NOT NULL DEFAULT 0,
				failed      INTEGER NOT NULL DEFAULT 0,
				detail      TEXT    NOT NULL DEFAULT '',
				created_at  INTEGER NOT NULL
			)`,
			`CREATE INDEX idx_template_log ON template_log(expand_date DESC)`,

			`CREATE TABLE setting (
				key   TEXT PRIMARY KEY,
				value TEXT NOT NULL
			)`,
		},
	},
	{
		// A template now writes exactly one task, so its definition moved onto the
		// template row and the child table is gone. Existing templates keep the first
		// item they had; the rest are dropped, which is the point of the change.
		version: 2,
		name:    "one task per template",
		stmts: []string{
			`ALTER TABLE template ADD COLUMN task_title           TEXT    NOT NULL DEFAULT ''`,
			`ALTER TABLE template ADD COLUMN task_notes           TEXT    NOT NULL DEFAULT ''`,
			`ALTER TABLE template ADD COLUMN task_priority        INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE template ADD COLUMN task_estimate        INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE template ADD COLUMN task_due_offset_days INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE template ADD COLUMN task_tags            TEXT    NOT NULL DEFAULT '[]'`,
			// The template's name stands in for a title when it never had an item, so
			// an old row keeps producing something instead of failing validation.
			`UPDATE template SET
				task_title = COALESCE(
					(SELECT NULLIF(TRIM(title), '') FROM template_item i
						WHERE i.template_id = template.id ORDER BY seq LIMIT 1),
					name),
				task_notes = COALESCE(
					(SELECT notes FROM template_item i
						WHERE i.template_id = template.id ORDER BY seq LIMIT 1), ''),
				task_priority = COALESCE(
					(SELECT priority FROM template_item i
						WHERE i.template_id = template.id ORDER BY seq LIMIT 1), 0),
				task_estimate = COALESCE(
					(SELECT estimate FROM template_item i
						WHERE i.template_id = template.id ORDER BY seq LIMIT 1), 0),
				task_due_offset_days = COALESCE(
					(SELECT due_offset_days FROM template_item i
						WHERE i.template_id = template.id ORDER BY seq LIMIT 1), 0),
				task_tags = COALESCE(
					(SELECT tags FROM template_item i
						WHERE i.template_id = template.id ORDER BY seq LIMIT 1), '[]')`,
			`DROP TABLE template_item`,
		},
	},
	{
		// Countdown goals arrived after the first release: one table, no children.
		// The two dates are stored as 'YYYY-MM-DD' text like every other calendar day
		// in the file, so a sqlite3 shell can read them and they sort correctly.
		version: 3,
		name:    "countdown goals",
		stmts: []string{
			`CREATE TABLE goal (
				id         TEXT    PRIMARY KEY,
				name       TEXT    NOT NULL,
				note       TEXT    NOT NULL DEFAULT '',
				start_date TEXT    NOT NULL,
				end_date   TEXT    NOT NULL,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL
			)`,
			`CREATE INDEX idx_goal_end ON goal(end_date)`,
		},
	},
	{
		// A task may carry its own pomodoro length. Zero — the default, and what
		// every row created before this step gets — means "the length the focus
		// page is set to", so the ALTER is safe for existing files.
		version: 4,
		name:    "per-task pomodoro length",
		stmts: []string{
			`ALTER TABLE task ADD COLUMN focus_minutes INTEGER NOT NULL DEFAULT 0`,
		},
	},
	{
		// A template may carry the length of one pomodoro for the task it writes, so
		// a "read for 20 minutes" template produces tasks that already know how long
		// they run. Same meaning for zero: the task falls back to the focus page.
		version: 5,
		name:    "per-template pomodoro length",
		stmts: []string{
			`ALTER TABLE template ADD COLUMN task_focus_minutes INTEGER NOT NULL DEFAULT 0`,
		},
	},
}

// SchemaVersion is the version the code expects. It is exported so the health
// check and the tests can assert the file matches the binary.
func SchemaVersion() int {
	if len(migrations) == 0 {
		return 0
	}
	return migrations[len(migrations)-1].version
}

// migrate applies every pending step in one transaction: a failure leaves the
// file exactly as it was, so a half-migrated database cannot exist.
func (d *DB) migrate() error {
	if _, err := d.db.Exec(`CREATE TABLE IF NOT EXISTS schema_migration (
		version    INTEGER PRIMARY KEY,
		applied_at INTEGER NOT NULL
	)`); err != nil {
		return mapError("create schema_migration", err)
	}
	var current int
	row := d.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migration`)
	if err := row.Scan(&current); err != nil {
		return mapError("read schema version", err)
	}
	if current > SchemaVersion() {
		// A newer build wrote this file. Refusing is the only safe answer: the
		// older binary does not know what the newer columns mean.
		return fmt.Errorf("sqlite: database schema v%d is newer than this build (v%d)", current, SchemaVersion())
	}
	pending := 0
	for _, m := range migrations {
		if m.version <= current {
			continue
		}
		pending++
	}
	if pending == 0 {
		return nil
	}
	return d.WithinTx(func() error {
		for _, m := range migrations {
			if m.version <= current {
				continue
			}
			for i, stmt := range m.stmts {
				if _, err := d.exec(stmt); err != nil {
					return fmt.Errorf("sqlite: migration %d (%s) statement %d: %w", m.version, m.name, i+1, err)
				}
			}
			if _, err := d.exec(`INSERT INTO schema_migration(version, applied_at) VALUES (?, ?)`,
				m.version, time.Now().UnixMilli()); err != nil {
				return fmt.Errorf("sqlite: record migration %d: %w", m.version, err)
			}
		}
		return nil
	})
}

// AppSchemaVersion reports the version stored in the file. It exists for the
// diagnostics panel and for tests that replay a migration.
func (d *DB) AppSchemaVersion() (int, error) {
	var version int
	err := d.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migration`).Scan(&version)
	if err != nil {
		return 0, mapError("read schema version", err)
	}
	return version, nil
}
