// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"runtime"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// snapshot is a copy of every per-sample slice of a world.
type snapshot struct {
	plate                  []int32
	crust, orogeny         []uint8
	thickness, elevation   []float64
	oceanicAge, orogenyAge []float64
	ridge, fold            []sphere.Vec
	axis                   []sphere.Vec
	speed                  []float64
}

func snap(w *model.World) snapshot {
	return snapshot{
		plate: slices.Clone(w.Plate), crust: slices.Clone(w.CrustType), orogeny: slices.Clone(w.OrogenyType),
		thickness: slices.Clone(w.Thickness), elevation: slices.Clone(w.Elevation),
		oceanicAge: slices.Clone(w.OceanicAge), orogenyAge: slices.Clone(w.OrogenyAge),
		ridge: slices.Clone(w.RidgeDirection), fold: slices.Clone(w.FoldDirection),
		axis: slices.Clone(w.PlateAxis), speed: slices.Clone(w.PlateSpeed),
	}
}

func (a snapshot) equal(b snapshot) bool {
	return slices.Equal(a.plate, b.plate) && slices.Equal(a.crust, b.crust) && slices.Equal(a.orogeny, b.orogeny) &&
		slices.Equal(a.thickness, b.thickness) && slices.Equal(a.elevation, b.elevation) &&
		slices.Equal(a.oceanicAge, b.oceanicAge) && slices.Equal(a.orogenyAge, b.orogenyAge) &&
		slices.Equal(a.ridge, b.ridge) && slices.Equal(a.fold, b.fold) &&
		slices.Equal(a.axis, b.axis) && slices.Equal(a.speed, b.speed)
}

// decorate gives every attribute a value that varies from sample to
// sample, within its documented range, so that a resampling that mixed
// samples up would show.
func decorate(w *model.World) {
	for i, p := range w.Mesh.Points {
		w.Thickness[i] = 20 + 10*p.X
		w.Elevation[i] = 3*p.Y - 2*p.Z*p.X
		w.FoldDirection[i] = sphere.Vec{X: 0.3, Y: -2, Z: 1}.Cross(p)
		w.OrogenyType[i] = uint8(i % 3)
		w.OrogenyAge[i] = 0
		if w.OrogenyType[i] != model.OrogenyNone {
			w.OrogenyAge[i] = 50 + 40*p.Z
		}
		w.OceanicAge[i], w.RidgeDirection[i] = 0, sphere.Vec{}
		if w.CrustType[i] == model.Oceanic {
			w.OceanicAge[i] = 100 + 90*p.Y
			if i%5 != 0 {
				w.RidgeDirection[i] = sphere.Vec{X: 1, Y: 1, Z: -0.5}.Cross(p).Normalize()
			}
		}
	}
}

func resample(t testing.TB, w *model.World, elapsed float64) {
	t.Helper()
	if err := Resample(t.Context(), w, elapsed); err != nil {
		t.Fatalf("Resample: %v", err)
	}
}

// With every speed zero, resampling leaves every attribute of every
// sample unchanged.
func TestResampleAtRest(t *testing.T) {
	params := earth()
	params.MaxPlateSpeed = 0
	w := moving(t, 6000, 3, params)
	decorate(w)
	before := snap(w)
	for range 3 {
		Advance(w, 2)
		resample(t, w, 2)
		checkCrust(t, w)
		if !snap(w).equal(before) {
			t.Fatal("resampling changed a world in which nothing moves")
		}
	}

	// The same holds when plates have speeds but no time has passed,
	// and through the whole simulation stage.
	w = moving(t, 6000, 3, earth())
	decorate(w)
	before = snap(w)
	resample(t, w, 2)
	if !snap(w).equal(before) {
		t.Fatal("resampling changed a world before any plate had moved")
	}
	// The simulation then only ages the ocean, erodes the land, and ages
	// the orogeny.
	w = moving(t, 6000, 3, params)
	decorate(w)
	aged := moving(t, 6000, 3, params)
	decorate(aged)
	for range 125 {
		if err := AgeOcean(t.Context(), aged, 2); err != nil {
			t.Fatal(err)
		}
		if err := Erode(t.Context(), aged, 2); err != nil {
			t.Fatal(err)
		}
		for i, kind := range aged.OrogenyType {
			if kind != model.OrogenyNone {
				aged.OrogenyAge[i] += 2
			}
		}
	}
	if n, err := simulate(t.Context(), w, nil, 500); err != nil || n != 0 {
		t.Fatalf("simulate: %d resamplings, error %v; want none", n, err)
	}
	if !snap(w).equal(snap(aged)) {
		t.Fatal("the simulation did more than age and erode a world in which nothing moves")
	}
	if snap(w).equal(before) {
		t.Fatal("the simulation did not age the ocean")
	}
}

