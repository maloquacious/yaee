// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import (
	"errors"
	"fmt"
	"math"

	"github.com/maloquacious/yaee/internal/cerrs"
	"github.com/maloquacious/yaee/internal/sphere"
)

// ErrInvalidConfig is wrapped by every error that Config.Validate
// returns. Test for it with errors.Is.
const ErrInvalidConfig = cerrs.Error("yaee: invalid config")

// Limits on the number of samples a Config may ask for. See
// Config.SampleSpacing.
const (
	MinSamples = 4
	MaxSamples = sphere.MaxPoints // 357,913,941
)

// MaxPlates is the largest value of Config.Plates.
const MaxPlates = 1000

// Config is everything that determines a world. The same Config always
// generates the same world, and the datafile records the Config that
// produced it.
//
// The zero Config is not valid. Start from DefaultConfig and change
// fields. No function replaces a zero field with a default.
//
// Config is plain data. It encodes to JSON as one object whose members
// are named by the json tags below, and decodes from that object without
// loss:
//
//	{"seed":0,"radius_km":6370,"sample_spacing_km":25,"duration_my":250,"time_step_my":2,"plates":40,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500}
//
// Lengths are in kilometres (km) and times in millions of years (My).
type Config struct {
	// Seed selects the world: all randomness is derived from it.
	// Unit: none. Range: any uint64, 0 to 18446744073709551615.
	// Default: 0.
	//
	// In JSON it is an integer that can exceed 2^53, so a decoder that
	// reads numbers as IEEE 754 doubles may corrupt it.
	Seed uint64 `json:"seed"`

	// Radius is the radius of the sphere at sea level.
	// Unit: km. Range: finite and greater than 0. Default: 6370.
	Radius float64 `json:"radius_km"`

	// SampleSpacing is the distance between neighbouring samples.
	// Unit: km. Range: finite and greater than 0, and such that the
	// sample count N is from MinSamples to MaxSamples. Default: 25.
	//
	// The world has exactly
	//
	//	N = round(8π · Radius² / (√3 · SampleSpacing²))
	//
	// samples, rounding half away from zero. The default Radius and
	// SampleSpacing give N = 942,059.
	SampleSpacing float64 `json:"sample_spacing_km"`

	// Duration is the span of time simulated.
	// Unit: My. Range: finite and at least 0. Default: 250.
	//
	// As of this version it is validated and recorded but has no effect
	// on the world.
	Duration float64 `json:"duration_my"`

	// TimeStep is the length of one simulation step.
	// Unit: My. Range: finite and greater than 0. Default: 2.
	//
	// As of this version it is validated and recorded but has no effect
	// on the world.
	TimeStep float64 `json:"time_step_my"`

	// Plates is the number of tectonic plates at the start.
	// Unit: none. Range: 1 to MaxPlates (1000), and no more than the
	// sample count N. Default: 40.
	//
	// Plates are numbered from 0 to Plates−1. Each is one connected
	// region of samples, and together they cover the sphere.
	Plates int `json:"plates"`

	// PlateWarpAmplitude sets how far the boundaries of the initial
	// plates depart from the straight boundaries of Voronoi cells.
	// Unit: km. Range: finite and at least 0. Default: 1500.
	//
	// 0 gives plates that are exactly the Voronoi cells of their
	// centroids: convex polygons with great-circle sides. A boundary is
	// displaced by about 0.34 times this value on average, and never by
	// more than 1.74 times it. Values above about half the distance
	// between plate centroids, which is about Radius · √(4π/Plates),
	// tear plates into pieces, and the pieces are then given to
	// neighbouring plates.
	PlateWarpAmplitude float64 `json:"plate_warp_amplitude_km"`

	// PlateWarpFeatureSize is the length of the largest bends in the
	// boundaries of the initial plates. Smaller bends, down to an
	// eighth of this length, are added to them.
	// Unit: km. Range: finite and greater than 0. Default: 2500.
	//
	// It has no effect when PlateWarpAmplitude is 0.
	PlateWarpFeatureSize float64 `json:"plate_warp_feature_size_km"`
}

