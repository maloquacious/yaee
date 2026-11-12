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
// loss. This is DefaultConfig:
//
//	{"seed":0,"radius_km":6370,"sample_spacing_km":25,"duration_my":250,"time_step_my":2,"plates":40,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-2.5,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.17,"subduction_distance_km":600,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.1,"sediment_accretion_km_per_my":0.3,"collision_penetration_km":300,"collision_distance_km":1000,"collision_coefficient_per_km":5e-7,"rifting_rate_per_my":0.004,"elevation_noise_amplitude_km":0.5,"elevation_noise_feature_size_km":2000,"rock_noise_amplitude_km":500,"rock_noise_feature_size_km":1000,"rock_reach_km":300,"rock_orogeny_max_age_my":150,"rock_arc_uplift_km_per_my":0.01}
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
	// displaced by about 0.41 times this value on average, and never by
	// more than 2.91 times it. Values above about half the distance
	// between plate centroids, which is about Radius · √(4π/Plates),
	// tear plates into pieces, and the pieces are then given to
	// neighbouring plates.
	PlateWarpAmplitude float64 `json:"plate_warp_amplitude_km"`

	// PlateWarpFeatureSize is the length of the largest bends in the
	// boundaries of the initial plates. Smaller and weaker bends, down to
	// 1/128 of this length, are added to them, so that a boundary is
	// irregular at every scale.
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
	// paper's Table 3 except where a field says otherwise. Sea level is
	// 0, and they must satisfy
	//
	//	TrenchElevation < AbyssalElevation < RidgeElevation < 0 < MaxContinentalElevation.

	// RidgeElevation is the highest elevation of oceanic crust: that of
	// a mid-ocean ridge (z_r).
	// Unit: km. Range: finite, less than 0, greater than
	// AbyssalElevation. Default: -2.5.
	//
	// The paper's Table 3 has −1. The default is the depth of the crest
	// of Earth's mid-ocean ridges; with −1 the young crust beside every
	// ridge forms a third peak in the distribution of elevation, between
	// the ocean floor and the land.
	//
	// The initial ocean floor is at this elevation at the coast.
	RidgeElevation float64 `json:"ridge_elevation_km"`

	// AbyssalElevation is the elevation of the abyssal plains (z_a):
	// the elevation that oceanic crust sinks toward as it ages, and
	// below which only a trench lies.
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
	// Default: 0.17.
	//
	// In each time step of length δt My, the elevation z of every sample
	// of oceanic crust above AbyssalElevation becomes
	//
	//	z − (1 − z/AbyssalElevation) · OceanicDampening · δt
	//
	// so oceanic crust sinks toward AbyssalElevation and never passes
	// it: after t My, crust that formed at RidgeElevation is near
	//
	//	AbyssalElevation + (RidgeElevation − AbyssalElevation) · exp(−OceanicDampening · t / |AbyssalElevation|)
	//
	// which with the defaults is −4.5 km after 30 My and −5.8 km after
	// 100 My. A trench is measured from the elevation this gives. If
	// OceanicDampening · δt is greater than |AbyssalElevation|, a step
	// puts the crust at AbyssalElevation exactly. 0 gives oceanic crust
	// that does not sink.
	//
	// The paper's Table 3 has 0.04, and its formula has TrenchElevation
	// where this one has AbyssalElevation, so that its ocean floor sinks
	// toward the depth of a trench for as long as it lives. With them
	// half of the default world's ocean floor is above −1.4 km. The
	// default gives the time constant |AbyssalElevation| /
	// OceanicDampening = 35 My; the depth of Earth's ocean floor against
	// its age fits 36 My (Stein and Stein, Nature 359, 1992).
	OceanicDampening float64 `json:"oceanic_dampening_km_per_my"`

	// SubductionDistance is how far behind a subduction front the
	// overriding plate is raised (the subduction distance r_s of the
	// paper's Table 3).
	// Unit: km. Range: finite and at least 0. Default: 600.
	//
	// Uplift is 0 at the front, greatest at SubductionDistance / 6
	// behind it, and 0 again at SubductionDistance and beyond. 0 gives
	// no uplift.
	//
	// The paper's Table 3 has 1800, with which a front raises land as
	// far inland as most continents reach, and nearly all the land of
	// the default world is a mountain range. 600 is about the width of
	// the Andes with their plateau.
	SubductionDistance float64 `json:"subduction_distance_km"`

	// SubductionUplift is the base rate at which subduction raises the
	// overriding plate (the subduction uplift u_0 of the paper's Table
	// 3).
	// Unit: km/My, which equals mm/y. Range: finite and at least 0.
	// Default: 0.6.
	//
	// The rate at a sample is SubductionUplift multiplied by three
	// factors: one of the distance from the front, 0 to 1; the relative
	// speed of the two plates divided by MaxPlateSpeed, 0 to 2; and the
	// square of (z − TrenchElevation) / (MaxContinentalElevation −
	// TrenchElevation), 0 to 1, with z the elevation of the crust that
	// is subducting. It is then multiplied by 1 − e /
	// MaxContinentalElevation, with e the sample's own elevation where
	// that is above 0, so that no sample is raised above
	// MaxContinentalElevation. 0 gives no uplift.
	SubductionUplift float64 `json:"subduction_uplift_km_per_my"`

	// ContinentalErosion is the rate at which land at
	// MaxContinentalElevation is worn down (the continental erosion ε_c
	// of the paper's Table 3).
	// Unit: km/My, which equals mm/y. Range: finite and at least 0.
	// Default: 0.1.
	//
	// In each time step of length δt My, the elevation z of every sample
	// of continental crust above sea level becomes
	//
	//	z − (z / MaxContinentalElevation) · ContinentalErosion · δt
	//
	// and not less than 0. 0 gives land that is not eroded.
	//
	// Land that nothing raises falls to 1/e of its elevation in
	// MaxContinentalElevation / ContinentalErosion My: 100 My with the
	// defaults. The paper's Table 3 has 0.03, which gives 333 My, longer
	// than the default Duration: no mountain range is worn down within a
	// run, and the land only rises. 100 My is of the order in which
	// Earth's old ranges have been worn down.
	ContinentalErosion float64 `json:"continental_erosion_km_per_my"`

	// SedimentAccretion is the rate at which an oceanic trench that is
	// no longer being formed fills with sediment (the sediment
	// accretion ε_t of the paper's Table 3).
	// Unit: km/My, which equals mm/y. Range: finite and at least 0.
	// Default: 0.3.
	//
	// 0 gives trenches that never fill.
	SedimentAccretion float64 `json:"sediment_accretion_km_per_my"`

	// CollisionPenetration is how far the continental crust of one plate
	// goes beneath the continental crust of another before the two
	// collide (the interpenetration threshold of the paper's §6).
	// Unit: km. Range: finite and at least 0. Default: 300.
	//
	// Until then the crust that goes beneath is consumed. At the
	// collision the smaller of the two continents in contact leaves its
	// plate and joins the other. 0 gives a collision in the first step in
	// which the continents converge.
	CollisionPenetration float64 `json:"collision_penetration_km"`

	// CollisionDistance is the greatest distance from a collision at
	// which land is raised (the collision distance r_c of the paper's
	// Table 3).
	// Unit: km. Range: finite and at least 0. Default: 1000.
	//
	// The distance for one collision is CollisionDistance ·
	// √((v / MaxPlateSpeed) · (a / a₀)), and at most CollisionDistance,
	// with v the speed in km/My at which the two plates meet, a the area
	// in km² of the continent that changes plates, and a₀ the area of
	// the sphere divided by Plates. 0 gives collisions that raise
	// nothing.
	//
	// The paper's Table 3 has 4200, with which the collision of two
	// continents raises both from coast to coast. 1000 is of the order
	// of the width of the Himalaya with the Tibetan plateau.
	CollisionDistance float64 `json:"collision_distance_km"`

	// CollisionCoefficient is the rise at the suture of a collision per
	// km² of the continent that changes plates (the collision
	// coefficient Δ_c of the paper's Table 3).
	// Unit: 1/km (km of rise per km² of area). Range: finite and at
	// least 0. Default: 5e-7.
	//
	// The rise at the suture is s = CollisionCoefficient · a, limited to
	// MaxContinentalElevation · (1 − exp(−s / MaxContinentalElevation)).
	// It falls to 0 at the distance that CollisionDistance gives. 0 gives
	// collisions that raise nothing.
	//
	// The paper's Table 3 has 1.3e-5, with which a continent of a million
	// km² rises 13 km at the suture before the limit, and one the size of
	// India 57 km: every collision of the default world raises its suture
	// to MaxContinentalElevation. With 5e-7 a continent the size of India
	// (4.4 million km²) rises 2 km, and only the largest reach half of
	// MaxContinentalElevation.
	CollisionCoefficient float64 `json:"collision_coefficient_per_km"`

	// RiftingRate is how often a plate breaks into two to four plates
	// that move apart (λ_0 of the paper's §4.4): the mean number of
	// times per My for a plate that is wholly continental and has the
	// area a₀, the area of the sphere divided by Plates.
	// Unit: 1/My. Range: finite and at least 0. Default: 0.004.
	//
	// A plate of area a, of which the share x is continental, rifts at
	// the rate RiftingRate · (0.25 + 0.75 · x) · a / a₀ per My, and not
	// at all if a is less than a₀ or MaxPlateSpeed is 0. 0 gives plates
	// that never rift.
	RiftingRate float64 `json:"rifting_rate_per_my"`

	// ElevationNoiseAmplitude is the greatest change that the
	// low-frequency noise makes to an elevation (the paper's §4.5: "we
	// apply low frequency coherent noise to obtain more realistic
	// variations over the planet"; it gives no value).
	// Unit: km. Range: finite and at least 0. Default: 0.5.
	//
	// After the last step of the simulation, a value of smooth noise is
	// added to the elevation of every sample, of either crust type. The
	// values lie from −ElevationNoiseAmplitude to
	// ElevationNoiseAmplitude, with a mean near 0 and a standard
	// deviation of about 0.26 times it. The result is limited to the
	// range TrenchElevation to MaxContinentalElevation. The noise lowers
	// some continental crust below sea level. 0 gives no noise, and
	// there is none when Duration is 0.
	ElevationNoiseAmplitude float64 `json:"elevation_noise_amplitude_km"`

	// ElevationNoiseFeatureSize is the distance across the largest
	// features of that noise: its values at two points this far apart
	// are unrelated. Features down to a quarter of this size are added
	// at lower strength.
	// Unit: km. Range: finite and greater than 0. Default: 2000.
	//
	// It has no effect when ElevationNoiseAmplitude is 0.
	ElevationNoiseFeatureSize float64 `json:"elevation_noise_feature_size_km"`

	// RockNoiseAmplitude is how far the noise that roughens the rock
	// type moves the edges between rock types of continental crust.
	// Unit: km. Range: finite and at least 0. Default: 500.
	//
	// The rock type is an alpha feature: its rule and defaults may
	// change in a minor version (see AttrRockType).
	//
	// After the simulation each sample gets a base rock type from its
	// crust (see AttrRockType). A continental sample then takes the rock
	// type c with the greatest score −min(d_c, RockReach) +
	// RockNoiseAmplitude · F_c, where d_c is the distance to the nearest
	// continental sample whose base rock type is c and F_c is smooth
	// noise from −1 to 1, one field for each rock type. An edge between
	// rock types moves by up to 2 · RockNoiseAmplitude either way. A
	// sample farther than RockReach from every other rock type changes
	// type only in patches, and never if 2 · RockNoiseAmplitude is at
	// most RockReach. 0 gives every sample its base rock type.
	RockNoiseAmplitude float64 `json:"rock_noise_amplitude_km"`

	// RockNoiseFeatureSize is the distance across the largest features
	// of that noise. Features down to a quarter of this size are added
	// at lower strength.
	// Unit: km. Range: finite and greater than 0. Default: 1000.
	//
	// It has no effect when RockNoiseAmplitude is 0.
	RockNoiseFeatureSize float64 `json:"rock_noise_feature_size_km"`

	// RockReach is the distance beyond which a rock type's nearness no
	// longer favours it, in the score that RockNoiseAmplitude describes.
	// Unit: km. Range: finite and greater than 0. Default: 300.
	//
	// It has no effect when RockNoiseAmplitude is 0.
	RockReach float64 `json:"rock_reach_km"`

	// RockOrogenyMaxAge is the greatest orogeny age at which an orogeny
	// still sets the rock type. Older belts are taken to be worn down
	// and covered by sediment.
	// Unit: My. Range: finite and at least 0. Default: 150.
	//
	// Continental crust whose orogeny age is greater than this has the
	// base rock type of crust with no orogeny, RockSedimentary, unless
	// RockArcUplift makes it RockIgneous. See AttrRockType.
	RockOrogenyMaxAge float64 `json:"rock_orogeny_max_age_my"`

	// RockArcUplift is the uplift rate above which continental crust is
	// an active volcanic arc, of base rock type RockIgneous, whatever its
	// orogeny.
	// Unit: km/My. Range: finite and at least 0. Default: 0.01.
	//
	// It is compared with the AttrUplift attribute: the rate at which
	// subduction raised the sample in the last step. Values at or above
	// 2 · SubductionUplift turn the rule off.
	RockArcUplift float64 `json:"rock_arc_uplift_km_per_my"`
}