// onePlate returns a world of n samples that is a single plate turning
// about axis at the given angular speed in radians per My.
func onePlate(t testing.TB, n int, axis sphere.Vec, speed float64) *model.World {
	t.Helper()
	params := earth()
	params.Plates = 1
	params.MaxPlateSpeed = math.Inf(1) // the tests set speeds directly
	w := newWorld(t, n, params)
	w.NumPlates = 1
	w.PlateAxis = []sphere.Vec{axis.Normalize()}
	w.PlateSpeed = []float64{speed}
	w.PlateRotation = []sphere.Rotation{sphere.Identity()}
	return w
}

// The analytic fields carried round the sphere: a scalar with features a
// quarter of the sphere across, and a tangent vector field.
func scalarField(p sphere.Vec) float64 { return 2*p.X - p.Y*p.Z + 1.5*p.X*p.Y + p.Z*p.Z }

var fieldAxis = sphere.Vec{X: 0.2, Y: -0.5, Z: 1}

func vectorField(p sphere.Vec) sphere.Vec { return fieldAxis.Cross(p).Scale(1 + 0.5*p.X) }

// carry turns a single plate holding the analytic fields through 120°
// in 8 resamplings, and returns the greatest error in the scalar field,
// in the vector field, and in the tangency of the vectors.
func carry(t testing.TB, n int) (scalarErr, vectorErr, tangentErr float64) {
	t.Helper()
	axis := sphere.Vec{X: 1, Y: 2, Z: 0.5}.Normalize()
	const (
		turns = 8
		angle = 120 * math.Pi / 180
		dt    = 2.0
	)
	w := onePlate(t, n, axis, angle/turns/dt)
	for i, p := range w.Mesh.Points {
		w.Elevation[i] = scalarField(p)
		w.Thickness[i] = 35
		w.CrustType[i] = model.Continental
		w.FoldDirection[i] = vectorField(p)
	}
	for range turns {
		Advance(w, dt)
		resample(t, w, 2)
	}
	// What is at q now was at q turned back by the whole rotation, and
	// its vectors have turned with the plate.
	total := sphere.NewRotation(axis, angle)
	for i, q := range w.Mesh.Points {
		from := total.Inverse().Apply(q)
		scalarErr = max(scalarErr, math.Abs(w.Elevation[i]-scalarField(from)))
		want := total.Apply(vectorField(from))
		vectorErr = max(vectorErr, w.FoldDirection[i].Sub(want).Len())
		tangentErr = max(tangentErr, math.Abs(w.FoldDirection[i].Dot(q)))
		if w.Plate[i] != 0 || w.CrustType[i] != model.Continental || math.Abs(w.Thickness[i]-35) > 1e-9 {
			t.Fatalf("sample %d: plate %d, crust type %d, thickness %v", i, w.Plate[i], w.CrustType[i], w.Thickness[i])
		}
	}
	if w.PlateRotation[0] != sphere.Identity() {
		t.Fatal("resampling did not reset the plate's rotation")
	}
	return scalarErr, vectorErr, tangentErr
}

