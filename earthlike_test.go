// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee_test

import (
	"fmt"
	"math"
	"testing"

	"github.com/maloquacious/yaee"
	"github.com/maloquacious/yaee/internal/stats"
)

// The default configuration gives an Earth-like world: the stated share
// of land, an ocean floor and continents that are two separate peaks in
// the distribution of elevation, and elevations within the model's
// limits. It runs the default configuration at 100 km per sample,
// 58,879 samples, for five seeds, and takes about 9 seconds on ten processors.
//
// The ranges are those documented in PLAN.md, milestone 9, widened for
// the coarser grid. They are met at the default 25 km per sample by
// seeds 1 to 5.
func TestDefaultIsEarthLike(t *testing.T) {
	if testing.Short() {
		t.Skip("five simulations of 250 My")
	}
	for seed := uint64(1); seed <= 5; seed++ {
		t.Run(fmt.Sprint("seed ", seed), func(t *testing.T) {
			t.Parallel()
			cfg := yaee.DefaultConfig()
			cfg.Seed, cfg.SampleSpacing = seed, 100
			s, err := stats.Measure(mustGenerate(t, cfg))
			if err != nil {
				t.Fatal(err)
			}
			within := func(name string, got, lo, hi float64) {
				t.Helper()
				if !(got >= lo && got <= hi) {
					t.Errorf("%s = %v, want %v to %v", name, got, lo, hi)
				}
			}
			within("land_fraction", s.LandFraction, 0.18, 0.32)
			within("continental_fraction", s.ContinentalFraction, 0.2, 0.32)
			within("oceanic_land_fraction", s.OceanicLandFraction, 0, 0.01)
			within("elevation_km.min", s.Elevation.Min, cfg.TrenchElevation, cfg.AbyssalElevation)
			within("elevation_km.max", s.Elevation.Max, 2, cfg.MaxContinentalElevation)
			within("continental_elevation_km.p50", s.ContinentalElevation.P50, 0.2, 1.2)
			within("continental_elevation_km.p90", s.ContinentalElevation.P90, 1, 3)
			within("oceanic_elevation_km.p50", s.OceanicElevation.P50, -5.5, -3.5)
			within("oceanic_elevation_km.p90", s.OceanicElevation.P90, -4, -2)
			within("plates.count", float64(s.Plates.Count), 20, 100)

			// Two peaks: the fullest 0.5 km bin below −2 km, the fullest
			// at or above −0.5 km, and between them no bin with a tenth
			// of the lesser peak.
			var ocean, land, between float64
			for i, f := range s.Histogram.Bins {
				switch lo := s.Histogram.First + float64(i)*s.Histogram.BinWidth; {
				case lo < -2:
					ocean = max(ocean, f)
				case lo >= -0.5:
					land = max(land, f)
				default:
					between = max(between, f)
				}
			}
			if ocean < 0.05 || land < 0.03 || between > 0.1*math.Min(ocean, land) {
				t.Errorf("the elevation histogram has an ocean peak of %v, a land peak of %v, and %v between them: not two peaks\n%v from %v km",
					ocean, land, between, s.Histogram.Bins, s.Histogram.First)
			}
		})
	}
}