// DefaultConfig returns the configuration of an Earth-like world at
// about 25 km per sample: Seed 0, Radius 6370 km, SampleSpacing 25 km,
// Duration 250 My, TimeStep 2 My, Plates 40, PlateWarpAmplitude 1500 km,
// PlateWarpFeatureSize 2500 km. It passes Validate.
func DefaultConfig() Config {
	return Config{
		Seed:                 0,
		Radius:               6370,
		SampleSpacing:        25,
		Duration:             250,
		TimeStep:             2,
		Plates:               40,
		PlateWarpAmplitude:   1500,
		PlateWarpFeatureSize: 2500,
	}
}

// Validate reports whether every field is in its range, as stated in
// the field's documentation. It returns nil if so. Otherwise it returns
// an error that wraps ErrInvalidConfig and whose text has one line for
// each invalid field, in the form
//
//	yaee: invalid config: Radius: must be finite and greater than 0, got -1
//
// where the name after the second colon is the Go name of the field.
func (c Config) Validate() error {
	var errs []error
	invalid := func(field, format string, args ...any) {
		errs = append(errs, fmt.Errorf("%w: %s: %s", ErrInvalidConfig, field, fmt.Sprintf(format, args...)))
	}
	positive := func(v float64) bool { return v > 0 && !math.IsInf(v, 0) }

	// Seed has no invalid value.
	samples := math.NaN() // the sample count, if Radius and SampleSpacing allow one
	if !positive(c.Radius) {
		invalid("Radius", "must be finite and greater than 0, got %v", c.Radius)
	}
	if !positive(c.SampleSpacing) {
		invalid("SampleSpacing", "must be finite and greater than 0, got %v", c.SampleSpacing)
	} else if positive(c.Radius) {
		// Compare as floating point first: the count can exceed an int.
		n := sampleCountFloat(c.Radius, c.SampleSpacing)
		if !(n >= MinSamples && n <= MaxSamples) {
			invalid("SampleSpacing", "with Radius %v it gives %.0f samples; the count must be from %d to %d, got spacing %v",
				c.Radius, n, MinSamples, MaxSamples, c.SampleSpacing)
		} else {
			samples = n
		}
	}
	if !(c.Duration >= 0) || math.IsInf(c.Duration, 0) {
		invalid("Duration", "must be finite and at least 0, got %v", c.Duration)
	}
	if !positive(c.TimeStep) {
		invalid("TimeStep", "must be finite and greater than 0, got %v", c.TimeStep)
	}
	if c.Plates < 1 || c.Plates > MaxPlates {
		invalid("Plates", "must be from 1 to %d, got %d", MaxPlates, c.Plates)
	} else if float64(c.Plates) > samples {
		invalid("Plates", "must be no more than the sample count, %.0f, got %d", samples, c.Plates)
	}
	if !(c.PlateWarpAmplitude >= 0) || math.IsInf(c.PlateWarpAmplitude, 0) {
		invalid("PlateWarpAmplitude", "must be finite and at least 0, got %v", c.PlateWarpAmplitude)
	}
	if !positive(c.PlateWarpFeatureSize) {
		invalid("PlateWarpFeatureSize", "must be finite and greater than 0, got %v", c.PlateWarpFeatureSize)
	}
	return errors.Join(errs...)
}

// sampleCountFloat is the sample count before conversion to an integer.
func sampleCountFloat(radius, spacing float64) float64 {
	return math.Round(8 * math.Pi * radius * radius / (math.Sqrt(3) * spacing * spacing))
}

// sampleCount returns the number of samples of a valid Config.
func (c Config) sampleCount() int {
	return sphere.SampleCount(c.Radius, c.SampleSpacing)
}
