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

const (
	oceanFloor = -4.0 // km, the elevation of oceanic crust in these tests
	landLevel  = 0.5  // km, the elevation of continental crust
)

// halves returns a world of two plates, each half of the sphere: plate 1
// holds the samples with x > 0 and plate 0 the rest. Plate k has crust
// of type crust[k], with oceanic age age[k] if it is oceanic, and turns
// about axis[k] at omega[k] radians per My.
//
// With the axes −z for plate 0 and +z for plate 1, plate 0 turns west
// and plate 1 east: they converge along the meridian of longitude 90°
// and diverge along that of −90°.
func halves(t testing.TB, n int, crust [2]uint8, age, omega [2]float64, axis [2]sphere.Vec) *model.World {
	t.Helper()
	w := newWorld(t, n, earth())
	w.NumPlates = 2
	w.PlateAxis = []sphere.Vec{axis[0].Normalize(), axis[1].Normalize()}
	w.PlateSpeed = omega[:]
	w.PlateRotation = []sphere.Rotation{sphere.Identity(), sphere.Identity()}
	for i, p := range w.Mesh.Points {
		k := 0
		if p.X > 0 {
			k = 1
		}
		w.Plate[i] = int32(k)
		w.CrustType[i] = crust[k]
		if crust[k] == model.Oceanic {
			w.Thickness[i], w.Elevation[i], w.OceanicAge[i] = 7, oceanFloor, age[k]
		} else {
			w.Thickness[i], w.Elevation[i] = 35, landLevel
		}
	}
	return w
}

var (
	westEast = [2]sphere.Vec{{Z: -1}, {Z: 1}}
	ocean    = model.Oceanic
	land     = model.Continental
)

// longitude returns the longitude of p in degrees from 90° east: 0 on
// the meridian where the plates of halves converge, positive to the
// east of it.
func longitude(p sphere.Vec) float64 {
	d := math.Atan2(p.Y, p.X)*180/math.Pi - 90
	if d < -180 {
		d += 360
	}
	return d
}

func steps(t testing.TB, s *Subduction, w *model.World, n int, dt float64) {
	t.Helper()
	for range n {
		if err := s.Step(t.Context(), w, dt); err != nil {
			t.Fatalf("Step: %v", err)
		}
	}
}

func TestOverrides(t *testing.T) {
	for _, c := range []struct {
		name         string
		typeA        uint8
		ageA         float64
		plateA       int32
		typeB        uint8
		ageB         float64
		plateB       int32
		aOverrides   bool
		whichSubduct string
	}{
		{"continental over oceanic", land, 0, 5, ocean, 10, 1, true, "B"},
		{"continental over oceanic, whatever the ages", land, 200, 5, ocean, 10, 1, true, "B"},
		{"oceanic under continental", ocean, 10, 1, land, 0, 5, false, "A"},
		{"younger oceanic plate over older", ocean, 30, 7, ocean, 80, 2, true, "B"},
		{"older oceanic plate under younger", ocean, 80, 2, ocean, 30, 7, false, "A"},
		{"equal oceanic ages: lower plate number on top", ocean, 50, 2, ocean, 50, 7, true, "B"},
		{"equal oceanic ages: higher plate number subducts", ocean, 50, 7, ocean, 50, 2, false, "A"},
		{"two continents: lower plate number on top", land, 90, 3, land, 10, 4, true, "B"},
		{"two continents: higher plate number subducts", land, 10, 4, land, 90, 3, false, "A"},
	} {
		got := Overrides(c.typeA, c.ageA, c.plateA, c.typeB, c.ageB, c.plateB)
		back := Overrides(c.typeB, c.ageB, c.plateB, c.typeA, c.ageA, c.plateA)
		if got != c.aOverrides || back == got {
			t.Errorf("%s: Overrides(A, B) = %v and Overrides(B, A) = %v, want %v and %v", c.name, got, back, c.aOverrides, !c.aOverrides)
		}
	}
}

func TestPlateOceanAges(t *testing.T) {
	w := halves(t, 2000, [2]uint8{ocean, land}, [2]float64{40, 0}, [2]float64{}, westEast)
	w.OceanicAge[slices.Index(w.Plate, 0)] = 40 + 2000 // one sample older
	ages := PlateOceanAges(w)
	count := 0
	for _, k := range w.Plate {
		if k == 0 {
			count++
		}
	}
	if want := 40 + 2000/float64(count); math.Abs(ages[0]-want) > 1e-9 || ages[1] != 0 {
		t.Fatalf("PlateOceanAges = %v, want [%v 0]", ages, want)
	}
}

