// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// randomPoint draws a point uniformly from the sphere.
func randomPoint(rng *rand.Rand) sphere.Vec {
	z := 1 - 2*rng.Float64()
	r := math.Sqrt(1 - z*z)
	sin, cos := math.Sincos(2 * math.Pi * rng.Float64())
	return sphere.Vec{X: r * cos, Y: r * sin, Z: z}
}

// moving returns a world of n samples with plates, crust, and motion.
func moving(t testing.TB, n int, seed uint64, params model.Params) *model.World {
	t.Helper()
	w := newWorld(t, n, params)
	run(t, w, seed, Plates{}, Crust{}, Motion{})
	return w
}

func TestMotionInitial(t *testing.T) {
	var fastest, slowest float64 = 0, math.Inf(1)
	for seed := range uint64(5) {
		w := moving(t, 5000, seed, earth())
		if len(w.PlateAxis) != w.NumPlates || len(w.PlateSpeed) != w.NumPlates || len(w.PlateRotation) != w.NumPlates {
			t.Fatalf("%d axes, %d speeds, %d rotations for %d plates",
				len(w.PlateAxis), len(w.PlateSpeed), len(w.PlateRotation), w.NumPlates)
		}
		top := make([]float64, w.NumPlates) // speed of each plate's fastest sample
		for i, p := range w.Mesh.Points {
			k := int(w.Plate[i])
			v := Velocity(w, k, p)
			// No sample of any plate moves faster than v0.
			if v.Len() > w.MaxPlateSpeed*(1+1e-12) {
				t.Fatalf("seed %d: sample %d of plate %d moves at %v km/My, above %v", seed, i, k, v.Len(), w.MaxPlateSpeed)
			}
			top[k] = max(top[k], v.Len())
		}
		for k := range w.NumPlates {
			if l := w.PlateAxis[k].Len(); math.Abs(l-1) > 1e-12 {
				t.Fatalf("seed %d: plate %d's axis has length %v", seed, k, l)
			}
			if !(w.PlateSpeed[k] >= 0) || math.IsInf(w.PlateSpeed[k], 0) {
				t.Fatalf("seed %d: plate %d has angular speed %v", seed, k, w.PlateSpeed[k])
			}
			if w.PlateRotation[k] != sphere.Identity() {
				t.Fatalf("seed %d: plate %d starts with rotation %v", seed, k, w.PlateRotation[k])
			}
			fastest, slowest = max(fastest, top[k]), min(slowest, top[k])
		}
		if got := maxSpeed(w); got != slices.Max(top) && math.Abs(got-slices.Max(top)) > 1e-9 {
			t.Errorf("seed %d: maxSpeed = %v, want %v", seed, got, slices.Max(top))
		}
	}
	// 200 speeds drawn uniformly from 0 to 100 km/My.
	if fastest < 95 || slowest > 5 {
		t.Errorf("fastest samples of 200 plates move at %v to %v km/My; want nearly 0 to 100", slowest, fastest)
	}
}

func TestMotionRepeatable(t *testing.T) {
	a, b, c := moving(t, 3000, 3, earth()), moving(t, 3000, 3, earth()), moving(t, 3000, 4, earth())
	if !slices.Equal(a.PlateAxis, b.PlateAxis) || !slices.Equal(a.PlateSpeed, b.PlateSpeed) {
		t.Error("the same seed gave different motion")
	}
	if slices.Equal(a.PlateAxis, c.PlateAxis) {
		t.Error("different seeds gave the same axes")
	}
	// Axes do not depend on the speed limit; speeds scale with it.
	params := earth()
	params.MaxPlateSpeed = 50
	d := moving(t, 3000, 3, params)
	if !slices.Equal(a.PlateAxis, d.PlateAxis) {
		t.Error("the speed limit changed the axes")
	}
	for k := range a.PlateSpeed {
		if math.Abs(d.PlateSpeed[k]-a.PlateSpeed[k]/2) > 1e-15 {
			t.Fatalf("plate %d: speed %v at half the limit, want %v", k, d.PlateSpeed[k], a.PlateSpeed[k]/2)
		}
	}
}

// With every speed zero, no sample's position changes.
func TestAdvanceAtRest(t *testing.T) {
	params := earth()
	params.MaxPlateSpeed = 0
	w := moving(t, 3000, 1, params)
	for range 50 {
		Advance(w, 2)
	}
	for i, p := range w.Mesh.Points {
		if got := Position(w, i); got != p {
			t.Fatalf("sample %d moved from %v to %v", i, p, got)
		}
	}
	for k, r := range w.PlateRotation {
		if r != sphere.Identity() {
			t.Fatalf("plate %d's rotation is %v", k, r)
		}
	}
}

