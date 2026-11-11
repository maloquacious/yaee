// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Dampening follows its closed-form solution,
// z(t) = z_t + (z(0) − z_t) · exp(−ε_o · t / |z_t|), within the error
// AgeOcean states, and never passes z_t. Continental crust is left
// alone.
func TestAgeOcean(t *testing.T) {
	params := earth()
	w := newWorld(t, 200, params)
	start := []float64{params.RidgeElevation, 0, 2.5, -6, -9.99, params.TrenchElevation, -12}
	for i := range w.Elevation {
		w.Elevation[i] = start[i%len(start)]
		w.OceanicAge[i] = float64(i)
		if i%3 == 0 {
			w.CrustType[i] = model.Continental
			w.OceanicAge[i] = 0
		}
	}
	const (
		steps = 125
		dt    = 2.0
	)
	zt, eps := params.TrenchElevation, params.OceanicDampening
	var worst float64
	for s := 1; s <= steps; s++ {
		if err := AgeOcean(t.Context(), w, dt); err != nil {
			t.Fatal(err)
		}
		elapsed := float64(s) * dt
		for i, z := range w.Elevation {
			z0 := start[i%len(start)]
			if w.CrustType[i] == model.Continental {
				if z != z0 || w.OceanicAge[i] != 0 {
					t.Fatalf("continental sample %d changed: elevation %v, oceanic age %v", i, z, w.OceanicAge[i])
				}
				continue
			}
			if got, want := w.OceanicAge[i], float64(i)+elapsed; math.Abs(got-want) > 1e-9 {
				t.Fatalf("sample %d: oceanic age %v after %v My, want %v", i, got, elapsed, want)
			}
			want := zt + (z0-zt)*math.Exp(-eps*elapsed/math.Abs(zt))
			bound := math.Abs(z0-zt)*elapsed*eps*eps*dt/(zt*zt) + 1e-12
			if diff := math.Abs(z - want); diff > bound {
				t.Fatalf("sample %d after %v My: elevation %v, closed form %v, off by %v, bound %v", i, elapsed, z, want, diff, bound)
			} else if z0 == params.RidgeElevation {
				worst = max(worst, diff)
			}
			// It never passes z_t, from above or from below.
			if (z0 >= zt && z < zt) || (z0 <= zt && z > zt) {
				t.Fatalf("sample %d passed the trench elevation: from %v to %v", i, z0, z)
			}
		}
	}
	t.Logf("crust from the ridge elevation: greatest difference from the closed form %.4f km", worst)
	if worst > 0.014 {
		t.Errorf("crust from the ridge elevation is off the closed form by %v km, want at most 0.014", worst)
	}
	// After 250 My crust from the ridge is near the abyssal elevation
	// and still sinking.
	if z := w.Elevation[7]; start[7%len(start)] != params.RidgeElevation || z > -6.5 || z < -7 {
		t.Errorf("crust from the ridge elevation is at %v km after 250 My, want about -6.7", z)
	}

	// A step too long for the formula lands on z_t; no dampening leaves
	// the elevation alone.
	w.Elevation[1], w.Elevation[2] = -1, -11
	if err := AgeOcean(t.Context(), w, 1000); err != nil {
		t.Fatal(err)
	}
	if w.Elevation[1] != zt || w.Elevation[2] != zt {
		t.Errorf("a 1000 My step left elevations %v and %v, want %v", w.Elevation[1], w.Elevation[2], zt)
	}
	w.OceanicDampening = 0
	w.Elevation[1] = -1
	age := w.OceanicAge[1]
	if err := AgeOcean(t.Context(), w, 2); err != nil {
		t.Fatal(err)
	}
	if w.Elevation[1] != -1 || w.OceanicAge[1] != age+2 {
		t.Errorf("without dampening: elevation %v, age %v; want -1, %v", w.Elevation[1], w.OceanicAge[1], age+2)
	}
}

func TestRidgeProfile(t *testing.T) {
	params := earth()
	if z := RidgeProfile(params, 0); z != params.RidgeElevation {
		t.Errorf("RidgeProfile at age 0 = %v, want the ridge elevation %v", z, params.RidgeElevation)
	}
	// It reaches the abyssal elevation at the initial ocean's age, and
	// falls toward the trench elevation without reaching it.
	if z := RidgeProfile(params, InitialOceanicAge(params)); math.Abs(z-params.AbyssalElevation) > 1e-9 {
		t.Errorf("RidgeProfile at the initial age = %v, want %v", z, params.AbyssalElevation)
	}
	prev := params.RidgeElevation
	for age := 1.0; age < 5000; age *= 1.5 {
		z := RidgeProfile(params, age)
		if !(z < prev) || z < params.TrenchElevation {
			t.Fatalf("RidgeProfile(%v) = %v after %v", age, z, prev)
		}
		prev = z
	}
	params.OceanicDampening = 0
	if z := RidgeProfile(params, 100); z != params.RidgeElevation {
		t.Errorf("without dampening RidgeProfile(100) = %v, want %v", z, params.RidgeElevation)
	}
}

