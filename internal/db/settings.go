package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// GetSetting decodes the JSON value of a settings row into dst. It reports
// false (and leaves dst untouched) when the row does not exist.
func (d *DB) GetSetting(ctx context.Context, key string, dst any) (bool, error) {
	var raw string
	err := d.Read.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read setting %s: %w", key, err)
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		return false, fmt.Errorf("decode setting %s: %w", key, err)
	}
	return true, nil
}

// SetSetting stores v (JSON-encoded) under key.
func (d *DB) SetSetting(ctx context.Context, key string, v any) error {
	return d.SetSettings(ctx, map[string]any{key: v})
}

// SetSettings stores several settings in one transaction.
func (d *DB) SetSettings(ctx context.Context, values map[string]any) error {
	tx, err := d.Write.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	now := time.Now().UnixMilli()
	for key, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("encode setting %s: %w", key, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			key, string(raw), now); err != nil {
			return fmt.Errorf("write setting %s: %w", key, err)
		}
	}
	return tx.Commit()
}
