package db

import "database/sql"

// BaselineRow is one ranked community area for a given day-of-week/hour.
type BaselineRow struct {
	AreaID     int
	AreaName   string
	Lat        float64
	Lon        float64
	AvgTrips   float64
	SampleDays int
}

// RankAreasForHour returns every community area's baseline expected trip
// volume for the given day-of-week (0=Sun..6=Sat) and hour (0-23), ranked
// highest-demand first. This is the SQL ranking/comparison step: it joins
// the static community_areas reference table against the precomputed
// hourly_baseline aggregation and orders by avg_trips DESC.
func RankAreasForHour(conn *sql.DB, dayOfWeek, hour int) ([]BaselineRow, error) {
	rows, err := conn.Query(`
		SELECT ca.id, ca.name, ca.lat, ca.lon, hb.avg_trips, hb.sample_days
		FROM hourly_baseline hb
		JOIN community_areas ca ON ca.id = hb.community_area_id
		WHERE hb.day_of_week = ? AND hb.hour = ?
		ORDER BY hb.avg_trips DESC
	`, dayOfWeek, hour)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []BaselineRow
	for rows.Next() {
		var r BaselineRow
		if err := rows.Scan(&r.AreaID, &r.AreaName, &r.Lat, &r.Lon, &r.AvgTrips, &r.SampleDays); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
