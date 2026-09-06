package recommend

// WeatherMultiplier converts current precipitation into a demand
// multiplier applied to the historical baseline.
//
// Assumption (documented in README "Assumptions & Limitations"): rain is a
// well-known driver of increased rideshare demand (people avoid walking/
// transit), but we do not have historical trip data tagged with the
// weather at the time, so we cannot calibrate this multiplier from our own
// dataset. Instead we use a simple, generic curve: demand scales with
// precipitation probability, with a smaller additional bump from
// precipitation intensity, capped so a single bad-weather reading can't
// dominate the recommendation.
func WeatherMultiplier(precipMM, precipProbabilityPct float64) float64 {
	if precipProbabilityPct < 0 {
		precipProbabilityPct = 0
	}
	if precipProbabilityPct > 100 {
		precipProbabilityPct = 100
	}
	if precipMM < 0 {
		precipMM = 0
	}
	intensity := precipMM
	if intensity > 10 {
		intensity = 10 // cap: beyond ~10mm/hr additional intensity doesn't add more demand signal
	}

	multiplier := 1.0 + 0.5*(precipProbabilityPct/100) + intensity*0.03

	const cap = 1.6
	if multiplier > cap {
		multiplier = cap
	}
	return multiplier
}
