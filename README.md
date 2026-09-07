# Operations Control Room

A small, working web app that gives a Chicago rideshare marketplace operator
**one recommendation**: which community area, if any, needs more drivers
dispatched right now — combining a SQL-derived historical demand baseline
(built from **real** Chicago trip data) with **live** weather.

> Case study: *Take Home Exercise — Operations Control Room (Full Stack)*.
> This is a ~8-10 hour scoped prototype, not a polished product. See
> [Assumptions, trade-offs & limitations](#assumptions-trade-offs--limitations).

---

## What it does

- **One recommendation, not a dashboard.** The app shows a single headline:
  either "dispatch more drivers to X" or "no intervention needed" — plus the
  evidence and uncertainty behind that call.
- **Historical baseline (SQL, real data):** for every community area /
  day-of-week / hour, we compute the average historical trip volume from a
  real extract of the public [Chicago Transportation Network Providers
  (TNC) Trips
  dataset](https://data.cityofchicago.org/Transportation/Transportation-Network-Providers-Trips/m6dm-c72p).
- **Live context (Open-Meteo):** current precipitation and forecasted
  precipitation probability at each area's centroid, fetched fresh on every
  request.
- **Combination:** live weather is converted into a demand multiplier applied
  to the baseline. The area with the largest weather-driven excess over its
  own baseline — if it clears a threshold — is the recommendation.

Both data sources required by the case study are real, live external
sources — not mocked: Open-Meteo is called live per request; the Chicago
data is a real extract fetched directly from the City of Chicago's public
API (see below for why it's a bounded extract rather than the full ~200M
row dataset).

---

## Quick start

Requires **Go 1.24+** and **Node 20+** (no Docker, no API keys required to
run the app).

```bash
# Terminal 1 — backend (serves http://localhost:8080)
cd backend
go run ./cmd/server

# Terminal 2 — frontend (serves http://localhost:3000)
cd frontend
npm install
npm run dev
```

Open http://localhost:3000. On first run the backend automatically creates
`backend/control_room.db` (SQLite) and seeds it from the bundled real trip
extract `backend/data/chicago_trips.csv` — no manual migration/ingest step,
and no network access needed to run the app (the Chicago data was already
fetched once and committed; only the weather call is live).

Run tests:

```bash
cd backend && go test ./...
```

### Regenerating the Chicago data extract (optional)

`backend/data/chicago_trips.csv` was produced by, and can be regenerated
with:

```bash
cd backend
CHICAGO_APP_TOKEN=<your free token> go run ./cmd/fetchchicago > data/chicago_trips.csv
```

`CHICAGO_APP_TOKEN` is optional (a free, self-service token from
[the Chicago Data Portal](https://data.cityofchicago.org/profile/edit/developer_settings)
raises the unauthenticated rate limit) but recommended if you re-run this.
This hits the live City of Chicago API directly — it is **not** run by the
server or required to use the app; see "Why a bounded real extract" below
for why this isn't done automatically at every server start.

### Configuration (optional)

| Var | Default | Purpose |
|---|---|---|
| `ADDR` (backend) | `:8080` | HTTP listen address |
| `DB_PATH` (backend) | `control_room.db` | SQLite file path |
| `SEED_CSV` (backend) | `data/chicago_trips.csv` | Trip CSV to ingest on first run |
| `CHICAGO_APP_TOKEN` (cmd/fetchchicago only) | none | Optional Socrata app token, see above |
| `NEXT_PUBLIC_API_BASE_URL` (frontend, in `frontend/.env.local`) | `http://localhost:8080` | Backend URL the UI calls |

---

## Architecture overview

```
frontend/ (Next.js + TypeScript, App Router)
  src/app/page.tsx        single-page UI: recommendation, evidence, caveats
  src/lib/types.ts        response types shared with the Go API's JSON shape

backend/ (Go, Gin)
  cmd/server/main.go      wiring: opens DB, seeds it, starts the Gin server
  cmd/fetchchicago/       one-time script: pulls real trips from the live
                          Chicago Data Portal API into data/chicago_trips.csv
  internal/db/            SQLite connection, schema migration, CSV ingest,
                           the SQL aggregation + ranking queries
  internal/chicago/       Socrata (Chicago Data Portal) HTTP client
  internal/weather/       Open-Meteo HTTP client (the live, per-request API)
  internal/recommend/     the decision logic: baseline + weather -> Recommendation
  internal/api/           Gin HTTP handlers (GET /api/recommendation, /api/health)
```

Request flow for `GET /api/recommendation`:

1. Determine current day-of-week/hour (or `?at=<RFC3339>` for demoing a
   specific moment, e.g. `?at=2022-12-09T17:00:00Z` — a Friday matching the
   fetched real week, useful since the bundled baseline data is from Dec
   2022). Note weather is *always* the live current/forecast conditions,
   regardless of `?at=` — only the baseline lookup is backdated.
2. `db.RankAreasForHour` — SQL query joining `community_areas` to the
   precomputed `hourly_baseline` table, ranked by baseline demand.
3. For each area, call Open-Meteo for current conditions. A per-area failure
   doesn't fail the request — that area just falls back to baseline-only
   (see `internal/recommend/recommend_test.go`).
4. `recommend.WeatherMultiplier` turns precipitation into a demand
   multiplier; the area with the largest excess over its own baseline,
   if it clears `ActionThreshold` (15%), becomes the target.
5. The full ranked list, the target (if any), and a list of caveats are
   returned as JSON and rendered by the frontend.

---

## Database design & important SQL

Three tables (`backend/internal/db/migrations/0001_init.sql`):

- **`community_areas`** — static reference data: Chicago community area id,
  name, and a lat/lon centroid (used to query weather).
- **`trips`** — real trip rows (`trip_start_timestamp`,
  `pickup_community_area`), fetched from the live Chicago Data Portal API
  by `cmd/fetchchicago` (see below for scope).
- **`hourly_baseline`** — materialized: expected trip volume per
  `(community_area_id, day_of_week, hour)`. Populated by the aggregation
  below, not written to directly.

**The core aggregation** (`internal/db/ingest.go`, `RebuildHourlyBaseline`) —
this is the "meaningful aggregation performed in SQL" requirement:

```sql
INSERT INTO hourly_baseline (community_area_id, day_of_week, hour, avg_trips, sample_days)
SELECT
    area_id, dow, hour,
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
GROUP BY area_id, dow, hour;
```

It's a two-level `GROUP BY`: first count real trips per area/date/hour, then
average that count across every date sharing the same day-of-week/hour.
That answers "how many trips does the Loop typically see on a Friday at
5pm?" — the baseline the live weather signal is compared against.

**The ranking query** (`internal/db/query.go`, `RankAreasForHour`) — the
"comparison/ranking" requirement:

```sql
SELECT ca.id, ca.name, ca.lat, ca.lon, hb.avg_trips, hb.sample_days
FROM hourly_baseline hb
JOIN community_areas ca ON ca.id = hb.community_area_id
WHERE hb.day_of_week = ? AND hb.hour = ?
ORDER BY hb.avg_trips DESC;
```

Weather adjustment and picking the single target area happen in Go
(`internal/recommend`), since they depend on a live external API call per
area — not something SQL alone can do.

---

## Why a bounded real extract, not the full dataset

The public Chicago TNC dataset is ~200M individual trip records. Two things
ruled out using it directly at either server-start or request time:

1. **Size.** Even a single busy community area can have 20,000-30,000 trips
   in one day; pulling the full dataset, or even a full multi-week census
   across all tracked areas, would mean gigabytes of data — impractical to
   fetch, store, or bundle for a local demo.
2. **The live query API's own performance.** During development, I tried
   pushing the counting/aggregation down to Socrata (the platform the
   Chicago Data Portal is built on) via `count(*) ... GROUP BY` queries.
   These were unreliably slow — 40 to 150+ seconds per query, and several
   timed out outright, even with an app token (which raises rate limits,
   not query speed). That ruled out both "aggregate live at request time"
   and "aggregate the whole dataset once via Socrata."

What worked reliably: **plain filtered, ordered, non-aggregated row
fetches** are fast (~10,000 real rows in ~10-20s). So `cmd/fetchchicago`
(`internal/chicago`) pulls real raw `(pickup_community_area,
trip_start_timestamp)` pairs for our 8 tracked community areas across **one
real week** (2022-12-05 to 2022-12-11 — the last full week before this
dataset's reporting window ends, avoiding the Christmas/New Year holiday
distortion just after it), issuing one small request per hour (168 requests,
run with bounded concurrency) rather than one request for the whole range.
This produced **762,038 real trip records in about 30 seconds**. All
aggregation on that data (the `GROUP BY` above) happens locally in our own
SQLite, which has no such performance problem.

This is committed as `backend/data/chicago_trips.csv` (~20MB) so the running
app never needs live access to the Chicago Data Portal — only Open-Meteo is
called live, matching the assignment's "one live external API" scope.

---

## Assumptions, trade-offs & limitations

**One real week, not a multi-week census.** Each `(area, day-of-week, hour)`
baseline bucket currently has exactly **1 real sample day** behind it (`sample_days: 1`
in the API response) — a single unusually busy or quiet day fully determines
that bucket. Widening this to several weeks is straightforward (re-run
`cmd/fetchchicago` with a longer loop over more weeks) but was left out to
keep the fetch fast and the committed CSV small; it's the single biggest
lever for improving baseline quality.

**Weather-to-demand multiplier.** The heuristic
(`internal/recommend/heuristic.go`) assumes precipitation increases
rideshare demand — well documented in industry generally — but the
multiplier's specific shape (`1.0 + 0.5×prob + 0.03×mm`, capped at 1.6) is a
reasonable guess, not calibrated against this dataset, because trips aren't
tagged with the weather at the time they occurred. A production version
would want historical weather joined to historical trips to fit this
properly.

**Weather is always "now," baseline is always Dec 2022.** The `?at=`
override changes which baseline bucket is compared, but Open-Meteo has no
historical mode here — the live call always returns current/forecast
conditions. The product framing is intentionally "how this day/hour
typically behaves vs. today's actual weather," not a historically matched
pair.

**Demand-only signal.** The recommendation has no visibility into actual
driver supply, active surge pricing, or one-off events/road closures. It
answers "where might demand spike" — an operator still has to know current
supply to decide if action is truly needed.

**Hour alignment.** Open-Meteo's hourly forecast array is indexed by the
current local hour rather than matched against exact timestamps, so
near an hour boundary the "current hour" reading can be off by up to an
hour.

**Threshold choice.** The 15% excess-over-baseline threshold for flagging an
area (`recommend.ActionThreshold`) is a judgment call, not derived from data
— it exists so ordinary weather noise doesn't cause the app to flag
something on every request.

All of the above are also surfaced live in the app's "What could make this
wrong" panel.

---

## Tests

`backend/internal/db/query_test.go` — `TestRankAreasForHour`: seeds raw
trips across two dates for the same day-of-week/hour, rebuilds the baseline
aggregation, and asserts the SQL ranking orders areas correctly and computes
the average across days correctly.

`backend/internal/recommend/heuristic_test.go` — `TestWeatherMultiplier`:
table-driven test of the pure weather→multiplier function, including
clamping of out-of-range inputs and the multiplier cap.

`backend/internal/recommend/recommend_test.go` —
`TestBuildDegradesGracefullyOnWeatherFailure`: seeds the real Chicago
extract, injects a `WeatherSource` that always errors, and asserts the
recommendation still builds successfully, falls back to baseline-only for
every area, and flags `WeatherDegraded`.

---

## Screenshots

_TODO: add screenshots/GIF of the running app (see "Quick start" above to
run it locally first)._

---

## AI-use disclosure

**Tool used:** Claude Code (Anthropic), used to scaffold the initial
end-to-end prototype (Go/Gin backend, SQLite schema/aggregation, real
Chicago Data Portal ingestion, Next.js frontend) from the case study PDF in
a single session, and then iterated with me across many follow-up rounds
rather than in one shot.

**Suggestions I accepted:** the overall architecture (Go/Gin + SQLite +
Next.js), the two-level `GROUP BY` aggregation design, and the per-hour
bounded fetch strategy for the Chicago data once it was proven fast in
practice (see below).

**Notable mid-session correction (accepted):** the first pass used
synthetic (generated, not real) trip data and a naive design that would
have tried to aggregate the full ~200M-row Chicago dataset live via the
Socrata query API. Both were wrong: synthetic data doesn't satisfy "use a
public mobility dataset," and the live aggregation approach turned out to
be unreliably slow (40-150+ second queries, frequent timeouts) even with an
app token. I pushed back on both and required real Gin + real data instead
of the stdlib/synthetic-data starting point. The fix — fetch real but
bounded raw rows fast, aggregate locally in SQL — came from actually timing
several query shapes against the live API rather than assuming the first
approach would scale.

**Suggestion I rejected: widening the baseline to 4 weeks of real data.** I
initially asked for this to make `sample_days` less thin (currently 1 real
day per bucket). Partway through the second fetch attempt (the first had
crashed on a request timeout), I stopped it — a 4x larger fetch and a much
bigger committed CSV felt like the wrong trade-off for an 8-10 hour scoped
prototype, and I wasn't confident the marginal baseline quality was worth
the added fetch fragility and repo bloat. I reverted `cmd/fetchchicago` to
`numWeeks = 1` and kept the original real one-week extract
(`chicago_trips.csv`, 762,038 rows). This is called out explicitly in
"Assumptions, trade-offs & limitations" above as the single biggest lever
for improving baseline quality if I had more time.

**Suggestion I was skeptical of, then verified hands-on: the "no action
needed" result.** The app consistently showed "no action needed" in my own
testing. Rather than assume the recommendation logic was broken, I had
Claude Code walk me through the `assess()` function line by line and
confirmed the real cause: actual Chicago weather at the time was genuinely
dry (0% precipitation probability everywhere), so every area's
`ExcessRatio` was legitimately near zero — "no action needed" was the
*correct* output, not a bug. To be able to demonstrate the other branch on
demand (for this README and the interview), I asked for an explicitly
opt-in, clearly-labeled demo override (`?demo_rain_area=<id>`) that fakes
heavy rain for exactly one requested area while leaving every other area's
weather live and real. It never activates unless a request asks for it,
and I verified via curl that it correctly flips `ActionNeeded` to `true`
without touching the real recommendation path.

**How I validated the generated code:**
- Read through the Go source myself and had Claude Code explain specific
  logic in detail before trusting it — in particular the `assess()`
  function, `WeatherMultiplier`'s formula and arguments, and what
  `ActionThreshold` and `sample_days` actually mean and where their values
  come from (i.e., which are principled vs. arbitrary judgment calls).
- Ran the app locally and connected DBeaver directly to the SQLite file
  (`backend/control_room.db`) to inspect the real `trips` and
  `hourly_baseline` tables myself, rather than trusting the API response
  alone.

**Most technically challenging part:** the weather-multiplier and
action-threshold logic (`internal/recommend/heuristic.go` and
`recommend.go`). Unlike the SQL aggregation or the data-fetch plumbing,
which are mechanically verifiable (run it, check the numbers), this part
is a heuristic with no ground truth to check it against — trips in the
dataset aren't tagged with the weather at the time they happened, so there
is no way to backtest whether `1.0 + 0.5×precip_probability + 0.03×mm`
(capped at 1.6) or the 15% action threshold are "right." Reasoning about
this meant separating what's defensible (precipitation increasing rideshare
demand is well-documented) from what's an arbitrary but reasonable
placeholder (the specific constants and cap) — and being able to say so
plainly in an interview instead of overstating the model's rigor.
