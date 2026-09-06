package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestRankAreasForHour exercises the core SQL aggregation + ranking: it
// inserts raw trips across two dates for the same day-of-week/hour, rebuilds
// the hourly_baseline aggregation, and checks that RankAreasForHour returns
// areas ordered by average trip volume, computed correctly across days.
func TestRankAreasForHour(t *testing.T) {
	conn, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer conn.Close()

	if err := upsertAreas(conn); err != nil {
		t.Fatalf("upsertAreas: %v", err)
	}

	// Friday 2024-01-05 and Friday 2024-01-12 are both day_of_week=5.
	// Loop (32): 3 trips on the first Friday at 17:00, 5 on the second -> avg 4.
	// Austin (25): 1 trip on the first Friday at 17:00, 1 on the second -> avg 1.
	insertTrip(t, conn, "2024-01-05T17:05:00Z", 32)
	insertTrip(t, conn, "2024-01-05T17:15:00Z", 32)
	insertTrip(t, conn, "2024-01-05T17:45:00Z", 32)
	insertTrip(t, conn, "2024-01-12T17:05:00Z", 32)
	insertTrip(t, conn, "2024-01-12T17:15:00Z", 32)
	insertTrip(t, conn, "2024-01-12T17:25:00Z", 32)
	insertTrip(t, conn, "2024-01-12T17:35:00Z", 32)
	insertTrip(t, conn, "2024-01-12T17:55:00Z", 32)
	insertTrip(t, conn, "2024-01-05T17:10:00Z", 25)
	insertTrip(t, conn, "2024-01-12T17:10:00Z", 25)

	if err := RebuildHourlyBaseline(conn); err != nil {
		t.Fatalf("RebuildHourlyBaseline: %v", err)
	}

	rows, err := RankAreasForHour(conn, 5, 17) // Friday, 17:00
	if err != nil {
		t.Fatalf("RankAreasForHour: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 areas, got %d: %+v", len(rows), rows)
	}
	if rows[0].AreaID != 32 || rows[0].AvgTrips != 4 {
		t.Errorf("expected Loop (32) avg 4 first, got %+v", rows[0])
	}
	if rows[1].AreaID != 25 || rows[1].AvgTrips != 1 {
		t.Errorf("expected Austin (25) avg 1 second, got %+v", rows[1])
	}
	if rows[0].SampleDays != 2 {
		t.Errorf("expected sample_days=2 for Loop, got %d", rows[0].SampleDays)
	}
}

func insertTrip(t *testing.T, conn *sql.DB, ts string, areaID int) {
	t.Helper()
	_, err := conn.Exec(`INSERT INTO trips (trip_start_timestamp, pickup_community_area) VALUES (?, ?)`, ts, areaID)
	if err != nil {
		t.Fatalf("insert trip: %v", err)
	}
}
