// Package chicago fetches a small, real slice of the City of Chicago's
// public Transportation Network Providers (TNC) Trips dataset from the
// Socrata Open Data API (SODA) at data.cityofchicago.org.
//
// The full dataset is ~200M individual trip records, and its live query
// engine is unreliably slow for server-side aggregation (count/group-by
// queries against it routinely took 40-150+ seconds or timed out entirely
// during development, even with an app token). Plain filtered, ordered,
// non-aggregated row fetches are fast and reliable (~10s for 10k rows), so
// this client pulls real raw rows -- just the two fields we need,
// trip_start_timestamp and pickup_community_area -- in small windows, and
// our own SQL does the counting/averaging locally (see internal/db).
package chicago

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const datasetEndpoint = "https://data.cityofchicago.org/resource/m6dm-c72p.json"

// RawTrip is one real trip record, reduced to the two fields our
// recommendation logic needs.
type RawTrip struct {
	AreaID    string `json:"pickup_community_area"`
	Timestamp string `json:"trip_start_timestamp"` // e.g. "2022-12-05T08:15:00.000"
}

type Client struct {
	HTTP *http.Client
	// AppToken is an optional Socrata app token (X-App-Token header) --
	// free and self-service at
	// https://data.cityofchicago.org/profile/edit/developer_settings.
	// It raises the unauthenticated rate limit; it does not meaningfully
	// speed up individual queries.
	AppToken string
}

func NewClient(appToken string) *Client {
	return &Client{
		HTTP:     &http.Client{Timeout: 45 * time.Second},
		AppToken: appToken,
	}
}

// FetchWeek pulls every real trip for the given community areas across the
// 7-day window starting at weekStart (which should be UTC-midnight), one
// small request per hour (168 total, all areas combined per request) to
// stay inside the query engine's fast path. Requests run with bounded
// concurrency. limitPerHour should comfortably exceed the true combined
// hourly volume across all requested areas so results aren't chronologically
// truncated within the hour (8000 comfortably covers our 8 tracked areas).
func (c *Client) FetchWeek(ctx context.Context, areaIDs []string, weekStart time.Time, limitPerHour int) ([]RawTrip, error) {
	type job struct{ hourStart time.Time }
	jobs := make(chan job)
	results := make(chan []RawTrip)
	errs := make(chan error, 1)

	const concurrency = 10
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				rows, err := c.fetchHourWithRetry(ctx, areaIDs, j.hourStart, limitPerHour)
				if err != nil {
					select {
					case errs <- fmt.Errorf("fetch hour %s: %w", j.hourStart.Format(time.RFC3339), err):
					default:
					}
					return
				}
				results <- rows
			}
		}()
	}

	go func() {
		for d := 0; d < 7; d++ {
			for h := 0; h < 24; h++ {
				jobs <- job{hourStart: weekStart.AddDate(0, 0, d).Add(time.Duration(h) * time.Hour)}
			}
		}
		close(jobs)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	var all []RawTrip
	for rows := range results {
		all = append(all, rows...)
	}
	select {
	case err := <-errs:
		return all, err
	default:
		return all, nil
	}
}

// fetchHourWithRetry retries a handful of times on failure. The live
// Chicago Data Portal API has unpredictable per-request latency -- most
// requests finish in a few seconds, but occasionally one hangs and times
// out for no discernible reason. A retry (rather than failing the whole
// multi-hundred-request fetch over one bad request) reliably gets it on
// the 2nd or 3rd attempt in practice.
func (c *Client) fetchHourWithRetry(ctx context.Context, areaIDs []string, hourStart time.Time, limit int) ([]RawTrip, error) {
	const maxAttempts = 4
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		rows, err := c.fetchHour(ctx, areaIDs, hourStart, limit)
		if err == nil {
			return rows, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return nil, fmt.Errorf("after %d attempts: %w", maxAttempts, lastErr)
}

func (c *Client) fetchHour(ctx context.Context, areaIDs []string, hourStart time.Time, limit int) ([]RawTrip, error) {
	hourEnd := hourStart.Add(time.Hour)
	areaFilter := "'" + strings.Join(areaIDs, "','") + "'"

	q := url.Values{}
	q.Set("$select", "pickup_community_area,trip_start_timestamp")
	q.Set("$where", fmt.Sprintf("trip_start_timestamp between '%s' and '%s' and pickup_community_area in(%s)",
		hourStart.Format("2006-01-02T15:04:05"), hourEnd.Format("2006-01-02T15:04:05"), areaFilter))
	q.Set("$order", "trip_start_timestamp")
	q.Set("$limit", fmt.Sprintf("%d", limit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, datasetEndpoint+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	if c.AppToken != "" {
		req.Header.Set("X-App-Token", c.AppToken)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("socrata returned status %d for hour %s", resp.StatusCode, hourStart.Format(time.RFC3339))
	}

	var rows []RawTrip
	if err := json.NewDecoder(resp.Body).Decode(&rows); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return rows, nil
}
