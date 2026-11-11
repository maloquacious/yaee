// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// landShare returns the continental share of the sphere's area.
func landShare(w *model.World) float64 {
	var land, all float64
	for i, a := range w.Mesh.PointAreas() {
		all += a
		if w.CrustType[i] == model.Continental {
			land += a
		}
	}
	return land / all
}

// checkCrust fails the test unless every crust parameter of every
// sample is finite, in its range, and neutral where it does not apply.
func checkCrust(t testing.TB, w *model.World) {
	t.Helper()
	finite := func(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
	direction := func(name string, i int, v sphere.Vec, unit bool) {
		t.Helper()
		if !finite(v.X) || !finite(v.Y) || !finite(v.Z) {
			t.Fatalf("sample %d: %s %v is not finite", i, name, v)
		}
		if v == (sphere.Vec{}) {
			return
		}
		if dot := v.Dot(w.Mesh.Points[i]); math.Abs(dot) > 1e-9*v.Len() {
			t.Fatalf("sample %d: %s %v is not tangent to the sphere", i, name, v)
		}
		if unit && math.Abs(v.Len()-1) > 1e-9 {
			t.Fatalf("sample %d: %s %v does not have length 1", i, name, v)
		}
	}
	for i := range w.Mesh.Points {
		z := w.Elevation[i]
		if !finite(z) || z < w.TrenchElevation || z > w.MaxContinentalElevation {
			t.Fatalf("sample %d: elevation %v is outside %v to %v", i, z, w.TrenchElevation, w.MaxContinentalElevation)
		}
		if e := w.Thickness[i]; !finite(e) || e <= 0 {
			t.Fatalf("sample %d: thickness %v", i, e)
		}
		if a := w.OceanicAge[i]; !finite(a) || a < 0 {
			t.Fatalf("sample %d: oceanic age %v", i, a)
		}
		if a := w.OrogenyAge[i]; !finite(a) || a < 0 {
			t.Fatalf("sample %d: orogeny age %v", i, a)
		}
		direction("ridge direction", i, w.RidgeDirection[i], true)
		direction("fold direction", i, w.FoldDirection[i], false)

		switch w.CrustType[i] {
		case model.Oceanic:
		case model.Continental:
			// Oceanic parameters are neutral on continental crust.
			if w.OceanicAge[i] != 0 || w.RidgeDirection[i] != (sphere.Vec{}) {
				t.Fatalf("sample %d is continental with oceanic age %v and ridge direction %v", i, w.OceanicAge[i], w.RidgeDirection[i])
			}
		default:
			t.Fatalf("sample %d: crust type %d", i, w.CrustType[i])
		}
		switch w.OrogenyType[i] {
		case model.OrogenyNone:
			if w.OrogenyAge[i] != 0 {
				t.Fatalf("sample %d has no orogeny but orogeny age %v", i, w.OrogenyAge[i])
			}
		case model.OrogenyAndean, model.OrogenyHimalayan:
		default:
			t.Fatalf("sample %d: orogeny type %d", i, w.OrogenyType[i])
		}
	}
}

func TestCrustLandCoverage(t *testing.T) {
	const n = 5000
	for _, coverage := range []float64{0, 0.001, 0.05, 0.3, 0.5, 0.7, 0.95, 1} {
		for seed := range uint64(4) {
			params := earth()
			params.LandCoverage = coverage
			w := newWorld(t, n, params)
			run(t, w, seed, Plates{}, Crust{})
			checkCrust(t, w)
			if got := landShare(w); math.Abs(got-coverage) > 1.5/n {
				t.Errorf("coverage %v, seed %d: continental share is %v, off by %v; want within %v",
					coverage, seed, got, got-coverage, 1.5/n)
			}
		}
	}
}

func TestCrustInitialValues(t *testing.T) {
	const n = 8000
	params := earth()
	w := newWorld(t, n, params)
	run(t, w, 4, Plates{}, Crust{})
	checkCrust(t, w)

	age := InitialOceanicAge(params)
	spacing := sphere.SampleSpacing(params.Radius, n)
	var coastal, interior [2]int
	for i, p := range w.Mesh.Points {
		kind := w.CrustType[i]
		// The sample's distance from the other crust type, by brute
		// force: to the nearest sample of the other type.
		nearestOther := math.Inf(1)
		for j, q := range w.Mesh.Points {
			if w.CrustType[j] != kind {
				nearestOther = min(nearestOther, sphere.Angle(p, q)*params.Radius)
			}
		}
		onCoast := false
		for _, j := range w.Mesh.Neighbours(i) {
			onCoast = onCoast || w.CrustType[j] != kind
		}

		z := w.Elevation[i]
		var atCoast, inInterior float64
		if kind == model.Continental {
			atCoast, inInterior = 0.1, 0.5
			if w.Thickness[i] != 35 {
				t.Fatalf("sample %d: continental thickness %v, want 35", i, w.Thickness[i])
			}
			if z <= 0 {
				t.Fatalf("sample %d: continental crust at elevation %v, not above sea level", i, z)
			}
		} else {
			atCoast, inInterior = -1, -6
			if w.Thickness[i] != 7 {
				t.Fatalf("sample %d: oceanic thickness %v, want 7", i, w.Thickness[i])
			}
			if w.OceanicAge[i] != age {
				t.Fatalf("sample %d: oceanic age %v, want %v", i, w.OceanicAge[i], age)
			}
			if z >= 0 {
				t.Fatalf("sample %d: oceanic crust at elevation %v, not below sea level", i, z)
			}
		}
		if lo, hi := min(atCoast, inInterior), max(atCoast, inInterior); z < lo-1e-12 || z > hi+1e-12 {
			t.Fatalf("sample %d: elevation %v is outside %v to %v", i, z, lo, hi)
		}
		switch {
		case onCoast:
			coastal[kind]++
			if math.Abs(z-atCoast) > 1e-12 {
				t.Fatalf("sample %d is on the coast at elevation %v, want %v", i, z, atCoast)
			}
		case nearestOther > 500+2*spacing:
			// The coast is within a sample spacing of the nearest
			// sample of the other type.
			interior[kind]++
			if math.Abs(z-inInterior) > 1e-12 {
				t.Fatalf("sample %d is %v km from the other crust type at elevation %v, want %v", i, nearestOther, z, inInterior)
			}
		default:
			// On the margin, elevation follows the distance from the
			// coast, which is within a spacing of nearestOther.
			d := nearestOther - spacing
			lo := atCoast + (inInterior-atCoast)*smoothstep((d-spacing)/marginWidth)
			hi := atCoast + (inInterior-atCoast)*smoothstep((d+spacing)/marginWidth)
			if lo > hi {
				lo, hi = hi, lo
			}
			if z < lo-1e-9 || z > hi+1e-9 {
				t.Fatalf("sample %d is %v km from the other crust type at elevation %v, want %v to %v", i, nearestOther, z, lo, hi)
			}
		}
		if w.OrogenyType[i] != model.OrogenyNone || w.FoldDirection[i] != (sphere.Vec{}) || w.RidgeDirection[i] != (sphere.Vec{}) {
			t.Fatalf("sample %d starts with an orogeny, a fold, or a ridge direction", i)
		}
	}
	for kind, name := range []string{"oceanic", "continental"} {
		if coastal[kind] < 50 || interior[kind] < 200 {
			t.Errorf("%s: %d coastal and %d interior samples; the test needs both", name, coastal[kind], interior[kind])
		}
	}
}

// The initial age is the time the paper's dampening takes from the
// ridge elevation to the abyssal elevation: integrating the dampening
// for that long must arrive at z_a.
func TestInitialOceanicAge(t *testing.T) {
	params := earth()
	age := InitialOceanicAge(params)
	if math.Abs(age-202.7) > 0.05 {
		t.Errorf("InitialOceanicAge = %v, want 202.7", age)
	}
	for _, set := range []func(*model.Params){
		func(*model.Params) {},
		func(p *model.Params) { p.RidgeElevation, p.AbyssalElevation, p.TrenchElevation = -0.5, -3, -4 },
		func(p *model.Params) { p.RidgeElevation, p.AbyssalElevation, p.TrenchElevation = -2.5, -2.6, -11 },
	} {
		params := earth()
		set(&params)
		age := InitialOceanicAge(params)
		if !(age > 0) || math.IsInf(age, 0) {
			t.Fatalf("InitialOceanicAge(%+v) = %v", params, age)
		}
		// dz/dt = −(1 − z/z_t) · ε_o (§4.5), by small Euler steps.
		z := params.RidgeElevation
		const steps = 200000
		dt := age / steps
		for range steps {
			z -= (1 - z/params.TrenchElevation) * params.OceanicDampening * dt
		}
		if math.Abs(z-params.AbyssalElevation) > 1e-4 {
			t.Errorf("dampening for %v My takes z_r = %v to %v, want z_a = %v", age, params.RidgeElevation, z, params.AbyssalElevation)
		}
	}

	// Twice the dampening, half the time; none, no age.
	params.OceanicDampening = 0.08
	if got := InitialOceanicAge(params); math.Abs(got-age/2) > 1e-9 {
		t.Errorf("with twice the dampening InitialOceanicAge = %v, want %v", got, age/2)
	}
	params.OceanicDampening = 0
	if got := InitialOceanicAge(params); got != 0 {
		t.Errorf("with no dampening InitialOceanicAge = %v, want 0", got)
	}
}

// Land lies only on the plates chosen to be continental, and with the
// default coverage some plates are wholly oceanic.
func TestCrustContinentalPlates(t *testing.T) {
	for seed := range uint64(5) {
		w := newWorld(t, 6000, earth())
		run(t, w, seed, Plates{}, Crust{})
		areas := w.Mesh.PointAreas()
		land := make([]float64, w.NumPlates)
		area := make([]float64, w.NumPlates)
		for i, a := range areas {
			area[w.Plate[i]] += a
			if w.CrustType[i] == model.Continental {
				land[w.Plate[i]] += a
			}
		}
		var withLand int
		var carrying float64 // area of the plates that carry land
		for k := range land {
			if land[k] > 0 {
				withLand++
				carrying += area[k]
			}
		}
		if withLand == 0 || withLand == w.NumPlates {
			t.Errorf("seed %d: %d of %d plates carry land; want some but not all", seed, withLand, w.NumPlates)
		}
		// The plates chosen have at least 0.3/0.7 of the area, and no
		// more than that plus one plate; those that carry land are
		// among them.
		if share := carrying / (4 * math.Pi); share > 0.3/continentFill+slices.Max(area)/(4*math.Pi)+1e-9 {
			t.Errorf("seed %d: plates carrying land cover %v of the sphere", seed, share)
		}
	}
}

func TestCrustExtremes(t *testing.T) {
	cases := []struct {
		name    string
		samples int
		set     func(*model.Params)
	}{
		{"one plate", 2000, func(p *model.Params) { p.Plates = 1 }},
		{"one plate, all land", 2000, func(p *model.Params) { p.Plates = 1; p.LandCoverage = 1 }},
		{"two plates", 2000, func(p *model.Params) { p.Plates = 2 }},
		{"no land", 2000, func(p *model.Params) { p.LandCoverage = 0 }},
		{"all land", 2000, func(p *model.Params) { p.LandCoverage = 1 }},
		{"four samples", 4, func(p *model.Params) { p.Plates = 4 }},
		{"four samples, one plate", 4, func(p *model.Params) { p.Plates = 1; p.LandCoverage = 0.5 }},
		{"a plate per sample", 80, func(p *model.Params) { p.Plates = 80 }},
		{"small sphere", 2000, func(p *model.Params) { p.Radius = 100; p.PlateWarpAmplitude = 30; p.PlateWarpFeatureSize = 50 }},
		{"other elevations", 2000, func(p *model.Params) {
			p.RidgeElevation, p.AbyssalElevation, p.TrenchElevation, p.MaxContinentalElevation = -0.01, -0.02, -0.03, 0.5
		}},
	}
	for _, c := range cases {
		params := earth()
		c.set(&params)
		w := newWorld(t, c.samples, params)
		run(t, w, 9, Plates{}, Crust{})
		checkCrust(t, w)
		// One sample of 4 is a quarter of the sphere, so the tolerance
		// is the area of the largest sample.
		tol := slices.Max(w.Mesh.PointAreas()) / (4 * math.Pi)
		if got := landShare(w); math.Abs(got-params.LandCoverage) > tol {
			t.Errorf("%s: continental share %v, want %v ± %v", c.name, got, params.LandCoverage, tol)
		}
		if t.Failed() {
			t.Fatalf("case %q failed", c.name)
		}
	}

	// With no land the ocean is everywhere at the abyssal elevation,
	// and with no ocean the land is everywhere at its interior height.
	params := earth()
	params.LandCoverage = 0
	w := newWorld(t, 500, params)
	run(t, w, 1, Plates{}, Crust{})
	for i, z := range w.Elevation {
		if z != -6 {
			t.Fatalf("no land: sample %d is at %v, want -6", i, z)
		}
	}
	params.LandCoverage = 1
	w = newWorld(t, 500, params)
	run(t, w, 1, Plates{}, Crust{})
	for i, z := range w.Elevation {
		if z != 0.5 {
			t.Fatalf("all land: sample %d is at %v, want 0.5", i, z)
		}
	}
}

func TestCrustRepeatable(t *testing.T) {
	make := func(seed uint64) *model.World {
		w := newWorld(t, 4000, earth())
		run(t, w, seed, Plates{}, Crust{})
		return w
	}
	a, b, c := make(11), make(11), make(12)
	if !slices.Equal(a.CrustType, b.CrustType) || !slices.Equal(a.Elevation, b.Elevation) {
		t.Error("the same seed gave different crust")
	}
	if slices.Equal(a.CrustType, c.CrustType) {
		t.Error("different seeds gave the same crust")
	}
}

// The crust stage draws from its own stream: the same plates under a
// different arrangement of land if only the crust's stream changes.
func TestCrustRunsOverExistingData(t *testing.T) {
	w := newWorld(t, 3000, earth())
	run(t, w, 5, Plates{}, Crust{})
	first := slices.Clone(w.CrustType)
	plates := slices.Clone(w.Plate)
	// Running the stage again over its own output gives the same crust.
	if err := (Crust{}).Run(t.Context(), w, model.Stream(5, "crust")); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(first, w.CrustType) || !slices.Equal(plates, w.Plate) {
		t.Error("running Crust twice changed the result")
	}
}

func TestCrustCancelled(t *testing.T) {
	w := newWorld(t, 20000, earth())
	run(t, w, 1, Plates{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (Crust{}).Run(ctx, w, model.Stream(1, "crust")); err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestSmoothstep(t *testing.T) {
	for _, c := range [][2]float64{{-1, 0}, {0, 0}, {0.25, 0.15625}, {0.5, 0.5}, {1, 1}, {7, 1}, {math.Inf(1), 1}} {
		if got := smoothstep(c[0]); got != c[1] {
			t.Errorf("smoothstep(%v) = %v, want %v", c[0], got, c[1])
		}
	}
}