// Two plates, the halves of the sphere east and west of the meridian
// plane x = 0, turn about the z axis in opposite directions at ω radians
// per My. Along the half meridian at longitude −90° they part: at
// latitude φ each leaves the meridian at ω · R · cos φ km/My, and after
// T My the gap reaches the angle ω · T of longitude to either side.
//
// With h the sample spacing in radians, the test holds the generator to
// these errors, for samples within 45° of the equator:
//
//   - age: within 1.5 · h / (ω cos φ) My of (distance from the ridge) /
//     (spreading speed), which is (longitude from the ridge) / ω;
//   - the ridge: the samples within half a spacing of the meridian are
//     within 0.5 km of the ridge elevation, and younger than
//     2 · h / (ω cos φ);
//   - plate: a sample more than 1.5 · h from the meridian joins the
//     plate on its own side;
//   - ridge direction: within 8° of the ridge's direction, and within
//     2.5° on average, with the sign of the paper's (q − q′) × q;
//   - continuity: across the join, elevation steps by no more than twice
//     the mean step between neighbours inside the new crust.
func TestDivergingPlates(t *testing.T) {
	const (
		n       = 60000
		omega   = 0.012 // radians per My: 76 km/My at the equator
		elapsed = 20.0  // My
		floor   = -4.0  // km, the elevation of both plates
	)
	params := earth()
	params.MaxPlateSpeed = math.Inf(1)
	w := newWorld(t, n, params)
	w.NumPlates = 2
	w.PlateAxis = []sphere.Vec{{Z: -1}, {Z: 1}} // plate 0 turns west, plate 1 east
	w.PlateSpeed = []float64{omega, omega}
	w.PlateRotation = []sphere.Rotation{sphere.Identity(), sphere.Identity()}
	for i, p := range w.Mesh.Points {
		if p.X > 0 {
			w.Plate[i] = 1
		}
		w.Thickness[i] = 7
		w.Elevation[i] = floor
		w.OceanicAge[i] = 100
	}
	Advance(w, elapsed)
	resample(t, w, elapsed)
	checkCrust(t, w)

	h := math.Sqrt(8 * math.Pi / (math.Sqrt(3) * n)) // sample spacing, radians
	reach := omega * elapsed                         // the gap's half width in longitude
	isNew := make([]bool, n)
	var checked, onRidge int
	var worstDirection, sumDirection, directions float64 // degrees
	for i, q := range w.Mesh.Points {
		lat := math.Asin(q.Z)
		if math.Abs(lat) > math.Pi/4 {
			continue
		}
		cos := math.Cos(lat)
		east := math.Atan2(q.Y, q.X) + math.Pi/2 // longitude from the ridge
		if east > math.Pi {
			east -= 2 * math.Pi
		}
		if math.Abs(east) > reach-1.5*h/cos {
			continue // at the edge of the gap, or outside it
		}
		checked++
		wantAge := math.Abs(east) / omega
		tolerance := 1.5 * h / (omega * cos)
		if got := w.OceanicAge[i]; math.Abs(got-wantAge) > tolerance {
			t.Fatalf("sample %d, %.2f° from the ridge at latitude %.1f°: age %v, want %v ± %v",
				i, east*180/math.Pi, lat*180/math.Pi, got, wantAge, tolerance)
		}
		if w.CrustType[i] != model.Oceanic || w.Thickness[i] != 7 || w.OrogenyType[i] != model.OrogenyNone {
			t.Fatalf("sample %d in the gap is not new oceanic crust", i)
		}
		// Elevation follows the blend for the age the sample got.
		alpha := w.OceanicAge[i] / elapsed
		if want := alpha*floor + (1-alpha)*RidgeProfile(params, w.OceanicAge[i]); math.Abs(w.Elevation[i]-want) > 1e-9 {
			t.Fatalf("sample %d: elevation %v, want %v", i, w.Elevation[i], want)
		}
		if math.Abs(east)*cos < h/2 {
			onRidge++
			if math.Abs(w.Elevation[i]-params.RidgeElevation) > 0.5 || w.OceanicAge[i] > 2*h/(omega*cos) {
				t.Fatalf("sample %d on the ridge: elevation %v, age %v", i, w.Elevation[i], w.OceanicAge[i])
			}
		}
		if math.Abs(east)*cos > 1.5*h {
			side := int32(0)
			if east > 0 {
				side = 1
			}
			if w.Plate[i] != side {
				t.Fatalf("sample %d, %.2f° east of the ridge, joined plate %d", i, east*180/math.Pi, w.Plate[i])
			}
			// (q − q′) × q, with q′ the projection of q onto the ridge.
			onMeridian := sphere.Vec{Y: q.Y, Z: q.Z}.Normalize()
			want := q.Sub(onMeridian).Cross(q).Normalize()
			off := sphere.Angle(w.RidgeDirection[i], want) * 180 / math.Pi
			worstDirection = max(worstDirection, off)
			sumDirection += off
			directions++
		}
	}
	if checked < 1000 || onRidge < 30 {
		t.Fatalf("only %d samples in the gap and %d on the ridge", checked, onRidge)
	}
	t.Logf("ridge direction: off by %.1f° on average, %.1f° at most", sumDirection/directions, worstDirection)
	if worstDirection > 8 || sumDirection/directions > 2.5 {
		t.Errorf("ridge direction is off by %.1f° on average and %.1f° at most; want at most 2.5° and 8°", sumDirection/directions, worstDirection)
	}

	// Continuity across the join. The plates' crust has its age of 100
	// My; new crust is no older than the gap.
	// Toward the poles the gap narrows to a sample or two, which hold
	// the ridge itself next to the old plates; those are left out.
	for i, q := range w.Mesh.Points {
		isNew[i] = w.OceanicAge[i] <= elapsed && math.Abs(q.Z) < math.Sqrt2/2
	}
	var inside, insideCount, join float64
	for i := range w.Mesh.Points {
		if !isNew[i] {
			continue
		}
		for _, j := range w.Mesh.Neighbours(i) {
			step := math.Abs(w.Elevation[i] - w.Elevation[j])
			switch {
			case isNew[j]:
				inside += step
				insideCount++
			case w.OceanicAge[j] > elapsed: // a sample of the old plates
				join = max(join, step)
			}
		}
	}
	mean := inside / insideCount
	t.Logf("%d gap samples; mean step inside the new crust %.3f km, greatest step across the join %.3f km", checked, mean, join)
	if join > 2*mean {
		t.Errorf("elevation steps by %v km across the join; the mean step inside the new crust is %v", join, mean)
	}
}

