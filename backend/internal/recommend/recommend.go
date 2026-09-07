// Package recommend combines the SQL-derived historical baseline with live
// weather to produce the single operator recommendation.
package recommend

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/rishabh/operation-control-room/internal/db"
	"github.com/rishabh/operation-control-room/internal/weather"
)

// ActionThreshold: an area is only flagged for driver reallocation if its
// weather-adjusted expected demand exceeds its baseline by at least this
// fraction. Below this, weather-driven noise isn't worth an operator
// disrupting driver positioning for -- so no intervention is recommended.
const ActionThreshold = 0.01

// WeatherSource is the subset of weather.Client used here, so tests can
// inject a fake that fails or returns fixed conditions.
type WeatherSource interface {
	Current(ctx context.Context, lat, lon float64) (weather.Conditions, error)
}

// AreaAssessment is one community area's baseline vs. weather-adjusted view.
type AreaAssessment struct {
	AreaID       int
	AreaName     string
	AvgTrips     float64 // historical baseline for this day-of-week/hour
	SampleDays   int
	WeatherOK    bool // false if the live weather call failed for this area
	Multiplier   float64
	Adjusted     float64 // AvgTrips * Multiplier
	ExcessRatio  float64 // (Adjusted - AvgTrips) / AvgTrips
	Weather      weather.Conditions
}

// Recommendation is the single decision returned to the operator.
type Recommendation struct {
	GeneratedAt   time.Time
	DayOfWeek     int
	Hour          int
	ActionNeeded  bool
	Target        *AreaAssessment
	Assessments   []AreaAssessment // all areas, ranked, for evidence display
	Threshold     float64
	WeatherDegraded bool // true if any area's live weather call failed
	Caveats       []string
}

// Build runs the SQL baseline ranking for the current day-of-week/hour,
// layers in live weather per area, and decides on a single recommendation.
// A weather API failure for an area degrades gracefully: that area falls
// back to its unadjusted baseline rather than failing the whole request.
func Build(ctx context.Context, conn *sql.DB, ws WeatherSource, now time.Time) (*Recommendation, error) {
	dow := int(now.Weekday())
	hour := now.Hour()

	rows, err := db.RankAreasForHour(conn, dow, hour)
	if err != nil {
		return nil, fmt.Errorf("rank areas: %w", err)
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no baseline data for day_of_week=%d hour=%d", dow, hour)
	}

	rec := assess(rows, ws, ctx, dow, hour, now)
	return rec, nil
}

func assess(rows []db.BaselineRow, ws WeatherSource, ctx context.Context, dow, hour int, now time.Time) *Recommendation {
	assessments := make([]AreaAssessment, 0, len(rows))
	degraded := false

	for _, row := range rows {
		a := AreaAssessment{
			AreaID:     row.AreaID,
			AreaName:   row.AreaName,
			AvgTrips:   row.AvgTrips,
			SampleDays: row.SampleDays,
			Multiplier: 1.0,
			Adjusted:   row.AvgTrips,
		}

		cond, err := ws.Current(ctx, 28.6139,77.2090)
		fmt.Printf("current codtion of %v is perciMM: %v and Probability: %v \n",a.AreaName,cond.PrecipitationMM,cond.PrecipProbability)
		if err != nil {
			degraded = true
		} else {
			a.WeatherOK = true
			a.Weather = cond
			a.Multiplier = WeatherMultiplier(cond.PrecipitationMM, cond.PrecipProbability)
			a.Adjusted = row.AvgTrips * a.Multiplier
		}
		if row.AvgTrips > 0 {
			a.ExcessRatio = (a.Adjusted - row.AvgTrips) / row.AvgTrips
		}
		assessments = append(assessments, a)
	}

	// Rank by excess ratio (weather-driven demand spike relative to that
	// area's own baseline) to find the single most urgent area.
	best := 0
	for i, a := range assessments {
		if a.ExcessRatio > assessments[best].ExcessRatio {
			best = i
		}
	}

	rec := &Recommendation{
		GeneratedAt:     now,
		DayOfWeek:       dow,
		Hour:            hour,
		Assessments:     assessments,
		Threshold:       ActionThreshold,
		WeatherDegraded: degraded,
	}

	if assessments[best].ExcessRatio >= ActionThreshold {
		target := assessments[best]
		rec.ActionNeeded = true
		rec.Target = &target
	}

	rec.Caveats = buildCaveats(rec)
	return rec
}

func buildCaveats(rec *Recommendation) []string {
	caveats := []string{
		"The historical baseline is built from one real week (Dec 5-11, 2022) of the public Chicago TNC dataset, not a full multi-week census -- each (area, day-of-week, hour) bucket has only 1 real sample, so a single unusual day fully determines that bucket's baseline.",
		"The weather-to-demand multiplier is a generic heuristic (higher precipitation probability/intensity -> higher demand), not calibrated against this dataset's own weather history, since trips aren't tagged with historical weather.",
		"This is a demand-side signal only: it does not account for current driver supply, active surge pricing, or road closures/events.",
		"Weather is always the live current/forecast conditions, not historical weather for the baseline's date -- so the comparison is 'this day/hour's typical demand vs. today's actual weather', not a historically matched pair.",
		"Open-Meteo's hourly forecast is matched to the current local hour by array index, not by exact timestamp; near hour boundaries this can be off by up to an hour.",
	}
	if rec.WeatherDegraded {
		caveats = append([]string{"Live weather was unavailable for at least one area; those areas fall back to baseline-only (no weather adjustment), which may understate their true demand."}, caveats...)
	}
	return caveats
}