// DefaultConfig returns the configuration of an Earth-like world at
// about 25 km per sample: Seed 0, Radius 6370 km, SampleSpacing 25 km,
// Duration 250 My, TimeStep 2 My, Plates 40, PlateWarpAmplitude 1500 km,
// PlateWarpFeatureSize 2500 km, LandCoverage 0.3, RidgeElevation −2.5 km,
// AbyssalElevation −6 km, TrenchElevation −10 km,
// MaxContinentalElevation 10 km, MaxPlateSpeed 100 km/My,
// OceanicDampening 0.17 km/My, SubductionDistance 600 km,
// SubductionUplift 0.6 km/My, ContinentalErosion 0.1 km/My,
// SedimentAccretion 0.3 km/My, CollisionPenetration 300 km,
// CollisionDistance 1000 km, CollisionCoefficient 5e-7 per km,
// RiftingRate 0.004 per My, ElevationNoiseAmplitude 0.5 km,
// ElevationNoiseFeatureSize 2000 km, RockNoiseAmplitude 500 km,
// RockNoiseFeatureSize 1000 km, RockReach 300 km, RockOrogenyMaxAge
// 150 My, and RockArcUplift 0.01 km/My. It passes Validate.
//
// Six of these differ from the paper's Table 3, which has RidgeElevation
// −1, OceanicDampening 0.04, SubductionDistance 1800, ContinentalErosion
// 0.03, CollisionDistance 4200, and CollisionCoefficient 1.3e-5. Each
// field's documentation says why.
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
		RidgeElevation:          -2.5,
		AbyssalElevation:        -6,
		TrenchElevation:         -10,
		MaxContinentalElevation: 10,

		MaxPlateSpeed: 100,

		OceanicDampening: 0.17,

		SubductionDistance: 600,
		SubductionUplift:   0.6,
		ContinentalErosion: 0.1,
		SedimentAccretion:  0.3,

		CollisionPenetration: 300,
		CollisionDistance:    1000,
		CollisionCoefficient: 5e-7,

		RiftingRate: 0.004,

		ElevationNoiseAmplitude:   0.5,
		ElevationNoiseFeatureSize: 2000,

		RockNoiseAmplitude:   500,
		RockNoiseFeatureSize: 1000,
		RockReach:            300,
		RockOrogenyMaxAge:    150,
		RockArcUplift:        0.01,
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
	for _, f := range []struct {
		name  string
		value float64
	}{
		{"SubductionDistance", c.SubductionDistance},
		{"SubductionUplift", c.SubductionUplift},
		{"ContinentalErosion", c.ContinentalErosion},
		{"SedimentAccretion", c.SedimentAccretion},
		{"CollisionPenetration", c.CollisionPenetration},
		{"CollisionDistance", c.CollisionDistance},
		{"CollisionCoefficient", c.CollisionCoefficient},
		{"RiftingRate", c.RiftingRate},
		{"ElevationNoiseAmplitude", c.ElevationNoiseAmplitude},
		{"RockNoiseAmplitude", c.RockNoiseAmplitude},
		{"RockOrogenyMaxAge", c.RockOrogenyMaxAge},
		{"RockArcUplift", c.RockArcUplift},
	} {
		if !(f.value >= 0) || math.IsInf(f.value, 0) {
			invalid(f.name, "must be finite and at least 0, got %v", f.value)
		}
	}
	if !positive(c.ElevationNoiseFeatureSize) {
		invalid("ElevationNoiseFeatureSize", "must be finite and greater than 0, got %v", c.ElevationNoiseFeatureSize)
	}
	if !positive(c.RockNoiseFeatureSize) {
		invalid("RockNoiseFeatureSize", "must be finite and greater than 0, got %v", c.RockNoiseFeatureSize)
	}
	if !positive(c.RockReach) {
		invalid("RockReach", "must be finite and greater than 0, got %v", c.RockReach)
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
