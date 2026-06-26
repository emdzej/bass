package devices

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// Device is a paired (user, app, device) triple.
type Device struct {
	ID             string    `json:"id"`
	UserSub        string    `json:"user_sub"`
	AppID          string    `json:"app_id"`
	Label          string    `json:"label,omitempty"`
	TokenExpires   time.Time `json:"token_expires"`
	RefreshExpires time.Time `json:"refresh_expires"`
	CreatedAt      time.Time `json:"created_at"`
	LastSeenAt     time.Time `json:"last_seen_at"`
	Revoked        bool      `json:"revoked"`
}

// Pairing is the result of pairing/refresh — includes the plaintext tokens.
// Caller is responsible for delivering them to the client and not logging them.
type Pairing struct {
	Device       Device
	SyncToken    string
	RefreshToken string
}

// Store is the SQL-backed device registry.
type Store struct {
	DB         *sql.DB
	TokenTTL   time.Duration
	RefreshTTL time.Duration
}

// Create mints a fresh device row for (user, app) and returns the plaintext
// tokens alongside the persisted device.
func (s *Store) Create(ctx context.Context, userSub, appID, label string) (*Pairing, error) {
	syncTok, err := MintToken()
	if err != nil {
		return nil, err
	}
	refreshTok, err := MintToken()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	d := Device{
		ID:             ulid.Make().String(),
		UserSub:        userSub,
		AppID:          appID,
		Label:          label,
		TokenExpires:   now.Add(s.TokenTTL),
		RefreshExpires: now.Add(s.RefreshTTL),
		CreatedAt:      now,
		LastSeenAt:     now,
	}
	_, err = s.DB.ExecContext(ctx,
		`INSERT INTO devices (id, user_sub, app_id, label, sync_token_hash, refresh_hash,
		                      token_expires, refresh_expires, created_at, last_seen_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, userSub, appID, label,
		HashToken(syncTok), HashToken(refreshTok),
		d.TokenExpires.Unix(), d.RefreshExpires.Unix(),
		d.CreatedAt.Unix(), d.LastSeenAt.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("insert device: %w", err)
	}
	return &Pairing{Device: d, SyncToken: syncTok, RefreshToken: refreshTok}, nil
}

// LookupBySyncToken finds the active (non-revoked, non-expired) device for
// the given sync token. Updates last_seen_at.
func (s *Store) LookupBySyncToken(ctx context.Context, token string) (*Device, error) {
	now := time.Now().UTC()
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, user_sub, app_id, COALESCE(label, ''), token_expires, refresh_expires,
		        created_at, last_seen_at, revoked_at
		   FROM devices
		  WHERE sync_token_hash = ? AND revoked_at IS NULL AND token_expires > ?`,
		HashToken(token), now.Unix(),
	)
	d, err := scanDevice(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	_, _ = s.DB.ExecContext(ctx, `UPDATE devices SET last_seen_at = ? WHERE id = ?`, now.Unix(), d.ID)
	return d, nil
}

// Refresh rotates both tokens for the device matching the given refresh
// token. Returns ErrInvalidToken if the refresh token is unknown or expired.
func (s *Store) Refresh(ctx context.Context, refreshToken string) (*Pairing, error) {
	now := time.Now().UTC()
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, user_sub, app_id, COALESCE(label, ''), token_expires, refresh_expires,
		        created_at, last_seen_at, revoked_at
		   FROM devices
		  WHERE refresh_hash = ? AND revoked_at IS NULL AND refresh_expires > ?`,
		HashToken(refreshToken), now.Unix(),
	)
	d, err := scanDevice(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidToken
		}
		return nil, err
	}
	syncTok, err := MintToken()
	if err != nil {
		return nil, err
	}
	newRefresh, err := MintToken()
	if err != nil {
		return nil, err
	}
	d.TokenExpires = now.Add(s.TokenTTL)
	d.RefreshExpires = now.Add(s.RefreshTTL)
	d.LastSeenAt = now
	_, err = s.DB.ExecContext(ctx,
		`UPDATE devices
		    SET sync_token_hash = ?, refresh_hash = ?, token_expires = ?, refresh_expires = ?,
		        last_seen_at = ?
		  WHERE id = ?`,
		HashToken(syncTok), HashToken(newRefresh),
		d.TokenExpires.Unix(), d.RefreshExpires.Unix(),
		now.Unix(), d.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update device: %w", err)
	}
	return &Pairing{Device: *d, SyncToken: syncTok, RefreshToken: newRefresh}, nil
}

// ListForUserApp returns all (active and revoked) devices for the given
// (user, app). Used by /v1/devices.
func (s *Store) ListForUserApp(ctx context.Context, userSub, appID string) ([]Device, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_sub, app_id, COALESCE(label, ''), token_expires, refresh_expires,
		        created_at, last_seen_at, revoked_at
		   FROM devices
		  WHERE user_sub = ? AND app_id = ?
		  ORDER BY created_at DESC`, userSub, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// ListForApp returns all devices for an app (admin view).
func (s *Store) ListForApp(ctx context.Context, appID string) ([]Device, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, user_sub, app_id, COALESCE(label, ''), token_expires, refresh_expires,
		        created_at, last_seen_at, revoked_at
		   FROM devices
		  WHERE app_id = ?
		  ORDER BY created_at DESC`, appID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

// Revoke marks a device as revoked. Idempotent.
func (s *Store) Revoke(ctx context.Context, id string) error {
	now := time.Now().UTC().Unix()
	_, err := s.DB.ExecContext(ctx,
		`UPDATE devices SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanDevice(s scanner) (*Device, error) {
	var (
		d                              Device
		tokenExp, refreshExp, created  int64
		lastSeen                       int64
		revokedAt                      sql.NullInt64
	)
	if err := s.Scan(&d.ID, &d.UserSub, &d.AppID, &d.Label,
		&tokenExp, &refreshExp, &created, &lastSeen, &revokedAt); err != nil {
		return nil, err
	}
	d.TokenExpires = time.Unix(tokenExp, 0).UTC()
	d.RefreshExpires = time.Unix(refreshExp, 0).UTC()
	d.CreatedAt = time.Unix(created, 0).UTC()
	d.LastSeenAt = time.Unix(lastSeen, 0).UTC()
	d.Revoked = revokedAt.Valid
	return &d, nil
}