// Relative speed is zero for two plates that move alike, and matches the
// analytic value for two that turn opposite ways about one axis.
func TestRelativeSpeed(t *testing.T) {
	const omega = 0.006
	w := halves(t, 500, [2]uint8{ocean, ocean}, [2]float64{}, [2]float64{omega, omega}, [2]sphere.Vec{{X: 1, Y: 2, Z: 3}, {X: 1, Y: 2, Z: 3}})
	for _, p := range w.Mesh.Points[:50] {
		if v := RelativeSpeed(w, 0, 1, p); v > 1e-9 {
			t.Fatalf("same axis and speed: relative speed %v at %v, want 0", v, p)
		}
	}
	w = halves(t, 500, [2]uint8{ocean, ocean}, [2]float64{}, [2]float64{omega, omega}, westEast)
	for _, p := range w.Mesh.Points[:50] {
		// Each moves at ω R cos(latitude), in opposite directions.
		want := 2 * omega * w.Radius * math.Sqrt(1-p.Z*p.Z)
		if v := RelativeSpeed(w, 0, 1, p); math.Abs(v-want) > 1e-9 {
			t.Fatalf("opposite turns: relative speed %v at %v, want %v", v, p, want)
		}
		if RelativeSpeed(w, 0, 1, p) != RelativeSpeed(w, 1, 0, p) {
			t.Fatal("relative speed depends on the order of the plates")
		}
	}
}

func TestTransferFunctions(t *testing.T) {
	const rs = 1800.0
	d0 := rs * controlFraction
	for _, c := range []struct{ d, want float64 }{
		{0, 0}, {d0, 1}, {rs, 0}, {-5, 0}, {rs + 1, 0}, {math.Inf(1), 0}, {d0 / 2, 0.5}, {(d0 + rs) / 2, 0.5},
	} {
		if got := DistanceTransfer(c.d, rs); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("DistanceTransfer(%v, %v) = %v, want %v", c.d, rs, got, c.want)
		}
	}
	last := 0.0
	for d := 0.0; d <= rs; d += 0.5 {
		f := DistanceTransfer(d, rs)
		if f < 0 || f > 1 {
			t.Fatalf("DistanceTransfer(%v) = %v is outside 0 to 1", d, f)
		}
		if d <= d0 && f < last || d > d0 && f > last {
			t.Fatalf("DistanceTransfer does not rise to %v km and fall after it: %v then %v at %v km", d0, last, f, d)
		}
		last = f
	}
	if DistanceTransfer(100, 0) != 0 {
		t.Error("DistanceTransfer with r_s = 0 is not 0")
	}

	if SpeedTransfer(0, 100) != 0 || SpeedTransfer(100, 100) != 1 || SpeedTransfer(50, 100) != 0.5 || SpeedTransfer(200, 100) != 2 {
		t.Error("SpeedTransfer is not v/v0")
	}
	if SpeedTransfer(10, 0) != 0 {
		t.Error("SpeedTransfer with v0 = 0 is not 0")
	}

	p := earth()
	for _, c := range []struct{ z, want float64 }{
		{p.TrenchElevation, 0}, {p.MaxContinentalElevation, 1}, {0, 0.25}, {-6, 0.04}, {-20, 0}, {30, 1},
	} {
		if got := HeightTransfer(p, c.z); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("HeightTransfer(%v) = %v, want %v", c.z, got, c.want)
		}
	}
}

// subducted returns how many samples of each plate are marked as
// subducted, and fails the test if one is further than tolerance degrees
// of longitude from the meridian where the plates of halves converge,
// measured where the sample now is.
func subducted(t testing.TB, w *model.World, tolerance float64) [2]int {
	t.Helper()
	var count [2]int
	for i, gone := range w.Subducted {
		if !gone {
			continue
		}
		count[w.Plate[i]]++
		if at := longitude(Position(w, i)); math.Abs(at) > tolerance {
			t.Fatalf("sample %d of plate %d is subducted %v° from the convergent boundary", i, w.Plate[i], at)
		}
	}
	return count
}