// gapWorld returns an empty world and a copy of its per-sample data in
// which every sample belongs to the plate that plateOf gives, or to no
// plate where that is negative.
func gapWorld(t testing.TB, n int, plateOf func(p sphere.Vec) int32, elevation float64) (*model.World, *model.World) {
	t.Helper()
	w := newWorld(t, n, earth())
	next := &model.World{
		Plate:          make([]int32, n),
		CrustType:      make([]uint8, n),
		Thickness:      make([]float64, n),
		Elevation:      make([]float64, n),
		OceanicAge:     make([]float64, n),
		RidgeDirection: make([]sphere.Vec, n),
		OrogenyAge:     make([]float64, n),
		OrogenyType:    make([]uint8, n),
		FoldDirection:  make([]sphere.Vec, n),
	}
	for i, p := range w.Mesh.Points {
		next.Plate[i] = plateOf(p)
		if next.Plate[i] >= 0 {
			next.Thickness[i] = 7
			next.Elevation[i] = elevation
			next.OceanicAge[i] = 100
		}
	}
	return w, next
}

// A gap that only one plate borders has no ridge: it closes with the
// elevation of the plate, lowered to sea level.
func TestGapInOnePlate(t *testing.T) {
	hole := func(p sphere.Vec) int32 {
		if p.Z > 0.9 {
			return -1
		}
		return 0
	}
	for _, c := range []struct{ border, want float64 }{{-5, -5}, {2, 0}} {
		w, next := gapWorld(t, 5000, hole, c.border)
		fillGaps(w.Mesh, w.Params, next, 10)
		var filled int
		for i, p := range w.Mesh.Points {
			if hole(p) >= 0 {
				continue
			}
			filled++
			if next.Plate[i] != 0 || next.CrustType[i] != model.Oceanic || next.Thickness[i] != 7 ||
				next.Elevation[i] != c.want || next.OceanicAge[i] != 10 || next.RidgeDirection[i] != (sphere.Vec{}) {
				t.Fatalf("border at %v km: sample %d got plate %d, elevation %v, age %v, ridge direction %v",
					c.border, i, next.Plate[i], next.Elevation[i], next.OceanicAge[i], next.RidgeDirection[i])
			}
		}
		if filled < 100 {
			t.Fatalf("only %d samples in the hole", filled)
		}
	}
}

