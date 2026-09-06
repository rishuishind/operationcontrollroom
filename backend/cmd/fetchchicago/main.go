// Command fetchchicago pulls several real weeks of trips from the public
// Chicago Transportation Network Providers (TNC) Trips dataset
// (data.cityofchicago.org, dataset m6dm-c72p) for our 8 tracked community
// areas, and writes them as a CSV to stdout in the format internal/db
// ingests (trip_start_timestamp, pickup_community_area).
//
// This is a one-time data-preparation step, not something the running app
// does -- the resulting CSV is committed to the repo (data/chicago_trips.csv)
// so the server never needs live access to the Chicago Data Portal. Live,
// per-request external data (weather) is fetched by the running app itself;
// see internal/weather.
//
// Run: CHICAGO_APP_TOKEN=... go run ./cmd/fetchchicago > data/chicago_trips.csv
// (CHICAGO_APP_TOKEN is optional -- a free token from
// https://data.cityofchicago.org/profile/edit/developer_settings raises the
// unauthenticated rate limit, useful if you re-run this or widen the range.)
package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/rishabh/operation-control-room/internal/chicago"
	"github.com/rishabh/operation-control-room/internal/db"
)

// numWeeks controls how many distinct real weeks feed each
// (area, day-of-week, hour) baseline bucket -- i.e. hourly_baseline's
// sample_days. More weeks = a sturdier average, at the cost of a larger
// fetch and a bigger committed CSV. Currently 1 week (matches the
// committed data/chicago_trips.csv); raise this and re-run to widen the
// baseline.
const numWeeks = 1

func main() {
	areaIDs := make([]string, len(db.Areas))
	for i, a := range db.Areas {
		areaIDs[i] = strconv.Itoa(a.ID)
	}

	// The dataset's reporting window ends 2022-12-31. lastWeekStart is the
	// last full Mon-Sun week available before the Christmas/New Year
	// holidays (which would distort the baseline); we walk backwards from
	// there for numWeeks consecutive weeks.
	lastWeekStart := time.Date(2022, 12, 5, 0, 0, 0, 0, time.UTC)

	client := chicago.NewClient(os.Getenv("CHICAGO_APP_TOKEN"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	var all []chicago.RawTrip
	for i := numWeeks - 1; i >= 0; i-- {
		weekStart := lastWeekStart.AddDate(0, 0, -7*i)
		log.Printf("fetching week %d/%d (starting %s) for %d areas (168 small requests)...",
			numWeeks-i, numWeeks, weekStart.Format("2006-01-02"), len(areaIDs))

		trips, err := client.FetchWeek(ctx, areaIDs, weekStart, 8000)
		if err != nil {
			log.Fatalf("fetch week of %s: %v", weekStart.Format("2006-01-02"), err)
		}
		log.Printf("  -> %d real trip records", len(trips))
		all = append(all, trips...)
	}
	log.Printf("fetched %d real trip records across %d weeks", len(all), numWeeks)

	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	if err := w.Write([]string{"trip_start_timestamp", "pickup_community_area"}); err != nil {
		log.Fatal(err)
	}
	for _, t := range all {
		if err := w.Write([]string{t.Timestamp, t.AreaID}); err != nil {
			log.Fatal(err)
		}
	}
	fmt.Fprintf(os.Stderr, "wrote %d rows\n", len(all))
}