// One plate covering the sphere, carrying analytic fields, is turned
// through 120° across 8 resamplings.
//
// Interpolation is linear, so each resampling adds an error of about
// (second derivative of the field) · h²/8 for a sample spacing of h
// radians, and the errors of successive resamplings add up. For N
// samples h² is 8π/(√3·N), so the error is proportional to the number of
// resamplings and to 1/N: four times the samples give a quarter of the
// error. The bound below is resamplings · h² for both fields, whose
// second derivatives are about 6.
func TestResampleCarriesFields(t *testing.T) {
	const n = 4000
	h2 := 8 * math.Pi / (math.Sqrt(3) * n)
	bound := 8 * h2
	scalarErr, vectorErr, tangentErr := carry(t, n)
	t.Logf("%d samples: scalar error %.4g, vector error %.4g (bound %.4g)", n, scalarErr, vectorErr, bound)
	if scalarErr > bound {
		t.Errorf("scalar field is off by %v, want at most %v", scalarErr, bound)
	}
	if vectorErr > bound {
		t.Errorf("vector field is off by %v, want at most %v", vectorErr, bound)
	}
	if tangentErr > 1e-12 {
		t.Errorf("a vector is off the tangent plane by %v", tangentErr)
	}
	// The fields must not simply have stayed where they were: the
	// values are of order 1, and the plate turned 120°.
	if scalarErr < 1e-9 && vectorErr < 1e-9 {
		t.Error("no error at all: the test did not interpolate")
	}

	// A quarter of the error at four times the samples.
	scalar4, vector4, _ := carry(t, 4*n)
	t.Logf("%d samples: scalar error %.4g, vector error %.4g", 4*n, scalar4, vector4)
	for _, r := range []struct {
		name         string
		coarse, fine float64
	}{{"scalar", scalarErr, scalar4}, {"vector", vectorErr, vector4}} {
		if ratio := r.coarse / r.fine; ratio < 3 || ratio > 5.5 {
			t.Errorf("%s error falls by a factor of %v with four times the samples; want about 4", r.name, ratio)
		}
	}
}

// Ridge directions are carried as unit tangent vectors.
func TestResampleRidgeDirection(t *testing.T) {
	axis := sphere.Vec{X: -1, Y: 0.3, Z: 2}.Normalize()
	const angle = 100 * math.Pi / 180
	w := onePlate(t, 4000, axis, angle/5/2)
	field := func(p sphere.Vec) sphere.Vec { return fieldAxis.Cross(p).Normalize() }
	for i, p := range w.Mesh.Points {
		w.Thickness[i] = 7
		w.Elevation[i] = -6
		w.OceanicAge[i] = 100 + 50*p.Z
		w.RidgeDirection[i] = field(p)
	}
	for range 5 {
		Advance(w, 2)
		resample(t, w, 2)
	}
	checkCrust(t, w)
	total := sphere.NewRotation(axis, angle)
	poleA, poleB := total.Apply(fieldAxis.Normalize()), total.Apply(fieldAxis.Normalize().Scale(-1))
	for i, q := range w.Mesh.Points {
		// The field has no direction at its two poles.
		if sphere.Angle(q, poleA) < 0.3 || sphere.Angle(q, poleB) < 0.3 {
			continue
		}
		want := total.Apply(field(total.Inverse().Apply(q)))
		if d := w.RidgeDirection[i].Sub(want).Len(); d > 0.02 {
			t.Fatalf("sample %d: ridge direction %v, want %v (off by %v)", i, w.RidgeDirection[i], want, d)
		}
		if got := w.OceanicAge[i]; math.Abs(got-(100+50*total.Inverse().Apply(q).Z)) > 0.5 {
			t.Fatalf("sample %d: oceanic age %v", i, got)
		}
	}
}

