// Package storage opens the bass SQLite database with the pragmas required
// for safe concurrent use.
package storage

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Open opens the SQLite database at path and applies the standard pragmas:
// WAL journal mode (concurrent reads alongside one writer), foreign keys,
// busy timeout, normal sync. Returns the *sql.DB ready to use.
func Open(path string) (*sql.DB, error) {
	// The DSN-style query params on the modernc.org/sqlite driver let us
	// set busy_timeout at connection time so we don't have to per-conn-PRAGMA.
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite handles serialisation internally; one writer suffices.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	return db, nil
}
