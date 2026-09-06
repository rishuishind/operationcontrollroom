// Package weather fetches the live current-conditions context from
// Open-Meteo (no API key required).
package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// Conditions is the subset of Open-Meteo's current-weather response we use.
type Conditions struct {
	TemperatureC      float64
	PrecipitationMM   float64
	PrecipProbability float64 // 0-100, from the current hour of the hourly forecast
	WindSpeedKMH      float64
	WeatherCode       int
}

// Client calls the Open-Meteo forecast API.
type Client struct {
	HTTP    *http.Client
	BaseURL string // override in tests
}

func NewClient() *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 5 * time.Second},
		BaseURL: "https://api.open-meteo.com/v1/forecast",
	}
}

type openMeteoResponse struct {
	Current struct {
		Temperature2m   float64 `json:"temperature_2m"`
		Precipitation   float64 `json:"precipitation"`
		WindSpeed10m    float64 `json:"wind_speed_10m"`
		WeatherCode     int     `json:"weather_code"`
	} `json:"current"`
	Hourly struct {
		Time                     []string  `json:"time"`
		PrecipitationProbability []float64 `json:"precipitation_probability"`
	} `json:"hourly"`
}

// Current fetches current conditions plus the current hour's precipitation
// probability for the given coordinates.
func (c *Client) Current(ctx context.Context, lat, lon float64) (Conditions, error) {
	q := url.Values{}
	q.Set("latitude", fmt.Sprintf("%.4f", lat))
	q.Set("longitude", fmt.Sprintf("%.4f", lon))
	q.Set("current", "temperature_2m,precipitation,wind_speed_10m,weather_code")
	q.Set("hourly", "precipitation_probability")
	q.Set("forecast_days", "1")
	q.Set("timezone", "auto")

	reqURL := c.BaseURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return Conditions{}, err
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return Conditions{}, fmt.Errorf("open-meteo request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Conditions{}, fmt.Errorf("open-meteo returned status %d", resp.StatusCode)
	}

	var body openMeteoResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Conditions{}, fmt.Errorf("decode open-meteo response: %w", err)
	}

	cond := Conditions{
		TemperatureC:    body.Current.Temperature2m,
		PrecipitationMM: body.Current.Precipitation,
		WindSpeedKMH:    body.Current.WindSpeed10m,
		WeatherCode:     body.Current.WeatherCode,
	}
	if len(body.Hourly.PrecipitationProbability) > 0 {
		// Hourly arrays are ordered by time starting at hour 0 of "today" in
		// the requested timezone; the current hour is a reasonable-enough
		// index without matching timestamps exactly.
		now := time.Now()
		idx := now.Hour()
		if idx < len(body.Hourly.PrecipitationProbability) {
			cond.PrecipProbability = body.Hourly.PrecipitationProbability[idx]
		}
	}
	return cond, nil
}
