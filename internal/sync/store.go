// Package sync implements the LWW key-value store backing /v1/sync.
package sync

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Item is a single sync entry.
type Item struct {
	Key        string `json:"key"`
	Value      []byte `json:"-"`              // raw bytes; encoded as base64 in the JSON envelope
	ValueB64   string `json:"value"`          // populated for wire format
	PayloadVer int    `json:"payload_ver"`
	Version    int64  `json:"version"`
	Deleted    bool   `json:"deleted"`
	UpdatedAt  int64  `json:"updated_at"`
	UpdatedBy  string `json:"updated_by"`
}

// WriteIntent is one entry in a client push.
type WriteIntent struct {
	Key         string
	Value       []byte
	PayloadVer  int
	Deleted     bool
	BaseVersion int64
}

// WriteResult is the per-item outcome of a push.
type WriteResult struct {
	Key             string `json:"key"`
	Status          string `json:"status"` // accepted | accepted_overwrite | rejected
	Version         int64  `json:"version,omitempty"`
	PreviousVersion int64  `json:"previous_version,omitempty"`
}

// Store talks to the items + version_counters tables.
type Store struct {
	DB *sql.DB
}

// Pull returns all items with version > since for the given (user, app),
// up to limit rows.
func (s *Store) Pull(ctx context.Context, userSub, appID string, since int64, limit int) ([]Item, int64, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT key, value, payload_ver, version, deleted, updated_at, updated_by
		   FROM items
		  WHERE user_sub = ? AND app_id = ? AND version > ?
		  ORDER BY version ASC
		  LIMIT ?`,
		userSub, appID, since, limit,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Item{}
	var maxVer int64 = since
	for rows.Next() {
		var (
			it      Item
			value   []byte
			deleted int
		)
		if err := rows.Scan(&it.Key, &value, &it.PayloadVer, &it.Version,
			&deleted, &it.UpdatedAt, &it.UpdatedBy); err != nil {
			return nil, 0, err
		}
		it.Value = value
		it.Deleted = deleted == 1
		out = append(out, it)
		if it.Version > maxVer {
			maxVer = it.Version
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, maxVer, nil
}

// CurrentVersion returns the latest assigned version for (user, app), or 0
// if no items have been written.
func (s *Store) CurrentVersion(ctx context.Context, userSub, appID string) (int64, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT version FROM version_counters WHERE user_sub = ? AND app_id = ?`,
		userSub, appID)
	var v int64
	if err := row.Scan(&v); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return v, nil
}

// Apply runs a batch of writes in a single transaction. For each item:
//   - looks up the existing row's version
//   - increments the per-(user, app) counter
//   - upserts the new row
//   - returns accepted if the client's base_version matched, accepted_overwrite if it didn't
//
// Pure LWW: a stale client write still wins. The status difference lets the
// client surface a warning if it cares.
func (s *Store) Apply(ctx context.Context, userSub, appID, deviceID string, intents []WriteIntent) ([]WriteResult, int64, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, err
	}
	defer tx.Rollback()

	results := make([]WriteResult, 0, len(intents))
	now := time.Now().UTC().Unix()
	for _, it := range intents {
		// Current version for this key, if any.
		var prev int64
		err := tx.QueryRowContext(ctx,
			`SELECT version FROM items WHERE user_sub = ? AND app_id = ? AND key = ?`,
			userSub, appID, it.Key,
		).Scan(&prev)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, 0, fmt.Errorf("read item version: %w", err)
		}

		// Increment per-(user, app) counter.
		next, err := bumpCounter(ctx, tx, userSub, appID)
		if err != nil {
			return nil, 0, err
		}

		payloadVer := it.PayloadVer
		if payloadVer == 0 {
			payloadVer = 1
		}
		deleted := 0
		if it.Deleted {
			deleted = 1
		}
		var value []byte
		if !it.Deleted {
			value = it.Value
		}

		_, err = tx.ExecContext(ctx,
			`INSERT INTO items (user_sub, app_id, key, value, payload_ver, version, deleted, updated_at, updated_by)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(user_sub, app_id, key) DO UPDATE SET
			   value = excluded.value,
			   payload_ver = excluded.payload_ver,
			   version = excluded.version,
			   deleted = excluded.deleted,
			   updated_at = excluded.updated_at,
			   updated_by = excluded.updated_by`,
			userSub, appID, it.Key, value, payloadVer, next, deleted, now, deviceID,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("upsert item: %w", err)
		}
		status := "accepted"
		if prev > 0 && it.BaseVersion != prev {
			status = "accepted_overwrite"
		}
		results = append(results, WriteResult{
			Key: it.Key, Status: status, Version: next, PreviousVersion: prev,
		})
	}

	// Final cursor.
	var cursor int64
	err = tx.QueryRowContext(ctx,
		`SELECT version FROM version_counters WHERE user_sub = ? AND app_id = ?`,
		userSub, appID,
	).Scan(&cursor)
	if err != nil {
		return nil, 0, err
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, err
	}
	return results, cursor, nil
}

func bumpCounter(ctx context.Context, tx *sql.Tx, userSub, appID string) (int64, error) {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO version_counters (user_sub, app_id, version) VALUES (?, ?, 1)
		 ON CONFLICT(user_sub, app_id) DO UPDATE SET version = version + 1`,
		userSub, appID,
	)
	if err != nil {
		return 0, fmt.Errorf("bump counter: %w", err)
	}
	var v int64
	err = tx.QueryRowContext(ctx,
		`SELECT version FROM version_counters WHERE user_sub = ? AND app_id = ?`,
		userSub, appID,
	).Scan(&v)
	return v, err
}