// With no plate holding anything, every sample gets ridge crust in
// plate 0.
func TestGapEverywhere(t *testing.T) {
	w, next := gapWorld(t, 500, func(sphere.Vec) int32 { return -1 }, 0)
	fillGaps(w.Mesh, w.Params, next, 10)
	for i := range w.Mesh.Points {
		if next.Plate[i] != 0 || next.CrustType[i] != model.Oceanic || next.Thickness[i] != 7 ||
			next.Elevation[i] != w.RidgeElevation || next.OceanicAge[i] != 0 {
			t.Fatalf("sample %d got plate %d, elevation %v, age %v", i, next.Plate[i], next.Elevation[i], next.OceanicAge[i])
		}
	}
}

// Three plates border a round gap. Every gap sample joins the plate of
// the border sample nearest to it, and the ridges, where the age is
// near 0, lie where the two nearest plates are equally near.
func TestGapBetweenThreePlates(t *testing.T) {
	const n = 20000
	plateOf := func(p sphere.Vec) int32 {
		if p.Z > 0.9 {
			return -1
		}
		// Three sectors of longitude.
		return int32((math.Atan2(p.Y, p.X) + math.Pi) / (2 * math.Pi / 3) * (1 - 1e-12))
	}
	w, next := gapWorld(t, n, plateOf, -4)
	var borders []int32
	for i, p := range w.Mesh.Points {
		if plateOf(p) >= 0 && slices.ContainsFunc(w.Mesh.Neighbours(i), func(j int32) bool { return plateOf(w.Mesh.Points[j]) < 0 }) {
			borders = append(borders, int32(i))
		}
	}
	const elapsed = 10.0
	fillGaps(w.Mesh, w.Params, next, elapsed)

	h := math.Sqrt(8 * math.Pi / (math.Sqrt(3) * n))
	var gaps, wrong, ridge int
	seen := make([]int, 3)
	for i, p := range w.Mesh.Points {
		if plateOf(p) >= 0 {
			continue
		}
		gaps++
		// The nearest border sample, and the nearest of another plate.
		a, b := math.Inf(1), math.Inf(1)
		var plateA int32
		for _, s := range borders {
			if d := sphere.Angle(p, w.Mesh.Points[s]); d < a {
				a, plateA = d, next.Plate[s]
			}
		}
		for _, s := range borders {
			if d := sphere.Angle(p, w.Mesh.Points[s]); d < b && next.Plate[s] != plateA {
				b = d
			}
		}
		seen[next.Plate[i]]++
		alpha := (b - a) / (b + a)
		if b-a > h/2 && next.Plate[i] != plateA {
			wrong++
		}
		if got := next.OceanicAge[i] / elapsed; math.Abs(got-alpha) > 0.05 {
			wrong++
		}
		if next.OceanicAge[i] < 0.1*elapsed {
			ridge++
			if math.Abs(next.Elevation[i]-w.RidgeElevation) > 0.5 {
				t.Fatalf("sample %d on a ridge has elevation %v", i, next.Elevation[i])
			}
		}
		if next.Elevation[i] > w.RidgeElevation+1e-9 || next.Elevation[i] < -4-1e-9 {
			t.Fatalf("sample %d: elevation %v is not between the plates' and the ridge's", i, next.Elevation[i])
		}
	}
	t.Logf("%d gap samples, %d on a ridge, %d not as the nearest border samples say; by plate %v", gaps, ridge, wrong, seen)
	if gaps < 500 || ridge < 20 {
		t.Fatalf("only %d gap samples and %d on a ridge", gaps, ridge)
	}
	if wrong > gaps/100 {
		t.Errorf("%d of %d gap samples differ from their nearest border samples", wrong, gaps)
	}
	// Each plate gets about a third of the gap.
	for k, s := range seen {
		if s < gaps/4 || s > gaps/2 {
			t.Errorf("plate %d got %d of %d gap samples, want about a third", k, s, gaps)
		}
	}
}

// Through a whole simulation the ocean stays within its limits: no
// crust below the trench elevation, no oceanic crust older than the
// initial age plus the duration, and new crust wherever plates parted.
func TestSimulateOcean(t *testing.T) {
	w := moving(t, 8000, 4, earth())
	oldest := InitialOceanicAge(w.Params) + w.Duration
	run(t, w, 4, Simulation{})
	checkCrust(t, w)
	var young, directed int
	for i := range w.Mesh.Points {
		if w.CrustType[i] != model.Oceanic {
			continue
		}
		if a := w.OceanicAge[i]; a > oldest+1e-6 {
			t.Fatalf("sample %d: oceanic age %v, more than the %v the run allows", i, a, oldest)
		} else if a < w.Duration {
			young++
			// Crust formed during the run is where the dampening puts
			// crust of its age, or shallower, having been blended with
			// nothing deeper than the plates beside it at the time.
			if w.RidgeDirection[i] != (sphere.Vec{}) {
				directed++
			}
		}
	}
	if young < 500 || directed < young/2 {
		t.Errorf("%d samples of crust formed during the run, %d with a ridge direction", young, directed)
	}
}
