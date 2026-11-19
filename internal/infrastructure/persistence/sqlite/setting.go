package sqlite

import (
	"database/sql"
	"errors"

	"github.com/iotus3/simplelife/app/internal/domain/shared"
)

// Get reads one settings value, or shared.ErrNotFound when it was never written.
// A missing row is the normal state of a fresh install, not an error to report.
func (r SettingsRepo) Get(key string) (string, error) {
	var value string
	err := r.queryRow(`SELECT value FROM setting WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) || isNoRows(err) {
			return "", shared.ErrNotFound
		}
		return "", mapError("read setting", err)
	}
	return value, nil
}

// Set writes one settings value.
func (r SettingsRepo) Set(key, value string) error {
	_, err := r.exec(`INSERT INTO setting (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}
