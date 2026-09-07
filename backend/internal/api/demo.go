package api

import (
	"context"
	"strconv"

	"github.com/rishabh/operation-control-room/internal/db"
	"github.com/rishabh/operation-control-room/internal/recommend"
	"github.com/rishabh/operation-control-room/internal/weather"
)

// demoRainWeather is a DEMO-ONLY wrapper, opt-in via
// GET /api/recommendation?demo_rain_area=<community area id>. It fakes
// heavy rain for exactly the one requested area (identified by matching its
// known lat/lon), leaving every other area's weather live and real.
//
// It exists solely so the "action recommended" UI state can be demonstrated
// on demand -- real Chicago weather is often dry, in which case every
// area's ExcessRatio is genuinely 0 and "no action needed" is the correct,
// non-faked answer. This wrapper never activates unless a request
// explicitly asks for it via the query param.
type demoRainWeather struct {
	inner                recommend.WeatherSource
	targetLat, targetLon float64
}

func (d demoRainWeather) Current(ctx context.Context, lat, lon float64) (weather.Conditions, error) {
	if lat == d.targetLat && lon == d.targetLon {
		return weather.Conditions{
			TemperatureC:      10,
			PrecipitationMM:   6,
			PrecipProbability: 95,
			WindSpeedKMH:      25,
			WeatherCode:       65, // Open-Meteo code for heavy rain
		}, nil
	}
	return d.inner.Current(ctx, lat, lon)
}

// withDemoRain returns ws unchanged if areaIDParam is empty or doesn't match
// one of our tracked community areas; otherwise it wraps ws so that one
// area's weather is faked as heavy rain.
func withDemoRain(ws recommend.WeatherSource, areaIDParam string) recommend.WeatherSource {
	if areaIDParam == "" {
		return ws
	}
	id, err := strconv.Atoi(areaIDParam)
	if err != nil {
		return ws
	}
	for _, a := range db.Areas {
		if a.ID == id {
			return demoRainWeather{inner: ws, targetLat: a.Lat, targetLon: a.Lon}
		}
	}
	return ws
}
