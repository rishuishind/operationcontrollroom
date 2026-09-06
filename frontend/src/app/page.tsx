"use client";

import { useEffect, useState, useCallback } from "react";
import type { Recommendation } from "@/lib/types";

const API_BASE = process.env.NEXT_PUBLIC_API_BASE_URL ?? "http://localhost:8080";

const DOW_NAMES = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];

function formatHour(hour: number): string {
  const h = hour % 12 === 0 ? 12 : hour % 12;
  return `${h}${hour < 12 ? "am" : "pm"}`;
}

export default function Home() {
  const [rec, setRec] = useState<Recommendation | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const res = await fetch(`${API_BASE}/api/recommendation`, { cache: "no-store" });
      if (!res.ok) throw new Error(`API returned ${res.status}`);
      const body = await res.json() 
      setRec(body);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load recommendation");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div className="min-h-screen bg-zinc-50 dark:bg-zinc-950 text-zinc-900 dark:text-zinc-50">
      <main className="mx-auto max-w-3xl px-6 py-12">
        <header className="mb-8 flex items-center justify-between">
          <div>
            <p className="text-sm font-medium uppercase tracking-wide text-zinc-500">
              Operations Control Room
            </p>
            <h1 className="text-2xl font-semibold">Right now, in Chicago</h1>
          </div>
          <button
            onClick={load}
            disabled={loading}
            className="rounded-full border border-zinc-300 dark:border-zinc-700 px-4 py-2 text-sm font-medium hover:bg-zinc-100 dark:hover:bg-zinc-900 disabled:opacity-50"
          >
            {loading ? "Refreshing…" : "Refresh"}
          </button>
        </header>

        {error && (
          <div className="rounded-xl border border-red-300 bg-red-50 dark:bg-red-950/40 dark:border-red-800 p-4 text-red-800 dark:text-red-200 mb-6">
            <p className="font-medium">Couldn&apos;t load a recommendation.</p>
            <p className="text-sm mt-1">{error}</p>
            <p className="text-sm mt-1">Is the Go backend running at {API_BASE}?</p>
          </div>
        )}

        {loading && !rec && (
          <div className="rounded-xl border border-zinc-200 dark:border-zinc-800 p-8 text-center text-zinc-500">
            Loading…
          </div>
        )}

        {rec && <RecommendationCard rec={rec} />}
      </main>
    </div>
  );
}

function RecommendationCard({ rec }: { rec: Recommendation }) {
  const timeLabel = `${DOW_NAMES[rec.DayOfWeek]} ${formatHour(rec.Hour)}`;

  return (
    <div className="space-y-6">
      <section
        className={`rounded-2xl border p-6 ${
          rec.ActionNeeded
            ? "border-amber-400 bg-amber-50 dark:bg-amber-950/30 dark:border-amber-700"
            : "border-emerald-400 bg-emerald-50 dark:bg-emerald-950/30 dark:border-emerald-700"
        }`}
      >
        {rec.ActionNeeded && rec.Target ? (
          <>
            <p className="text-xs font-semibold uppercase tracking-wide text-amber-700 dark:text-amber-400">
              Action recommended · {timeLabel}
            </p>
            <h2 className="text-xl font-semibold mt-1">
              Dispatch more drivers to {rec.Target.AreaName}
            </h2>
            <p className="mt-2 text-sm leading-relaxed text-zinc-700 dark:text-zinc-300">
              {rec.Target.AreaName} typically sees{" "}
              <strong>{rec.Target.AvgTrips.toFixed(1)} trips/hour</strong> at this day and hour.
              Current weather conditions (
              {rec.Target.Weather.PrecipProbability}% chance of precipitation
              {rec.Target.Weather.PrecipitationMM > 0
                ? `, ${rec.Target.Weather.PrecipitationMM.toFixed(1)}mm currently falling`
                : ""}
              ) suggest demand could rise to{" "}
              <strong>{rec.Target.Adjusted.toFixed(1)} trips/hour</strong> — a{" "}
              <strong>{(rec.Target.ExcessRatio * 100).toFixed(0)}%</strong> increase over baseline,
              above the {(rec.Threshold * 100).toFixed(0)}% threshold for flagging an area.
            </p>
          </>
        ) : (
          <>
            <p className="text-xs font-semibold uppercase tracking-wide text-emerald-700 dark:text-emerald-400">
              No action needed · {timeLabel}
            </p>
            <h2 className="text-xl font-semibold mt-1">
              No area currently needs driver reallocation
            </h2>
            <p className="mt-2 text-sm leading-relaxed text-zinc-700 dark:text-zinc-300">
              Weather-adjusted demand is within {(rec.Threshold * 100).toFixed(0)}% of the
              historical baseline in every tracked community area. Current conditions and
              baseline evidence are shown below.
            </p>
          </>
        )}
      </section>

      <Evidence rec={rec} />
      <Caveats rec={rec} />
    </div>
  );
}

function Evidence({ rec }: { rec: Recommendation }) {
  const maxAdjusted = Math.max(...rec.Assessments.map((a) => a.Adjusted), 1);

  return (
    <section className="rounded-2xl border border-zinc-200 dark:border-zinc-800 p-6">
      <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 mb-4">
        Supporting evidence — all tracked areas
      </h3>
      <ul className="space-y-3">
        {rec.Assessments.map((a) => (
          <li key={a.AreaID}>
            <div className="flex items-baseline justify-between text-sm mb-1">
              <span className="font-medium">
                {a.AreaName}
                {rec.Target?.AreaID === a.AreaID && (
                  <span className="ml-2 text-amber-600 dark:text-amber-400 text-xs font-semibold">
                    TARGET
                  </span>
                )}
                {!a.WeatherOK && (
                  <span className="ml-2 text-zinc-400 text-xs">weather unavailable</span>
                )}
              </span>
              <span className="text-zinc-500 tabular-nums">
                {a.AvgTrips.toFixed(1)} → {a.Adjusted.toFixed(1)} trips/hr
              </span>
            </div>
            <div className="relative h-2 rounded-full bg-zinc-100 dark:bg-zinc-800 overflow-hidden">
              <div
                className="absolute inset-y-0 left-0 rounded-full bg-zinc-400 dark:bg-zinc-600"
                style={{ width: `${(a.AvgTrips / maxAdjusted) * 100}%` }}
              />
              <div
                className={`absolute inset-y-0 left-0 rounded-full ${
                  rec.Target?.AreaID === a.AreaID ? "bg-amber-500" : "bg-zinc-500 dark:bg-zinc-400"
                } opacity-60`}
                style={{ width: `${(a.Adjusted / maxAdjusted) * 100}%` }}
              />
            </div>
          </li>
        ))}
      </ul>
      <p className="mt-4 text-xs text-zinc-400">
        Bars show baseline (dark) vs. weather-adjusted expected demand (colored overlay) per hour,
        based on {rec.Assessments[0]?.SampleDays ?? "?"} sample day(s) of history for this
        day-of-week/hour.
      </p>
    </section>
  );
}

function Caveats({ rec }: { rec: Recommendation }) {
  return (
    <section className="rounded-2xl border border-zinc-200 dark:border-zinc-800 p-6">
      <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 mb-3">
        What could make this wrong
      </h3>
      <ul className="space-y-2 text-sm text-zinc-600 dark:text-zinc-400 list-disc list-inside">
        {rec.Caveats.map((c, i) => (
          <li key={i}>{c}</li>
        ))}
      </ul>
    </section>
  );
}
