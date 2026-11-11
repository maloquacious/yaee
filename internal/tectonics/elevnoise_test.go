// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
)

// noiseWorld returns a world of n samples whose elevation is z
// everywhere, with the elevation noise of the given amplitude and
// feature size, in km.
func noiseWorld(t testing.TB, n int, z, amplitude, featureSize float64) *model.World {
	t.Helper()
	params := earth()
	params.ElevationNoiseAmplitude, params.ElevationNoiseFeatureSize = amplitude, featureSize
	w := newWorld(t, n, params)
	w.Elevation = slices.Repeat([]float64{z}, n)
	return w
}

func TestElevationNoise(t *testing.T) {
	const n, amplitude = 4000, 0.5
	w := noiseWorld(t, n, 1, amplitude, 2000)
	run(t, w, 7, ElevationNoise{})
	var sum, sumSq, greatest float64
	for i, z := range w.Elevation {
		d := z - 1
		if math.Abs(d) > amplitude {
			t.Fatalf("sample %d moved %v km, more than the amplitude %v km", i, d, amplitude)
		}
		sum += d
		sumSq += d * d
		greatest = max(greatest, math.Abs(d))
	}
	mean := sum / n
	dev := math.Sqrt(sumSq/n - mean*mean)
	// The documented standard deviation is about 0.26 of the amplitude.
	if math.Abs(mean) > 0.1*amplitude || dev < 0.15*amplitude || dev > 0.4*amplitude {
		t.Errorf("the noise has mean %v km and standard deviation %v km, want about 0 and %v", mean, dev, 0.26*amplitude)
	}
	if greatest < 0.5*amplitude {
		t.Errorf("the greatest change is %v km, want more than half the amplitude %v km", greatest, amplitude)
	}

	// It is low-frequency: neighbours differ by far less than the
	// amplitude. The samples are about 357 km apart, the features 2000.
	var steps float64
	edges := 0
	for i := range w.Elevation {
		for _, j := range w.Mesh.Neighbours(i) {
			steps += math.Abs(w.Elevation[i] - w.Elevation[j])
			edges++
		}
	}
	if mean := steps / float64(edges); mean > 0.3*amplitude {
		t.Errorf("neighbours differ by %v km on average, want less than %v", mean, 0.3*amplitude)
	}

	// The same seed gives the same noise, another seed another.
	again := noiseWorld(t, n, 1, amplitude, 2000)
	run(t, again, 7, ElevationNoise{})
	if !slices.Equal(again.Elevation, w.Elevation) {
		t.Error("the same seed gave different noise")
	}
	other := noiseWorld(t, n, 1, amplitude, 2000)
	run(t, other, 8, ElevationNoise{})
	if slices.Equal(other.Elevation, w.Elevation) {
		t.Error("another seed gave the same noise")
	}
}

// The result stays within the trench elevation and the greatest
// continental elevation.
func TestElevationNoiseLimited(t *testing.T) {
	for _, z := range []float64{-10, 10} {
		w := noiseWorld(t, 1000, z, 3, 2000)
		run(t, w, 1, ElevationNoise{})
		changed := false
		for i, got := range w.Elevation {
			if got < w.TrenchElevation || got > w.MaxContinentalElevation {
				t.Fatalf("sample %d at %v km became %v km, outside %v to %v", i, z, got, w.TrenchElevation, w.MaxContinentalElevation)
			}
			changed = changed || got != z
		}
		if !changed {
			t.Errorf("no sample at %v km changed", z)
		}
	}
}

// No amplitude or no duration gives no noise.
func TestElevationNoiseOff(t *testing.T) {
	w := noiseWorld(t, 1000, 1, 0, 2000)
	run(t, w, 1, ElevationNoise{})
	if slices.ContainsFunc(w.Elevation, func(z float64) bool { return z != 1 }) {
		t.Error("an amplitude of 0 changed the elevation")
	}
	w = noiseWorld(t, 1000, 1, 0.5, 2000)
	w.Duration = 0
	run(t, w, 1, ElevationNoise{})
	if slices.ContainsFunc(w.Elevation, func(z float64) bool { return z != 1 }) {
		t.Error("a Duration of 0 changed the elevation")
	}
}

func TestElevationNoiseCancelled(t *testing.T) {
	w := noiseWorld(t, 1000, 1, 0.5, 2000)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (ElevationNoise{}).Run(ctx, w, model.Stream(1, "elevation-noise")); err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
