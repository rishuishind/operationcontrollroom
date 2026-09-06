package recommend_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/rishabh/operation-control-room/internal/db"
	"github.com/rishabh/operation-control-room/internal/recommend"
	"github.com/rishabh/operation-control-room/internal/weather"
)

// failingWeather simulates the live external API being down.
type failingWeather struct{}

func (failingWeather) Current(ctx context.Context, lat, lon float64) (weather.Conditions, error) {
	return weather.Conditions{}, context.DeadlineExceeded
}

// TestBuildDegradesGracefullyOnWeatherFailure verifies that when the live
// weather API is unreachable, Build still returns a usable recommendation
// (baseline-only) instead of failing the whole request, and flags the
// degraded state so the frontend/operator knows the signal is incomplete.
func TestBuildDegradesGracefullyOnWeatherFailure(t *testing.T) {
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	if err := db.EnsureSeeded(conn, "../../data/chicago_trips.csv"); err != nil {
		t.Fatalf("seed: %v", err)
	}

	now := time.Date(2022, 12, 9, 17, 0, 0, 0, time.UTC) // a Friday, matches fetched week
	rec, err := recommend.Build(context.Background(), conn, failingWeather{}, now)
	if err != nil {
		t.Fatalf("Build returned error instead of degrading: %v", err)
	}

	if !rec.WeatherDegraded {
		t.Error("expected WeatherDegraded=true when weather API fails")
	}
	if rec.ActionNeeded {
		t.Error("expected no action recommended when weather is unavailable (multiplier falls back to 1.0 for every area)")
	}
	for _, a := range rec.Assessments {
		if a.WeatherOK {
			t.Errorf("area %s: expected WeatherOK=false with a failing weather source", a.AreaName)
		}
		if a.Multiplier != 1.0 {
			t.Errorf("area %s: expected multiplier 1.0 fallback, got %v", a.AreaName, a.Multiplier)
		}
	}
	if len(rec.Caveats) == 0 {
		t.Error("expected caveats to be non-empty")
	}
}
