// Package apps implements the app registry: registered apps, their allowed
// origins, redirect URIs, and server-cap key allowlists.
package apps

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

var (
	ErrNotFound = errors.New("app not found")
	ErrConflict = errors.New("app already exists")
)

// App is a registered backendless application.
type App struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Origins      []string  `json:"origins"`
	RedirectURIs []string  `json:"redirect_uris"`
	KeyAllowlist []string  `json:"key_allowlist"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// AllowsOrigin reports whether the given Origin header value matches an
// allowed origin entry. Exact match only (per the design decision in SPEC §14).
func (a *App) AllowsOrigin(origin string) bool {
	for _, o := range a.Origins {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

// AllowsRedirectURI reports whether the given redirect_uri is registered.
// Exact match only.
func (a *App) AllowsRedirectURI(uri string) bool {
	for _, u := range a.RedirectURIs {
		if u == uri {
			return true
		}
	}
	return false
}

// AllowsKey reports whether the given key matches any pattern in the
// server-cap allowlist (path.Match glob).
func (a *App) AllowsKey(key string) bool {
	for _, pat := range a.KeyAllowlist {
		if pat == "*" {
			return true
		}
		ok, err := path.Match(pat, key)
		if err == nil && ok {
			return true
		}
	}
	return false
}

// Store is the SQL-backed app registry.
type Store struct {
	DB *sql.DB
}

// Create inserts a new app. Returns ErrConflict if the id is already taken.
func (s *Store) Create(ctx context.Context, a App) error {
	if a.KeyAllowlist == nil {
		a.KeyAllowlist = []string{"*"}
	}
	now := time.Now().UTC()
	a.CreatedAt, a.UpdatedAt = now, now

	origins, _ := json.Marshal(a.Origins)
	redirects, _ := json.Marshal(a.RedirectURIs)
	allowlist, _ := json.Marshal(a.KeyAllowlist)

	_, err := s.DB.ExecContext(ctx,
		`INSERT INTO apps (id, name, origins, redirect_uris, key_allowlist, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.Name, string(origins), string(redirects), string(allowlist),
		now.Unix(), now.Unix(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
		return fmt.Errorf("insert app: %w", err)
	}
	return nil
}

// Get fetches an app by id.
func (s *Store) Get(ctx context.Context, id string) (*App, error) {
	row := s.DB.QueryRowContext(ctx,
		`SELECT id, name, origins, redirect_uris, key_allowlist, created_at, updated_at
		 FROM apps WHERE id = ?`, id)
	return scanApp(row)
}

// List returns all apps.
func (s *Store) List(ctx context.Context) ([]App, error) {
	rows, err := s.DB.QueryContext(ctx,
		`SELECT id, name, origins, redirect_uris, key_allowlist, created_at, updated_at
		 FROM apps ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []App{}
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

// Update overwrites the mutable fields. ID and created_at are preserved.
func (s *Store) Update(ctx context.Context, a App) error {
	origins, _ := json.Marshal(a.Origins)
	redirects, _ := json.Marshal(a.RedirectURIs)
	allowlist, _ := json.Marshal(a.KeyAllowlist)
	now := time.Now().UTC().Unix()
	res, err := s.DB.ExecContext(ctx,
		`UPDATE apps SET name = ?, origins = ?, redirect_uris = ?, key_allowlist = ?, updated_at = ?
		 WHERE id = ?`,
		a.Name, string(origins), string(redirects), string(allowlist), now, a.ID,
	)
	if err != nil {
		return fmt.Errorf("update app: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes an app and cascades to its devices.
func (s *Store) Delete(ctx context.Context, id string) error {
	res, err := s.DB.ExecContext(ctx, `DELETE FROM apps WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete app: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanApp(s scanner) (*App, error) {
	var (
		a                                App
		origins, redirects, allowlist    string
		createdUnix, updatedUnix         int64
	)
	if err := s.Scan(&a.ID, &a.Name, &origins, &redirects, &allowlist, &createdUnix, &updatedUnix); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(origins), &a.Origins)
	_ = json.Unmarshal([]byte(redirects), &a.RedirectURIs)
	_ = json.Unmarshal([]byte(allowlist), &a.KeyAllowlist)
	a.CreatedAt = time.Unix(createdUnix, 0).UTC()
	a.UpdatedAt = time.Unix(updatedUnix, 0).UTC()
	return &a, nil
}
