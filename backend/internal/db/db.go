// Package db owns the SQLite connection, schema migration, and the CSV
// ingest + SQL aggregation that builds the hourly_baseline table.
package db

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed migrations/0001_init.sql
var schemaSQL string

// Open connects to the SQLite file at path (created if absent) and applies
// the schema migration.
func Open(path string) (*sql.DB, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1) // avoid SQLite "database is locked" under concurrent writes

	if _, err := conn.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	if err := migrate(conn); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return conn, nil
}

func migrate(conn *sql.DB) error {
	_, err := conn.Exec(schemaSQL)
	return err
}