// After every resampling every sample belongs to exactly one plate, the
// crust is valid, no category takes a value its triangle's corners did
// not hold, and no sample moves faster than v0.
func TestResampleMovingPlates(t *testing.T) {
	for seed := range uint64(3) {
		w := moving(t, 6000, seed, earth())
		decorate(w)
		var gaps, overlaps int
		for round := range 6 {
			for range 3 {
				Advance(w, 2)
			}
			before := snap(w)
			// Which plates hold each sample's position, by Query.
			type hit struct {
				plate   int
				corners []int32
				weights [3]float64
			}
			holders := make([][]hit, len(w.Mesh.Points))
			for i, q := range w.Mesh.Points {
				for k := range w.NumPlates {
					if tri, weights, ok := Query(w, k, q); ok {
						holders[i] = append(holders[i], hit{k, w.Mesh.Tri[3*tri : 3*tri+3], weights})
					}
				}
			}

			resample(t, w, 6)
			checkCrust(t, w)
			held := make([]bool, w.NumPlates) // plates that hold a sample's position
			for _, hs := range holders {
				for _, h := range hs {
					held[h.plate] = true
				}
			}
			for i := range w.Mesh.Points {
				k := w.Plate[i]
				if k < 0 || int(k) >= w.NumPlates {
					t.Fatalf("seed %d, round %d: sample %d has plate %d", seed, round, i, k)
				}
				switch len(holders[i]) {
				case 0:
					gaps++
					// New oceanic crust, no older than the gap and not
					// above sea level, in a plate that holds something.
					if w.CrustType[i] != model.Oceanic || w.Thickness[i] != 7 || w.OrogenyType[i] != model.OrogenyNone ||
						w.OceanicAge[i] > 6 || w.Elevation[i] > 0 || w.FoldDirection[i] != (sphere.Vec{}) {
						t.Fatalf("seed %d, round %d: sample %d in a gap did not get new oceanic crust: age %v, elevation %v",
							seed, round, i, w.OceanicAge[i], w.Elevation[i])
					}
					if !held[k] {
						t.Fatalf("seed %d, round %d: gap sample %d has plate %d, which holds no sample", seed, round, i, k)
					}
					continue
				case 1:
				default:
					overlaps++
				}
				at := slices.IndexFunc(holders[i], func(h hit) bool { return h.plate == int(k) })
				if at < 0 {
					t.Fatalf("seed %d, round %d: sample %d went to plate %d, which does not hold it", seed, round, i, k)
				}
				h := holders[i][at]
				var crust, orogeny []uint8
				lo, hi := math.Inf(1), math.Inf(-1)
				for c, s := range h.corners {
					if h.weights[c] > 0 {
						crust, orogeny = append(crust, before.crust[s]), append(orogeny, before.orogeny[s])
						lo, hi = min(lo, before.elevation[s]), max(hi, before.elevation[s])
					}
				}
				if !slices.Contains(crust, w.CrustType[i]) || !slices.Contains(orogeny, w.OrogenyType[i]) {
					t.Fatalf("seed %d, round %d: sample %d has crust type %d and orogeny type %d; its corners held %v and %v",
						seed, round, i, w.CrustType[i], w.OrogenyType[i], crust, orogeny)
				}
				if z := w.Elevation[i]; z < lo-1e-12 || z > hi+1e-12 {
					t.Fatalf("seed %d, round %d: sample %d has elevation %v, outside its corners' %v to %v", seed, round, i, z, lo, hi)
				}
				// Continental crust wins an overlap.
				if w.CrustType[i] == model.Oceanic {
					for _, o := range holders[i] {
						nearest := 0
						for c := 1; c < 3; c++ {
							if o.weights[c] > o.weights[nearest] {
								nearest = c
							}
						}
						if o.weights[nearest] > 0.5 && before.crust[o.corners[nearest]] == model.Continental {
							t.Fatalf("seed %d, round %d: sample %d is oceanic though plate %d holds continental crust there", seed, round, i, o.plate)
						}
					}
				}
			}
			for k := range w.NumPlates {
				if w.PlateRotation[k] != sphere.Identity() {
					t.Fatalf("plate %d's rotation was not reset", k)
				}
				if w.PlateAxis[k] != before.axis[k] || w.PlateSpeed[k] > before.speed[k] {
					t.Fatalf("plate %d: axis or speed changed from %v, %v to %v, %v", k, before.axis[k], before.speed[k], w.PlateAxis[k], w.PlateSpeed[k])
				}
			}
			for i, p := range w.Mesh.Points {
				if v := Velocity(w, int(w.Plate[i]), p).Len(); v > w.MaxPlateSpeed*(1+1e-12) {
					t.Fatalf("seed %d, round %d: sample %d moves at %v km/My", seed, round, i, v)
				}
			}
		}
		if gaps < 100 || overlaps < 100 {
			t.Errorf("seed %d: %d samples in gaps and %d in overlaps; the test needs both", seed, gaps, overlaps)
		}
	}
}

// A bounding cap must contain everything its plate holds.
func TestPlateCaps(t *testing.T) {
	w := moving(t, 3000, 8, earth())
	for range 20 {
		Advance(w, 2)
	}
	centre, cosine := plateCaps(w)
	for k := range w.NumPlates {
		centre[k] = w.PlateRotation[k].Apply(centre[k])
	}
	for _, q := range sphere.FibonacciPoints(20000) {
		for k := range w.NumPlates {
			if _, _, ok := Query(w, k, q); ok && q.Dot(centre[k]) < cosine[k] {
				t.Fatalf("plate %d holds %v, outside its cap", k, q)
			}
		}
	}
	// The caps are not all the whole sphere.
	var small int
	for _, c := range cosine {
		if c > 0 {
			small++
		}
	}
	if small < w.NumPlates/2 {
		t.Errorf("only %d of %d caps are smaller than a hemisphere", small, w.NumPlates)
	}

	// A plate without samples has a cap that holds nothing.
	for i := range w.Plate {
		if w.Plate[i] == 3 {
			w.Plate[i] = 4
		}
	}
	_, cosine = plateCaps(w)
	if !math.IsInf(cosine[3], 1) {
		t.Errorf("an empty plate's cap has cosine %v, want +Inf", cosine[3])
	}
	resample(t, w, 2)
	if slices.Contains(w.Plate, 3) {
		t.Error("an empty plate gained samples")
	}
}

