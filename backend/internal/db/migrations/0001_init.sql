-- Operations Control Room: initial schema
-- 3 tables: community_areas (reference), trips (real raw trip extract),
-- hourly_baseline (materialized aggregation computed FROM trips via SQL).

CREATE TABLE IF NOT EXISTS community_areas (
    id   INTEGER PRIMARY KEY,      -- Chicago community area number
    name TEXT NOT NULL,
    lat  REAL NOT NULL,
    lon  REAL NOT NULL
);

-- Real trip records pulled from the public Chicago Transportation Network
-- Providers (TNC) Trips dataset (data.cityofchicago.org, dataset
-- m6dm-c72p) via cmd/fetchchicago -- one real week (2022-12-05..12-11,
-- the most recent full week before the dataset's reporting window ends)
-- across our 8 tracked community areas. Only the two columns our
-- aggregation needs are pulled (pickup_community_area,
-- trip_start_timestamp); see README "Assumptions & Limitations" for why
-- this is a real slice rather than a full census.
CREATE TABLE IF NOT EXISTS trips (
    id                    INTEGER PRIMARY KEY AUTOINCREMENT,
    trip_start_timestamp  DATETIME NOT NULL,
    pickup_community_area INTEGER NOT NULL REFERENCES community_areas(id)
);

CREATE INDEX IF NOT EXISTS idx_trips_area_time
    ON trips (pickup_community_area, trip_start_timestamp);

-- Materialized baseline: expected trip volume per community area, per
-- day-of-week (0=Sunday..6=Saturday), per hour-of-day (0-23).
-- Populated by a SQL aggregation (GROUP BY) over `trips` -- see
-- internal/db/ingest.go: RebuildHourlyBaseline.
CREATE TABLE IF NOT EXISTS hourly_baseline (
    community_area_id INTEGER NOT NULL REFERENCES community_areas(id),
    day_of_week       INTEGER NOT NULL, -- 0=Sun .. 6=Sat (SQLite strftime('%w'))
    hour              INTEGER NOT NULL, -- 0-23
    avg_trips         REAL NOT NULL,
    sample_days       INTEGER NOT NULL, -- how many distinct real dates contributed
    PRIMARY KEY (community_area_id, day_of_week, hour)
);
