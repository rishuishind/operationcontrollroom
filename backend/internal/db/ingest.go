package db

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
)

// EnsureSeeded loads community area reference data and, if the trips table
// is empty, ingests the CSV at csvPath and rebuilds the hourly_baseline
// aggregation. Safe to call on every server start: it's a no-op once data
// is present.
func EnsureSeeded(conn *sql.DB, csvPath string) error {
	if err := upsertAreas(conn); err != nil {
		return fmt.Errorf("seed community areas: %w", err)
	}

	var count int
	if err := conn.QueryRow(`SELECT COUNT(*) FROM trips`).Scan(&count); err != nil {
		return fmt.Errorf("count trips: %w", err)
	}
	if count > 0 {
		return nil
	}

	if err := IngestCSV(conn, csvPath); err != nil {
		return fmt.Errorf("ingest %s: %w", csvPath, err)
	}
	return RebuildHourlyBaseline(conn)
}

func upsertAreas(conn *sql.DB) error {
	stmt, err := conn.Prepare(`INSERT OR REPLACE INTO community_areas (id, name, lat, lon) VALUES (?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, a := range Areas {
		if _, err := stmt.Exec(a.ID, a.Name, a.Lat, a.Lon); err != nil {
			return err
		}
	}
	return nil
}

// IngestCSV bulk-loads a real trip extract (columns: trip_start_timestamp,
// pickup_community_area -- produced by cmd/fetchchicago directly from the
// live Chicago Data Portal) into the trips table. Rows referencing an
// unknown community area are skipped.
func IngestCSV(conn *sql.DB, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return fmt.Errorf("read header: %w", err)
	}
	if err := requireColumns(header, "trip_start_timestamp", "pickup_community_area"); err != nil {
		return err
	}

	known := make(map[string]bool, len(Areas))
	for _, a := range Areas {
		known[fmt.Sprintf("%d", a.ID)] = true
	}

	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO trips (trip_start_timestamp, pickup_community_area) VALUES (?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()

	rows := 0
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("read row %d: %w", rows, err)
		}
		if !known[rec[1]] {
			continue // unknown community area id, skip
		}
		if _, err := stmt.Exec(rec[0], rec[1]); err != nil {
			tx.Rollback()
			return fmt.Errorf("insert row %d: %w", rows, err)
		}
		rows++
	}
	return tx.Commit()
}

func requireColumns(header []string, want ...string) error {
	pos := make(map[string]int, len(header))
	for i, h := range header {
		pos[h] = i
	}
	for _, w := range want {
		if _, ok := pos[w]; !ok {
			return fmt.Errorf("csv missing required column %q", w)
		}
	}
	// Ingest currently assumes the exact column order produced by
	// cmd/fetchchicago; a stricter implementation would look up each
	// column by name.
	return nil
}

// RebuildHourlyBaseline recomputes the hourly_baseline table from trips.
//
// This is the SQL aggregation at the heart of the recommendation: for
// every (community area, calendar date, hour) we first count real trips,
// then average that count across every date sharing the same
// (area, day-of-week, hour). With one real week ingested, that average is
// currently a single real day's count per bucket -- see README for the
// trade-off and how to widen it (fetch more weeks).
func RebuildHourlyBaseline(conn *sql.DB) error {
	if _, err := conn.Exec(`DELETE FROM hourly_baseline`); err != nil {
		return err
	}
	_, err := conn.Exec(`
		INSERT INTO hourly_baseline (community_area_id, day_of_week, hour, avg_trips, sample_days)
		SELECT
			area_id,
			dow,
			hour,
			AVG(trip_count) AS avg_trips,
			COUNT(*)        AS sample_days
		FROM (
			SELECT
				pickup_community_area AS area_id,
				CAST(strftime('%w', trip_start_timestamp) AS INTEGER) AS dow,
				CAST(strftime('%H', trip_start_timestamp) AS INTEGER) AS hour,
				strftime('%Y-%m-%d', trip_start_timestamp) AS date,
				COUNT(*) AS trip_count
			FROM trips
			GROUP BY area_id, dow, hour, date
		) daily_counts
		GROUP BY area_id, dow, hour
	`)
	return err
}