func TestSteps(t *testing.T) {
	for _, c := range []struct {
		duration, step float64
		want           int
	}{
		{250, 2, 125}, {0, 2, 0}, {1, 2, 1}, {2, 2, 1}, {2.0001, 2, 2}, {251, 2, 126},
		{0.3, 0.1, 3}, {0.7, 0.1, 7}, {1e-9, math.Pi, 1}, {100, 0.001, 100000}, {4.35, 0.05, 87},
	} {
		got := Steps(c.duration, c.step)
		if got != c.want {
			t.Errorf("Steps(%v, %v) = %d, want %d", c.duration, c.step, got, c.want)
		}
		if got > 0 {
			if last := c.duration - float64(got-1)*c.step; !(last > 0) || last > c.step*(1+1e-12) {
				t.Errorf("Steps(%v, %v) = %d leaves a last step of %v", c.duration, c.step, got, last)
			}
		}
	}
}

// The schedule: a resampling each time the fastest sample has moved the
// given distance, and one at the end.
func TestSimulateSchedule(t *testing.T) {
	for _, c := range []struct {
		duration, step, distance float64
		want                     int
	}{
		{250, 2, 500, 42},  // 200 km a step: steps 3, 6, ..., 123, and the last, 125
		{250, 2, 199, 125}, // every step
		{250, 2, 1e9, 1},   // only at the end
		{9, 2, 1e9, 1},
		{0, 2, 500, 0},
	} {
		// One plate whose fastest sample moves at exactly 100 km/My.
		w := onePlate(t, 500, sphere.Vec{X: 1}, 100.0/6370)
		w.MaxPlateSpeed = 1e9
		w.Duration, w.TimeStep = c.duration, c.step
		for i := range w.Elevation {
			w.Thickness[i] = 7
		}
		if v := maxSpeed(w); math.Abs(v-100) > 0.1 {
			t.Fatalf("the plate's fastest sample moves at %v km/My, want 100", v)
		}
		got, err := simulate(t.Context(), w, nil, c.distance)
		if err != nil {
			t.Fatal(err)
		}
		want := c.want
		if got != want {
			t.Errorf("%v My in steps of %v, resampling every %v km: %d resamplings, want %d", c.duration, c.step, c.distance, got, want)
		}
		if w.PlateRotation[0] != sphere.Identity() {
			t.Errorf("the simulation ended with a plate turned")
		}
	}
}

// The simulation's result does not depend on the number of processors.
func TestSimulateIndependentOfProcessors(t *testing.T) {
	make := func() snapshot {
		w := moving(t, 20000, 2, earth())
		decorate(w)
		w.Duration = 40
		if _, err := simulate(t.Context(), w, nil, 500); err != nil {
			t.Fatal(err)
		}
		return snap(w)
	}
	many := make()
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)
	if !make().equal(many) {
		t.Fatalf("the world differs between 1 processor and %d", prev)
	}
}

// Plates drift: after the simulation most samples have changed plate or
// crust, and the continental area is roughly kept.
func TestSimulateMovesPlates(t *testing.T) {
	w := moving(t, 8000, 1, earth())
	before := snap(w)
	land := landShare(w)
	run(t, w, 1, Simulation{})
	checkCrust(t, w)
	var changed int
	for i := range w.Plate {
		if w.Plate[i] != before.plate[i] || w.CrustType[i] != before.crust[i] {
			changed++
		}
	}
	if changed < len(w.Plate)/4 {
		t.Errorf("only %d of %d samples changed plate or crust type in 250 My", changed, len(w.Plate))
	}
	if got := landShare(w); got < land/2 || got > 1.5*land {
		t.Errorf("continental share went from %v to %v", land, got)
	}
}

func TestSimulateCancelled(t *testing.T) {
	w := moving(t, 5000, 1, earth())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := (Simulation{}).Run(ctx, w, nil); err != context.Canceled {
		t.Fatalf("Simulation: got %v, want context.Canceled", err)
	}
	Advance(w, 2)
	if err := Resample(ctx, w, 2); err != context.Canceled {
		t.Fatalf("Resample: got %v, want context.Canceled", err)
	}
}