// After time t, a sample's position is its start position rotated by ωt
// about the plate's axis.
func TestAdvance(t *testing.T) {
	w := moving(t, 3000, 2, earth())
	var elapsed float64
	for _, dt := range []float64{2, 2, 0.5, 7, 2, 1e-3, 30} {
		Advance(w, dt)
		elapsed += dt
	}
	var furthest float64
	for i, p := range w.Mesh.Points {
		k := w.Plate[i]
		want := sphere.NewRotation(w.PlateAxis[k], w.PlateSpeed[k]*elapsed).Apply(p)
		got := Position(w, i)
		if d := got.Sub(want).Len(); d > 1e-13 {
			t.Fatalf("sample %d is at %v after %v My, want %v (off by %v)", i, got, elapsed, want, d)
		}
		if l := got.Len(); math.Abs(l-1) > 1e-13 {
			t.Fatalf("sample %d left the sphere: length %v", i, l)
		}
		// The distance moved is no more than v0 · t, along the surface.
		moved := sphere.Angle(p, got) * w.Radius
		if moved > w.MaxPlateSpeed*elapsed*(1+1e-9) {
			t.Fatalf("sample %d moved %v km in %v My", i, moved, elapsed)
		}
		furthest = max(furthest, moved)
	}
	if furthest < 0.5*w.MaxPlateSpeed*elapsed {
		t.Errorf("the furthest sample moved %v km in %v My; the test needs plates that move", furthest, elapsed)
	}
}

// The surface velocity is tangent to the sphere, has magnitude
// ωR sin(angle from the axis), and is the rate at which Advance moves a
// point.
func TestVelocity(t *testing.T) {
	w := moving(t, 2000, 5, earth())
	rng := rand.New(rand.NewPCG(1, 2))
	for range 2000 {
		k := rng.IntN(w.NumPlates)
		p := randomPoint(rng)
		v := Velocity(w, k, p)
		if d := v.Dot(p); math.Abs(d) > 1e-12*w.MaxPlateSpeed {
			t.Fatalf("plate %d at %v: velocity %v is not tangent (v·p = %v)", k, p, v, d)
		}
		want := w.PlateSpeed[k] * w.Radius * math.Sin(sphere.Angle(p, w.PlateAxis[k]))
		if math.Abs(v.Len()-want) > 1e-9 {
			t.Fatalf("plate %d at %v: speed %v, want ωR sin θ = %v", k, p, v.Len(), want)
		}
		// Finite difference: turn p through a short time.
		const dt = 1e-5
		moved := sphere.NewRotation(w.PlateAxis[k], w.PlateSpeed[k]*dt).Apply(p)
		rate := moved.Sub(p).Scale(w.Radius / dt)
		if rate.Sub(v).Len() > 1e-3 {
			t.Fatalf("plate %d at %v: velocity %v, but the point moves at %v", k, p, v, rate)
		}
	}
	// On the axis nothing moves.
	if v := Velocity(w, 0, w.PlateAxis[0]); v.Len() > 1e-12 {
		t.Errorf("velocity on the axis is %v", v)
	}
}

// Before anything has moved, every point is held by exactly one plate,
// and a sample is held by its own plate with weight 1.
func TestQueryAtRest(t *testing.T) {
	w := moving(t, 3000, 6, earth())
	rng := rand.New(rand.NewPCG(3, 4))
	var mixed int
	for range 3000 {
		q := randomPoint(rng)
		var holders int
		for k := range w.NumPlates {
			tri, weights, ok := Query(w, k, q)
			if !ok {
				if weights != ([3]float64{}) {
					t.Fatalf("plate %d does not hold %v but returned weights %v", k, q, weights)
				}
				continue
			}
			holders++
			corners := w.Mesh.Tri[3*tri : 3*tri+3]
			var sum float64
			foreign := false
			for c, s := range corners {
				if weights[c] < 0 || weights[c] > 1 {
					t.Fatalf("weight %v out of range", weights[c])
				}
				if int(w.Plate[s]) != k {
					foreign = true
					if weights[c] != 0 {
						t.Fatalf("plate %d at %v: corner %d of plate %d has weight %v", k, q, s, w.Plate[s], weights[c])
					}
				}
				sum += weights[c]
			}
			if math.Abs(sum-1) > 1e-12 {
				t.Fatalf("plate %d at %v: weights %v sum to %v", k, q, weights, sum)
			}
			if foreign {
				mixed++
			}
			// The triangle is the one that contains q, and the plate
			// holds q because q's nearest corner is its own.
			wantTri, bary := w.Locator.Locate(q)
			if tri != wantTri {
				t.Fatalf("Query found triangle %d at %v, Locate %d", tri, q, wantTri)
			}
			owner := 0
			for c := 1; c < 3; c++ {
				if bary[c] > bary[owner] {
					owner = c
				}
			}
			if int(w.Plate[corners[owner]]) != k {
				t.Fatalf("plate %d holds %v, whose nearest corner belongs to plate %d", k, q, w.Plate[corners[owner]])
			}
		}
		if holders != 1 {
			t.Fatalf("%d plates hold %v, want 1", holders, q)
		}
	}
	if mixed < 100 {
		t.Errorf("only %d of 3000 points fell in a triangle shared between plates; the test needs some", mixed)
	}

	for i, p := range w.Mesh.Points {
		for k := range w.NumPlates {
			tri, weights, ok := Query(w, k, p)
			if ok != (int(w.Plate[i]) == k) {
				t.Fatalf("Query(plate %d, sample %d of plate %d) = %v", k, i, w.Plate[i], ok)
			}
			if !ok {
				continue
			}
			for c, s := range w.Mesh.Tri[3*tri : 3*tri+3] {
				if want := map[bool]float64{true: 1, false: 0}[int(s) == i]; weights[c] != want {
					t.Fatalf("sample %d: weights %v over corners %v, want exactly 1 at the sample", i, weights, w.Mesh.Tri[3*tri:3*tri+3])
				}
			}
		}
	}
}

