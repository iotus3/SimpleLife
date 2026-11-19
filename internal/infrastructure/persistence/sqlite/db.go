// Package sqlite implements every repository port on one SQLite file (modernc.org/sqlite, pure
// Go). Times are Unix milliseconds, money is integer minor units, the invariants are also
// unique indexes, and every write goes through one connection so WithinTx nests correctly.
package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// driverName is the name modernc.org/sqlite registers itself under.
const driverName = "sqlite"

// DB owns the connection pool and the transaction state.
//
// The zero value is not usable; call Open or OpenMemory.
type DB struct {
	db *sql.DB
	// conn is the connection the open transaction runs on. Holding it, rather than a *sql.Tx, is
	// what allows BEGIN IMMEDIATE: database/sql only issues a deferred BEGIN, which takes the
	// write lock too late to protect a read-then-write.
	conn  *sql.Conn
	depth int
	// memory reports an in-memory database, which has no log to checkpoint on close.
	memory bool
}

// querier is what a repository needs: either the pool or the transaction
// connection. Routing through one interface is why a use case can call six
// repositories inside WithinTx and still get one atomic write.
type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// q returns the transaction connection, or the pool when no transaction is open.
func (d *DB) q() querier {
	if d.conn != nil {
		return d.conn
	}
	return d.db
}

// Open opens and migrates the database. The pragmas travel in the DSN because foreign_keys and
// busy_timeout are per connection, and a reopened connection would lose them.
func Open(path string) (*DB, error) {
	if strings.TrimSpace(path) == "" {
		return nil, shared.Invalid("sqlite.path.empty", "", "database path is required")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create data directory: %w", err)
		}
	}
	return open(withPragmas(path, false), false)
}

// OpenMemory opens a private in-memory database. Each call gets its own name, so
// two tests never share state; the connection pool keeps it alive because a plain
// ":memory:" database dies when its last connection closes.
func OpenMemory(name string) (*DB, error) {
	if name == "" {
		name = "default"
	}
	return open(withPragmas("file:"+url.PathEscape("simplelife-"+name)+"?mode=memory&cache=shared", true), true)
}

func open(dsn string, memory bool) (*DB, error) {
	pool, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("sqlite: open: %w", err)
	}
	// One connection: writes serialise, a transaction can never deadlock against
	// itself, and the per-connection pragmas stay in force.
	pool.SetMaxOpenConns(1)
	pool.SetMaxIdleConns(1)
	if err := pool.Ping(); err != nil {
		_ = pool.Close()
		return nil, fmt.Errorf("sqlite: ping: %w", err)
	}
	db := &DB{db: pool, memory: memory}
	if err := db.migrate(); err != nil {
		_ = pool.Close()
		return nil, err
	}
	return db, nil
}

// withPragmas builds the DSN. WAL is skipped for in-memory databases: there is no
// second reader to coordinate with, and the mode needs a file.
func withPragmas(path string, memory bool) string {
	pragmas := []string{
		"foreign_keys(1)",
		"busy_timeout(5000)",
		"synchronous(NORMAL)",
	}
	if !memory {
		pragmas = append([]string{"journal_mode(WAL)"}, pragmas...)
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	parts := make([]string, 0, len(pragmas))
	for _, p := range pragmas {
		parts = append(parts, "_pragma="+p)
	}
	return path + sep + strings.Join(parts, "&")
}

// Close releases the pool; calling it twice is safe. A file database is checkpointed first, so
// the data folder ends up holding simplelife.db alone, not a database plus a -wal log.
func (d *DB) Close() error {
	if d.db == nil {
		return nil
	}
	if !d.memory {
		if _, err := d.db.ExecContext(context.Background(), "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
			// A checkpoint that fails still leaves a readable database: the next open
			// replays the log. Closing is the part that must not be skipped.
			_ = err
		}
	}
	err := d.db.Close()
	d.db = nil
	return err
}

// WithinTx implements shared.TxRunner with BEGIN IMMEDIATE, which takes the write lock at BEGIN
// so two writers cannot both read the same "is it there?" answer.
func (d *DB) WithinTx(fn func() error) error {
	if d.conn != nil {
		// Nested: join the outer unit of work. A nested failure is reported to the
		// caller, which decides whether to abort; the outer transaction stays open
		// so the caller can still choose to commit what it has.
		d.depth++
		defer func() { d.depth-- }()
		return fn()
	}
	conn, err := d.db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("sqlite: acquire connection: %w", err)
	}
	if _, err := conn.ExecContext(context.Background(), "BEGIN IMMEDIATE"); err != nil {
		_ = conn.Close()
		return mapError("begin", err)
	}
	d.conn = conn
	d.depth = 1
	// A panic must not leave the transaction open: with one connection, every
	// later write would block forever.
	finished := false
	defer func() {
		if !finished {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
		d.conn = nil
		d.depth = 0
		_ = conn.Close()
	}()
	if err := fn(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(context.Background(), "COMMIT"); err != nil {
		return mapError("commit", err)
	}
	finished = true
	return nil
}

// exec runs a statement on the active transaction or the pool.
func (d *DB) exec(query string, args ...any) (sql.Result, error) {
	res, err := d.q().ExecContext(context.Background(), query, args...)
	if err != nil {
		return nil, mapError("exec", err)
	}
	return res, nil
}

// query runs a read on the active transaction or the pool.
func (d *DB) query(query string, args ...any) (*sql.Rows, error) {
	rows, err := d.q().QueryContext(context.Background(), query, args...)
	if err != nil {
		return nil, mapError("query", err)
	}
	return rows, nil
}

// queryRow runs a single-row read. The error surfaces at Scan, which every caller
// funnels through mapError.
func (d *DB) queryRow(query string, args ...any) *sql.Row {
	return d.q().QueryRowContext(context.Background(), query, args...)
}

// mapError turns a driver error into the domain's vocabulary where it has a word for it;
// uniqueness becomes shared.ErrDuplicate so no caller parses SQLite message text.
func mapError(op string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return shared.ErrNotFound
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "UNIQUE constraint failed"):
		return shared.Dupf("%s: %s", op, msg)
	case strings.Contains(msg, "FOREIGN KEY constraint failed"):
		return shared.Conflict("sqlite.foreign_key", "referenced row is missing or still in use").Wrap(err)
	case strings.Contains(msg, "CHECK constraint failed"):
		return shared.Invalid("sqlite.check", "", msg).Wrap(err)
	}
	return fmt.Errorf("sqlite: %s: %w", op, err)
}
