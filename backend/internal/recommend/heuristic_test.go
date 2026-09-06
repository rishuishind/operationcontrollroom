package recommend

import "testing"

func TestWeatherMultiplier(t *testing.T) {
	cases := []struct {
		name       string
		precipMM   float64
		precipProb float64
		want       float64
	}{
		{"dry, no rain expected", 0, 0, 1.0},
		{"heavy downpour, certain", 12, 100, 1.6},  // capped
		{"light rain, 50% chance", 1, 50, 1.28},
		{"negative inputs clamp to zero", -5, -10, 1.0},
		{"probability over 100 clamps to 100", 0, 250, 1.5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := WeatherMultiplier(c.precipMM, c.precipProb)
			if got != c.want {
				t.Errorf("WeatherMultiplier(%v, %v) = %v, want %v", c.precipMM, c.precipProb, got, c.want)
			}
		})
	}
}
