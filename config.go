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

// MaxSteps is the largest number of time steps a Config may ask for.
// See Config.Duration.
const MaxSteps = 1_000_000

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
//	{"seed":0,"radius_km":6370,"sample_spacing_km":25,"duration_my":250,"time_step_my":2,"plates":40,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.04}
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
	// Unit: My. Range: finite, at least 0, and at most MaxSteps
	// (1,000,000) times TimeStep. Default: 250.
	//
	// 0 gives the initial world: plates and crust as they start, before
	// any plate has moved.
	Duration float64 `json:"duration_my"`

	// TimeStep is the length of one simulation step.
	// Unit: My. Range: finite and greater than 0. Default: 2.
	//
	// The simulation takes the smallest whole number of steps that
	// covers Duration. If Duration is not a multiple of TimeStep, the
	// last step is the shorter remainder.
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

	// LandCoverage is the share of the sphere's area that is
	// continental crust at the start.
	// Unit: none; a fraction. Range: 0 to 1. Default: 0.3.
	//
	// The share achieved differs from this value by at most 1.5/N for
	// N samples. All continental crust starts above sea level and all
	// oceanic crust below it.
	LandCoverage float64 `json:"land_coverage"`

	// The four elevations below are constants of the model, from the
	// paper's Table 3. Sea level is 0, and they must satisfy
	//
	//	TrenchElevation < AbyssalElevation < RidgeElevation < 0 < MaxContinentalElevation.

	// RidgeElevation is the highest elevation of oceanic crust: that of
	// a mid-ocean ridge (z_r).
	// Unit: km. Range: finite, less than 0, greater than
	// AbyssalElevation. Default: -1.
	//
	// The initial ocean floor is at this elevation at the coast.
	RidgeElevation float64 `json:"ridge_elevation_km"`

	// AbyssalElevation is the elevation of the abyssal plains (z_a).
	// Unit: km. Range: finite, less than RidgeElevation, greater than
	// TrenchElevation. Default: -6.
	//
	// The initial ocean floor is at this elevation 500 km or more from
	// the coast.
	AbyssalElevation float64 `json:"abyssal_elevation_km"`

	// TrenchElevation is the elevation of the deepest oceanic trench
	// (z_t). No elevation is below it.
	// Unit: km. Range: finite, less than AbyssalElevation. Default: -10.
	TrenchElevation float64 `json:"trench_elevation_km"`

	// MaxContinentalElevation is the highest elevation of continental
	// crust (z_c). No elevation is above it.
	// Unit: km. Range: finite and greater than 0. Default: 10.
	//
	// The initial land is at 0.01 of this elevation at the coast and
	// 0.05 of it 500 km or more inland.
	MaxContinentalElevation float64 `json:"max_continental_elevation_km"`

	// MaxPlateSpeed is the greatest speed at which any point of a plate
	// moves over the surface (v0 in the paper's Table 3).
	// Unit: km/My, which equals mm/y. Range: finite and at least 0.
	// Default: 100.
	//
	// Each plate turns about its own axis through the centre of the
	// sphere. The speed of a plate's fastest point is drawn uniformly
	// from 0 to this value. 0 gives plates that do not move.
	MaxPlateSpeed float64 `json:"max_plate_speed_km_per_my"`

	// OceanicDampening is the rate at which oceanic crust at sea level
	// sinks as it ages (the oceanic elevation damping ε_o of the paper's
	// Table 3).
	// Unit: km/My, which equals mm/y. Range: finite and at least 0.
	// Default: 0.04.
	//
	// In each time step of length δt My, the elevation z of every sample
	// of oceanic crust becomes
	//
	//	z − (1 − z/TrenchElevation) · OceanicDampening · δt
	//
	// so oceanic crust sinks toward TrenchElevation and never passes
	// it: after t My, crust that formed at RidgeElevation is near
	//
	//	TrenchElevation + (RidgeElevation − TrenchElevation) · exp(−OceanicDampening · t / |TrenchElevation|)
	//
	// which with the defaults is −6 km after 203 My. If
	// OceanicDampening · δt is greater than |TrenchElevation|, a step
	// puts the crust at TrenchElevation exactly. 0 gives oceanic crust
	// that does not sink.
	OceanicDampening float64 `json:"oceanic_dampening_km_per_my"`
}

// DefaultConfig returns the configuration of an Earth-like world at
// about 25 km per sample: Seed 0, Radius 6370 km, SampleSpacing 25 km,
// Duration 250 My, TimeStep 2 My, Plates 40, PlateWarpAmplitude 1500 km,
// PlateWarpFeatureSize 2500 km, LandCoverage 0.3, RidgeElevation −1 km,
// AbyssalElevation −6 km, TrenchElevation −10 km,
// MaxContinentalElevation 10 km, MaxPlateSpeed 100 km/My, and
// OceanicDampening 0.04 km/My. It passes Validate.
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

		LandCoverage:            0.3,
		RidgeElevation:          -1,
		AbyssalElevation:        -6,
		TrenchElevation:         -10,
		MaxContinentalElevation: 10,

		MaxPlateSpeed: 100,

		OceanicDampening: 0.04,
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
	} else if c.Duration > MaxSteps*c.TimeStep && !math.IsInf(c.Duration, 0) {
		invalid("Duration", "with TimeStep %v it takes more than %d steps, got %v", c.TimeStep, MaxSteps, c.Duration)
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
	if !(c.LandCoverage >= 0 && c.LandCoverage <= 1) {
		invalid("LandCoverage", "must be from 0 to 1, got %v", c.LandCoverage)
	}
	// Each elevation is checked against the one above it, and only if
	// that one is a number, so one wrong value is reported once.
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	below := func(v, limit float64) bool { return v < limit || math.IsNaN(limit) }
	if !finite(c.RidgeElevation) || !(c.RidgeElevation < 0) {
		invalid("RidgeElevation", "must be finite and less than 0, got %v", c.RidgeElevation)
	}
	if !finite(c.AbyssalElevation) || !below(c.AbyssalElevation, c.RidgeElevation) {
		invalid("AbyssalElevation", "must be finite and less than RidgeElevation (%v), got %v", c.RidgeElevation, c.AbyssalElevation)
	}
	if !finite(c.TrenchElevation) || !below(c.TrenchElevation, c.AbyssalElevation) {
		invalid("TrenchElevation", "must be finite and less than AbyssalElevation (%v), got %v", c.AbyssalElevation, c.TrenchElevation)
	}
	if !positive(c.MaxContinentalElevation) {
		invalid("MaxContinentalElevation", "must be finite and greater than 0, got %v", c.MaxContinentalElevation)
	}
	if !(c.MaxPlateSpeed >= 0) || math.IsInf(c.MaxPlateSpeed, 0) {
		invalid("MaxPlateSpeed", "must be finite and at least 0, got %v", c.MaxPlateSpeed)
	}
	if !(c.OceanicDampening >= 0) || math.IsInf(c.OceanicDampening, 0) {
		invalid("OceanicDampening", "must be finite and at least 0, got %v", c.OceanicDampening)
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