// The plate that subducts is the oceanic one beneath a continental one,
// whichever is faster, and of two oceanic plates the older.
func TestSubductingPlate(t *testing.T) {
	const (
		n     = 20000
		omega = 0.006 // 38 km/My at the equator
	)
	for _, c := range []struct {
		name   string
		crust  [2]uint8
		age    [2]float64
		omega  [2]float64
		loser  int
		winner int
	}{
		{"oceanic 0 into continental 1", [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{omega, omega}, 0, 1},
		{"oceanic 1 into continental 0", [2]uint8{land, ocean}, [2]float64{0, 100}, [2]float64{omega, omega}, 1, 0},
		{"fast oceanic into still continental", [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{2 * omega, 0}, 0, 1},
		{"fast continental over still oceanic", [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{0, 2 * omega}, 0, 1},
		{"older oceanic 0 beneath younger 1", [2]uint8{ocean, ocean}, [2]float64{120, 40}, [2]float64{omega, omega}, 0, 1},
		{"older oceanic 1 beneath younger 0", [2]uint8{ocean, ocean}, [2]float64{40, 120}, [2]float64{omega, omega}, 1, 0},
		{"two continents: the higher number", [2]uint8{land, land}, [2]float64{}, [2]float64{omega, omega}, 1, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			w := halves(t, n, c.crust, c.age, c.omega, westEast)
			s := NewSubduction(w)
			steps(t, s, w, 4, 2)
			// 8 My at a closing speed of 76 km/My: 5.5° of longitude.
			count := subducted(t, w, 8)
			if count[c.loser] == 0 || count[c.winner] != 0 {
				t.Fatalf("subducted samples per plate %v: want only plate %d", count, c.loser)
			}
			if len(s.Front(c.winner)) == 0 || len(s.Front(c.loser)) != 0 {
				t.Fatalf("front samples: %d on plate 0 and %d on plate 1, want all on plate %d", len(s.Front(0)), len(s.Front(1)), c.winner)
			}
			for _, i := range s.Front(c.winner) {
				if s.Under(int(i)) != c.loser || w.Plate[i] != int32(c.winner) || w.Subducted[i] {
					t.Fatalf("front sample %d: plate %d over plate %d", i, w.Plate[i], s.Under(int(i)))
				}
				if at := longitude(Position(w, int(i))); math.Abs(at) > 8 {
					t.Fatalf("front sample %d is %v° from the convergent boundary", i, at)
				}
			}
			for i := range w.Plate {
				if _, _, ok := s.FrontDistance(w, i); ok && w.Plate[i] != int32(c.winner) {
					t.Fatalf("sample %d of the subducting plate is in a subduction region", i)
				}
			}

			// After resampling, the overlap holds the overriding plate's
			// crust, and every sample has one plate.
			before := snap(w)
			rot := slices.Clone(w.PlateRotation)
			resample(t, w, 8)
			checkCrust(t, w)
			overlap := 0
			for i, p := range w.Mesh.Points {
				if w.Plate[i] != 0 && w.Plate[i] != 1 {
					t.Fatalf("sample %d has plate %d", i, w.Plate[i])
				}
				if w.Subducted[i] {
					t.Fatalf("sample %d is still marked as subducted", i)
				}
				if math.Abs(p.Z) > 0.5 || math.Abs(longitude(p)) > 15 {
					continue
				}
				// Where both plates lay over this sample, it is the
				// winner's: the point is inside both halves, turned.
				in := func(k int) bool {
					x := rot[k].Inverse().Apply(p).X
					return k == 1 && x > 0.03 || k == 0 && x < -0.03
				}
				if in(0) && in(1) {
					overlap++
					if w.Plate[i] != int32(c.winner) || w.CrustType[i] != c.crust[c.winner] {
						t.Fatalf("sample %d in the overlap: plate %d, crust type %d; want the overriding plate %d", i, w.Plate[i], w.CrustType[i], c.winner)
					}
				}
				// No new crust where plates converge.
				if w.CrustType[i] == ocean && w.OceanicAge[i] < 30 {
					t.Fatalf("sample %d at the convergent boundary is new oceanic crust, age %v", i, w.OceanicAge[i])
				}
			}
			if overlap < 20 {
				t.Fatalf("only %d samples in the overlap", overlap)
			}
			_ = before
		})
	}
}

// Plates that slide past each other, or move apart, are not converging.
func TestNotConverging(t *testing.T) {
	const omega = 0.006
	// Turning about the x axis moves each plate along the boundary, the
	// great circle x = 0.
	w := halves(t, 20000, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{omega, omega}, [2]sphere.Vec{{X: 1}, {X: -1}})
	s := NewSubduction(w)
	steps(t, s, w, 6, 2)
	// The edge of a plate is jagged at the scale of one sample, so teeth
	// of the oceanic plate do pass beneath teeth of the other; nothing
	// further in does.
	spacing := sphere.SampleSpacing(w.Radius, 20000)
	for i, gone := range w.Subducted {
		if gone && (w.Plate[i] != 0 || math.Abs(w.Mesh.Points[i].X)*w.Radius > 1.5*spacing) {
			t.Fatalf("sliding plates: sample %d of plate %d, %v km from the boundary, is subducted",
				i, w.Plate[i], math.Abs(w.Mesh.Points[i].X)*w.Radius)
		}
	}
	if len(s.Front(0))+len(s.Front(1)) != 0 {
		t.Fatal("sliding plates have a subduction front")
	}
	if slices.ContainsFunc(w.Uplift, func(u float64) bool { return u != 0 }) {
		t.Fatal("sliding plates were uplifted")
	}

	// Plates turning opposite ways about z move apart along the meridian
	// of −90°: nothing subducts there, and no front forms there.
	w = halves(t, 20000, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{omega, omega}, westEast)
	s = NewSubduction(w)
	steps(t, s, w, 6, 2)
	subducted(t, w, 10)
	for k := range 2 {
		for _, i := range s.Front(k) {
			if at := longitude(Position(w, int(i))); math.Abs(at) > 10 {
				t.Fatalf("front sample %d is %v° from the convergent boundary", i, at)
			}
		}
	}
}

// A plate holds nothing where its crust is subducted.
func TestQuerySubducted(t *testing.T) {
	w := halves(t, 2000, [2]uint8{ocean, land}, [2]float64{}, [2]float64{}, westEast)
	i := slices.Index(w.Plate, 1)
	p := w.Mesh.Points[i]
	if _, _, ok := Query(w, 1, p); !ok {
		t.Fatal("plate 1 does not hold its own sample")
	}
	w.Subducted[i] = true
	if _, weights, ok := Query(w, 1, p); ok || weights != ([3]float64{}) {
		t.Fatal("plate 1 holds a sample that is subducted")
	}
}

// converging returns halves with oceanic plate 0 subducting beneath
// continental plate 1, after one step of 2 My, with the fronts found.
func converging(t testing.TB, n int) (*model.World, *Subduction) {
	t.Helper()
	w := halves(t, n, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{0.006, 0.006}, westEast)
	s := NewSubduction(w)
	if err := s.Advance(t.Context(), w, 2); err != nil {
		t.Fatal(err)
	}
	if err := s.Fronts(t.Context(), w); err != nil {
		t.Fatal(err)
	}
	return w, s
}

// d(p) is within 1.5 sample spacings of the distance to the front, and no
// more than half a spacing above the distance to the nearest front
// sample; nothing beyond r_s is in the subduction region.
func TestFrontDistance(t *testing.T) {
	const n = 60000
	w, s := converging(t, n)
	spacing := sphere.SampleSpacing(w.Radius, n)
	front := s.Front(1)
	if len(front) < 100 {
		t.Fatalf("%d front samples", len(front))
	}
	checked := 0
	var worstLine, worstNearest float64
	for i, p := range w.Mesh.Points {
		// The front is the edge of plate 1 on the side y > 0: half of the
		// great circle x = 0. The distance to it is asin(x) radians.
		line := math.Asin(max(0, p.X)) * w.Radius
		d, to, ok := s.FrontDistance(w, i)
		if w.Plate[i] == 0 {
			if ok {
				t.Fatalf("sample %d of the subducting plate has a front distance", i)
			}
			continue
		}
		if p.Y < 0.5 || math.Abs(p.Z) > 0.6 {
			continue // near the ends of the front
		}
		switch {
		case line > w.SubductionDistance+1.5*spacing:
			if ok {
				t.Fatalf("sample %d, %v km from the front, is in the subduction region", i, line)
			}
			continue
		case line > w.SubductionDistance-1.5*spacing:
			continue
		case !ok:
			t.Fatalf("sample %d, %v km from the front, is not in the subduction region", i, line)
		}
		checked++
		if got := sphere.Angle(p, w.Mesh.Points[to]) * w.Radius; math.Abs(got-d) > 1e-6 {
			t.Fatalf("sample %d: distance %v, but its front sample %d is %v km away", i, d, to, got)
		}
		nearest := math.Inf(1)
		for _, f := range front {
			nearest = min(nearest, sphere.Angle(p, w.Mesh.Points[f])*w.Radius)
		}
		if d < nearest-1e-6 {
			t.Fatalf("sample %d: distance %v is less than the %v to the nearest front sample", i, d, nearest)
		}
		worstNearest = max(worstNearest, d-nearest)
		worstLine = max(worstLine, math.Abs(d-line))
	}
	if checked < 1000 {
		t.Fatalf("only %d samples checked", checked)
	}
	if worstNearest > spacing/2 || worstLine > 1.5*spacing {
		t.Fatalf("front distance is up to %.1f km above the nearest front sample (limit %.1f) and %.1f km from the true distance (limit %.1f)",
			worstNearest, spacing/2, worstLine, 1.5*spacing)
	}
}

// One step of uplift raises every sample of the subduction region by
// the amount the formula gives, and nothing else.
func TestUplift(t *testing.T) {
	const (
		n  = 60000
		dt = 2.0
	)
	w, s := converging(t, n)
	before := snap(w)
	if err := s.Uplift(t.Context(), w, dt); err != nil {
		t.Fatal(err)
	}
	checkCrust(t, w)
	raised, byDistance := 0, map[int]int{}
	for i, p := range w.Mesh.Points {
		d, _, ok := s.FrontDistance(w, i)
		z0 := before.elevation[i]
		if !ok {
			if w.Elevation[i] != z0 || w.Uplift[i] != 0 || w.FoldDirection[i] != (sphere.Vec{}) || w.OrogenyType[i] != model.OrogenyNone {
				t.Fatalf("sample %d outside the subduction region was changed", i)
			}
			continue
		}
		x := Position(w, i)
		v := RelativeSpeed(w, 0, 1, x)
		// By hand: u_0 · f(d) · g(v) · h(z̃), the slab being ocean floor,
		// and the fade toward z_c.
		f := DistanceTransfer(d, 1800)
		h := math.Pow((oceanFloor-(-10))/(10-(-10)), 2)
		want := 0.6 * f * (v / 100) * h * (1 - landLevel/10)
		if math.Abs(w.Uplift[i]-want) > 1e-12 || math.Abs(w.Elevation[i]-(z0+want*dt)) > 1e-12 {
			t.Fatalf("sample %d, %v km from the front: uplift %v and elevation %v, want %v and %v",
				i, d, w.Uplift[i], w.Elevation[i], want, z0+want*dt)
		}
		if want > 0 {
			raised++
			byDistance[int(d/300)]++
			if w.OrogenyType[i] != model.OrogenyAndean || w.OrogenyAge[i] != 0 {
				t.Fatalf("sample %d was raised: orogeny type %d, age %v", i, w.OrogenyType[i], w.OrogenyAge[i])
			}
		}
		// The fold direction is tangent and points the way the oceanic
		// plate moves relative to this one: west.
		fold := w.PlateRotation[1].Apply(w.FoldDirection[i])
		rel := Velocity(w, 0, x).Sub(Velocity(w, 1, x))
		if math.Abs(fold.Dot(x)) > 1e-12 || fold.Len() == 0 || fold.Dot(rel) < 0.999999*fold.Len()*rel.Len() {
			t.Fatalf("sample %d: fold direction %v, relative velocity %v", i, fold, rel)
		}
		if got, want := fold.Len(), foldWeight*v*dt; math.Abs(got-want) > 1e-12 {
			t.Fatalf("sample %d: fold length %v, want %v", i, got, want)
		}
		_ = p
	}
	if raised < 3000 || len(byDistance) < 6 {
		t.Fatalf("%d samples raised, at %d bands of distance", raised, len(byDistance))
	}

	// A second step ages the orogeny that has begun, and turns the fold
	// no further from the direction of convergence.
	if err := s.Uplift(t.Context(), w, dt); err != nil {
		t.Fatal(err)
	}
	for i := range w.Mesh.Points {
		if w.OrogenyType[i] == model.OrogenyAndean && w.OrogenyAge[i] != dt {
			t.Fatalf("sample %d: orogeny age %v after a second step, want %v", i, w.OrogenyAge[i], dt)
		}
	}
}

// Crust that subduction raises above sea level becomes Andean with
// orogeny age 0, whatever orogeny it had.
func TestUpliftEmerges(t *testing.T) {
	w, s := converging(t, 20000)
	w.SubductionUplift = 50
	var shelf []int
	for i := range w.Mesh.Points {
		if d, _, ok := s.FrontDistance(w, i); ok && d > 200 && d < 500 {
			w.Elevation[i] = -0.001
			w.OrogenyType[i], w.OrogenyAge[i] = model.OrogenyHimalayan, 70
			shelf = append(shelf, i)
		}
	}
	if len(shelf) < 20 {
		t.Fatalf("%d samples on the shelf", len(shelf))
	}
	if err := s.Uplift(t.Context(), w, 2); err != nil {
		t.Fatal(err)
	}
	for _, i := range shelf {
		if w.Elevation[i] <= 0 || w.OrogenyType[i] != model.OrogenyAndean || w.OrogenyAge[i] != 0 {
			t.Fatalf("sample %d: elevation %v, orogeny type %d, age %v", i, w.Elevation[i], w.OrogenyType[i], w.OrogenyAge[i])
		}
	}
}

// No elevation passes z_c, however strong the uplift and however long
// it lasts, and none falls below z_t.
func TestElevationLimits(t *testing.T) {
	w := halves(t, 20000, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{0.006, 0.006}, westEast)
	w.SubductionUplift = 400
	w.SedimentAccretion = 0
	s := NewSubduction(w)
	top := math.Inf(-1)
	for range 12 {
		steps(t, s, w, 3, 2)
		checkCrust(t, w) // z_t ≤ z ≤ z_c
		resample(t, w, 6)
		s.Reset(w)
		checkCrust(t, w)
		top = max(top, slices.Max(w.Elevation))
	}
	if top < 0.99*w.MaxContinentalElevation {
		t.Fatalf("the highest elevation reached is %v km: the uplift was not strong enough to test the limit", top)
	}
}

// The uplift of one step does not depend on the sample spacing, within
// 15% of the greatest uplift: compare it at set distances from the front
// at two resolutions.
func TestUpliftIndependentOfResolution(t *testing.T) {
	// The mean uplift of the samples near the equator in bands 100 km
	// wide centred at each distance from the true front.
	at := []float64{450, 700, 1000, 1300, 1600}
	profile := func(n int) []float64 {
		w, s := converging(t, n)
		if err := s.Uplift(t.Context(), w, 2); err != nil {
			t.Fatal(err)
		}
		sum, count := make([]float64, len(at)), make([]int, len(at))
		for i, p := range w.Mesh.Points {
			if w.Plate[i] != 1 || p.Y < 0.5 || math.Abs(p.Z) > 0.2 {
				continue
			}
			line := math.Asin(p.X) * w.Radius
			for b, d := range at {
				if math.Abs(line-d) <= 50 {
					sum[b] += w.Uplift[i]
					count[b]++
				}
			}
		}
		for b := range sum {
			if count[b] < 5 {
				t.Fatalf("%d samples: only %d in the band at %v km", n, count[b], at[b])
			}
			sum[b] /= float64(count[b])
		}
		return sum
	}
	coarse, fine := profile(30000), profile(120000)
	peak := slices.Max(fine)
	for b := range at {
		if math.Abs(coarse[b]-fine[b]) > 0.15*peak {
			t.Errorf("uplift %v km from the front: %v km/My at 30000 samples, %v at 120000 (peak %v)", at[b], coarse[b], fine[b], peak)
		}
	}
	if !(fine[0] > fine[2] && fine[2] > fine[4] && fine[4] > 0) {
		t.Errorf("uplift does not fall with distance from the front: %v", fine)
	}
}

func TestSlabPull(t *testing.T) {
	// No front: the axis stays.
	w := halves(t, 20000, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{0.006, 0.006}, westEast)
	s := NewSubduction(w)
	s.SlabPull(w, 2)
	if w.PlateAxis[0] != westEast[0] || w.PlateAxis[1] != westEast[1] || w.PlateSpeed[0] != 0.006 {
		t.Fatal("slab pull changed a plate that has no subduction front")
	}

	// Plate 0 subducts along the meridian of 90°, and turns about −z.
	// Give it an axis that carries it along the front instead, and the
	// pull turns the axis toward one that carries it into the front.
	pull := func(n int) (change sphere.Vec, w *model.World, s *Subduction) {
		w, s = converging(t, n)
		// Two more steps, so that the front is whole at both spacings.
		for range 2 {
			if err := s.Advance(t.Context(), w, 2); err != nil {
				t.Fatal(err)
			}
			if err := s.Fronts(t.Context(), w); err != nil {
				t.Fatal(err)
			}
		}
		w.PlateAxis[0] = sphere.Vec{X: 1}
		before := w.PlateAxis[0]
		fastest := maxSpeed(w)
		s.SlabPull(w, 2)
		for k, a := range w.PlateAxis {
			if math.Abs(a.Len()-1) > 1e-12 {
				t.Fatalf("axis of plate %d has length %v", k, a.Len())
			}
		}
		if w.PlateAxis[1] != westEast[1] {
			t.Fatal("slab pull changed the overriding plate")
		}
		if got := maxSpeed(w); got > fastest*(1+1e-12) || got > w.MaxPlateSpeed {
			t.Fatalf("fastest sample moves at %v km/My after the pull, %v before", got, fastest)
		}
		return w.PlateAxis[0].Sub(before), w, s
	}
	change, w, s := pull(40000)
	if change.Len() == 0 {
		t.Fatal("slab pull did not change the subducting plate's axis")
	}
	// The plate's centroid is at (−1, 0, 0) turned; the front is toward
	// +y from it. Turning about −z carries the centroid toward the front.
	c := w.PlateRotation[0].Apply(sphere.Vec{X: -1})
	var mean sphere.Vec
	for _, i := range s.Front(1) {
		mean = mean.Add(Position(w, int(i)))
	}
	toward := c.Cross(mean.Normalize()).Normalize()
	if change.Dot(toward) < 0.9*change.Len() {
		t.Fatalf("the axis changed by %v, not toward %v, the axis that turns the plate into its front", change, toward)
	}
	if v := Velocity(w, 0, c); v.Dot(mean) <= 0 {
		t.Fatalf("after the pull the plate's centroid moves at %v, away from its front", v)
	}

	// The same front at half the spacing pulls as hard, within 20%: a
	// jagged edge has more samples along it than its length in spacings.
	fine, _, _ := pull(160000)
	if ratio := fine.Len() / change.Len(); math.Abs(ratio-1) > 0.2 {
		t.Fatalf("axis change %v at 40000 samples and %v at 160000: ratio %v", change.Len(), fine.Len(), ratio)
	}
}

// A trench forms in front of the subduction front, on the subducting
// plate, no deeper than z_t.
func TestTrenchForms(t *testing.T) {
	const n = 120000
	w, s := converging(t, n)
	spacing := sphere.SampleSpacing(w.Radius, n)
	if err := s.Trenches(t.Context(), w, 2); err != nil {
		t.Fatal(err)
	}
	checkCrust(t, w)
	front := s.Front(1)
	trench, deepest := 0, 0.0
	for i := range w.Mesh.Points {
		x := Position(w, i)
		nearest := math.Inf(1)
		if math.Abs(longitude(x)) < 6 {
			for _, f := range front {
				nearest = min(nearest, sphere.Angle(x, Position(w, int(f)))*w.Radius)
			}
		}
		depth := w.Trench[i]
		if depth == 0 {
			if w.Elevation[i] != oceanFloor && w.Elevation[i] != landLevel {
				t.Fatalf("sample %d has no trench but its elevation changed to %v", i, w.Elevation[i])
			}
			if w.Plate[i] == 0 && !w.Subducted[i] && nearest < trenchWidth-spacing/2 {
				t.Fatalf("sample %d of the subducting plate, %v km from the front, has no trench", i, nearest)
			}
			continue
		}
		trench++
		deepest = max(deepest, depth)
		if w.Plate[i] != 0 || w.CrustType[i] != ocean || w.Subducted[i] {
			t.Fatalf("sample %d has a trench: plate %d, crust type %d, subducted %v", i, w.Plate[i], w.CrustType[i], w.Subducted[i])
		}
		if math.Abs(w.Elevation[i]-(oceanFloor-depth)) > 1e-12 || w.Elevation[i] < w.TrenchElevation {
			t.Fatalf("sample %d: elevation %v with a trench of %v km", i, w.Elevation[i], depth)
		}
		// The profile, for a distance within half a spacing of that to
		// the nearest front sample.
		least := (oceanFloor - w.TrenchElevation) * math.Pow(max(0, 1-(nearest+spacing/2)/trenchWidth), 2)
		most := (oceanFloor - w.TrenchElevation) * math.Pow(max(0, 1-nearest/trenchWidth), 2)
		if depth < least-1e-9 || depth > most+1e-9 {
			t.Fatalf("sample %d, %v km from the front: trench %v km, want %v to %v", i, nearest, depth, least, most)
		}
	}
	if trench < 100 || deepest < 0.5*(oceanFloor-w.TrenchElevation) {
		t.Fatalf("%d trench samples, the deepest lowered by %v km", trench, deepest)
	}

	// Forming it again changes nothing: the profile is a state, not a
	// rate.
	w.SedimentAccretion = 0
	before := slices.Clone(w.Elevation)
	if err := s.Trenches(t.Context(), w, 2); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(before, w.Elevation) {
		t.Fatal("forming a trench twice lowered it twice")
	}
}

// Sediment raises a trench that is no longer being formed at ε_t until
// the trench is gone, and touches nothing else.
func TestSedimentAccretion(t *testing.T) {
	w := halves(t, 2000, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{}, westEast)
	s := NewSubduction(w)
	in := slices.Index(w.Plate, 0)
	w.Trench[in], w.Elevation[in] = 2, oceanFloor-2
	before := slices.Clone(w.Elevation)
	for step, want := range []float64{1.4, 0.8, 0.2, 0, 0} {
		if err := s.Trenches(t.Context(), w, 2); err != nil {
			t.Fatal(err)
		}
		if math.Abs(w.Trench[in]-want) > 1e-12 || math.Abs(w.Elevation[in]-(oceanFloor-want)) > 1e-12 {
			t.Fatalf("after step %d: trench %v at elevation %v, want %v at %v", step+1, w.Trench[in], w.Elevation[in], want, oceanFloor-want)
		}
	}
	for i := range before {
		if i != in && (w.Elevation[i] != before[i] || w.Trench[i] != 0) {
			t.Fatalf("sample %d outside a trench was changed", i)
		}
	}
}

func TestErode(t *testing.T) {
	const dt = 2.0
	w := halves(t, 2000, [2]uint8{ocean, land}, [2]float64{100, 0}, [2]float64{}, westEast)
	high, shelf, sea := -1, -1, slices.Index(w.Plate, 0)
	for i, k := range w.Plate {
		if k == 1 && high < 0 {
			high = i
		} else if k == 1 && shelf < 0 {
			shelf = i
		}
	}
	w.Elevation[high], w.Elevation[shelf], w.Elevation[sea] = 10, -0.2, 3
	for range 125 {
		if err := Erode(t.Context(), w, dt); err != nil {
			t.Fatal(err)
		}
	}
	// z(t) = z(0) · exp(−ε_c · t / z_c), and the stated error.
	want := 10 * math.Exp(-0.03*250/10)
	bound := 10 * 250 * 0.03 * 0.03 * dt / (2 * 10 * 10)
	if got := w.Elevation[high]; got > want || want-got > bound {
		t.Fatalf("10 km of land after 250 My: %v km, want %v less at most %v", got, want, bound)
	}
	if w.Elevation[shelf] != -0.2 || w.Elevation[sea] != 3 {
		t.Fatalf("erosion changed continental crust below sea level (%v) or oceanic crust (%v)", w.Elevation[shelf], w.Elevation[sea])
	}
	// A step too long for the formula lands on sea level.
	w.ContinentalErosion = 1000
	if err := Erode(t.Context(), w, dt); err != nil {
		t.Fatal(err)
	}
	for i, z := range w.Elevation {
		if w.CrustType[i] == land && i != shelf && z != 0 {
			t.Fatalf("sample %d: %v km after a step that erodes everything", i, z)
		}
	}
}

// A fragment of continental crust narrower than the distance it moves
// between two resamplings, and than the distance it moves in a step,
// leaves a continuous wake of new crust on the plate it crosses, not a
// row of separate footprints (issue #6).
func TestFragmentLeavesNoFootprints(t *testing.T) {
	const (
		n      = 30000
		radius = 0.05 // radians: a fragment 640 km across
		dt     = 2.0
	)
	for _, c := range []struct {
		name    string
		omega   float64 // of the fragment, about z
		perStep float64 // km moved in a step, at the equator
	}{
		{"narrower than a resampling's travel", 0.015, 191},
		{"narrower than a step's travel", 0.07, 892},
	} {
		t.Run(c.name, func(t *testing.T) {
			params := earth()
			params.MaxPlateSpeed = math.Inf(1)
			w := newWorld(t, n, params)
			w.NumPlates = 2
			w.PlateAxis = []sphere.Vec{{Z: 1}, {Z: 1}}
			w.PlateSpeed = []float64{0, c.omega}
			w.PlateRotation = []sphere.Rotation{sphere.Identity(), sphere.Identity()}
			start := sphere.Vec{X: 1}
			for i, p := range w.Mesh.Points {
				w.Thickness[i], w.Elevation[i], w.OceanicAge[i] = 7, oceanFloor, 100
				if sphere.Angle(p, start) < radius {
					w.Plate[i], w.CrustType[i] = 1, land
					w.Thickness[i], w.Elevation[i], w.OceanicAge[i] = 35, landLevel, 0
				}
			}
			s := NewSubduction(w)
			const stepsPerResampling, resamplings = 6, 3
			for range resamplings {
				steps(t, s, w, stepsPerResampling, dt)
				resample(t, w, stepsPerResampling*dt)
				s.Reset(w)
				checkCrust(t, w)
			}
			turned := c.omega * dt * stepsPerResampling * resamplings // radians of longitude
			if hop := c.omega * dt * stepsPerResampling; hop < 2*radius {
				t.Fatalf("the fragment moves %v radians between resamplings, less than its width", hop)
			}
			// Along the middle of the fragment's path, from behind where
			// it started to behind where it is now, every sample is new
			// oceanic crust.
			wake, old := 0, 0
			for i, p := range w.Mesh.Points {
				east := math.Atan2(p.Y, p.X)
				if math.Abs(p.Z) > 0.4*radius || east < 0 || east > turned-1.2*radius {
					continue
				}
				wake++
				if w.CrustType[i] != ocean || w.OceanicAge[i] >= 100 {
					old++
				}
			}
			if wake < 30 || old != 0 {
				t.Fatalf("%d of %d samples along the fragment's path are not new oceanic crust", old, wake)
			}
			// The fragment arrives whole.
			arrived := 0
			end := sphere.Vec{X: math.Cos(turned), Y: math.Sin(turned)}
			for i, p := range w.Mesh.Points {
				if sphere.Angle(p, end) < 0.6*radius {
					arrived++
					if w.CrustType[i] != land || w.Plate[i] != 1 {
						t.Fatalf("sample %d inside the fragment where it now is: plate %d, crust type %d", i, w.Plate[i], w.CrustType[i])
					}
				}
			}
			if arrived < 5 {
				t.Fatalf("%d samples inside the fragment", arrived)
			}
		})
	}
}

// The marks of a step do not depend on the number of processors.
func TestSubductionRepeatable(t *testing.T) {
	run := func() snapshot {
		w := moving(t, 6000, 3, earth())
		s := NewSubduction(w)
		steps(t, s, w, 5, 2)
		resample(t, w, 10)
		return snap(w)
	}
	if !run().equal(run()) {
		t.Fatal("two runs differ")
	}
}