// After a plate has moved, the query at a sample's new position returns
// a triangle with that sample as a corner.
func TestQueryFollowsThePlate(t *testing.T) {
	w := moving(t, 4000, 7, earth())
	for range 25 {
		Advance(w, 2) // 50 My: up to 5000 km
	}
	for i := range w.Mesh.Points {
		k := int(w.Plate[i])
		tri, weights, ok := Query(w, k, Position(w, i))
		if !ok {
			t.Fatalf("plate %d does not hold its sample %d at its new position", k, i)
		}
		corner := slices.Index(w.Mesh.Tri[3*tri:3*tri+3], int32(i))
		if corner < 0 {
			t.Fatalf("sample %d: triangle %d has corners %v", i, tri, w.Mesh.Tri[3*tri:3*tri+3])
		}
		if weights[corner] < 1-1e-9 {
			t.Fatalf("sample %d: weights %v, want 1 at corner %d", i, weights, corner)
		}
	}
}

// A plate that has moved away from a point no longer holds it, and
// holds the points it has moved onto.
func TestQueryNotCovered(t *testing.T) {
	params := earth()
	params.Plates = 2
	w := newWorld(t, 4000, params)
	// Plate 0 is the cap within 30° of the north pole; plate 1 the rest.
	w.NumPlates = 2
	cos30 := math.Cos(math.Pi / 6)
	for i, p := range w.Mesh.Points {
		w.Plate[i] = 1
		if p.Z > cos30 {
			w.Plate[i] = 0
		}
	}
	// Plate 0 turns a quarter turn about the x axis, which carries the
	// pole to (0, −1, 0). Plate 1 stays.
	w.PlateAxis = []sphere.Vec{{X: 1}, {Z: 1}}
	w.PlateSpeed = []float64{math.Pi / 2 / 10, 0}
	w.PlateRotation = []sphere.Rotation{sphere.Identity(), sphere.Identity()}
	north, moved := sphere.Vec{Z: 1}, sphere.Vec{Y: -1}

	if _, _, ok := Query(w, 0, north); !ok {
		t.Fatal("plate 0 does not hold the north pole before it moves")
	}
	for range 5 {
		Advance(w, 2)
	}
	if got := w.PlateRotation[0].Apply(north); got.Sub(moved).Len() > 1e-12 {
		t.Fatalf("the pole moved to %v, want %v", got, moved)
	}
	if _, _, ok := Query(w, 0, north); ok {
		t.Error("plate 0 still holds the north pole after moving away")
	}
	if _, _, ok := Query(w, 0, moved); !ok {
		t.Error("plate 0 does not hold the point its centre moved to")
	}
	// Plate 1 has not moved: it holds neither its hole at the pole nor
	// lost the ground plate 0 now lies over.
	if _, _, ok := Query(w, 1, north); ok {
		t.Error("plate 1 holds the north pole, which it never did")
	}
	if _, _, ok := Query(w, 1, moved); !ok {
		t.Error("plate 1 lost the point plate 0 moved over")
	}
	// Plate 0 holds exactly the cap of 30° about its new centre, to
	// within a sample spacing.
	spacing := sphere.SampleSpacing(1, len(w.Mesh.Points)) // radians
	rng := rand.New(rand.NewPCG(5, 6))
	for range 5000 {
		q := randomPoint(rng)
		angle := sphere.Angle(q, moved)
		_, _, ok := Query(w, 0, q)
		if math.Abs(angle-math.Pi/6) > spacing && ok != (angle < math.Pi/6) {
			t.Fatalf("point %v from the cap's centre: held = %v", angle, ok)
		}
	}
}
