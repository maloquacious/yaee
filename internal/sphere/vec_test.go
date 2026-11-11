// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/rand/v2"
	"testing"
)

// randUnit returns a uniformly distributed unit vector.
func randUnit(rng *rand.Rand) Vec {
	for {
		v := Vec{rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()}
		if n := v.Len(); n > 1e-6 {
			return v.Scale(1 / n)
		}
	}
}

// perpendicular returns a unit vector perpendicular to a.
func perpendicular(a Vec, rng *rand.Rand) Vec {
	for {
		t := randUnit(rng).Tangent(a)
		if n := t.Len(); n > 0.1 {
			return t.Scale(1 / n)
		}
	}
}

func TestVecArithmetic(t *testing.T) {
	a, b := Vec{1, 2, 3}, Vec{-4, 5, 0.5}
	if got, want := a.Add(b), (Vec{-3, 7, 3.5}); got != want {
		t.Errorf("Add = %v, want %v", got, want)
	}
	if got, want := a.Sub(b), (Vec{5, -3, 2.5}); got != want {
		t.Errorf("Sub = %v, want %v", got, want)
	}
	if got, want := a.Scale(2), (Vec{2, 4, 6}); got != want {
		t.Errorf("Scale = %v, want %v", got, want)
	}
	if got, want := a.Dot(b), 7.5; got != want {
		t.Errorf("Dot = %v, want %v", got, want)
	}
	if got, want := a.Cross(b), (Vec{-14, -12.5, 13}); got != want {
		t.Errorf("Cross = %v, want %v", got, want)
	}
	if got, want := (Vec{3, 4, 12}).Len(), 13.0; got != want {
		t.Errorf("Len = %v, want %v", got, want)
	}
	if got := (Vec{3, 4, 12}).Normalize().Len(); math.Abs(got-1) > 1e-15 {
		t.Errorf("Normalize length = %v, want 1", got)
	}
	if got := (Vec{}).Normalize(); got != (Vec{}) {
		t.Errorf("Normalize of zero = %v, want zero", got)
	}
	if got, want := (Vec{1, 0, 0}).Cross(Vec{0, 1, 0}), (Vec{0, 0, 1}); got != want {
		t.Errorf("x × y = %v, want %v", got, want)
	}
}

func TestAngleAccuracy(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 1))
	angles := []float64{1e-6, 1e-3, 0.004, 1, math.Pi / 2, 3, math.Pi - 1e-3, math.Pi - 1e-6}
	for _, want := range angles {
		for range 100 {
			a := randUnit(rng)
			u := perpendicular(a, rng)
			s, c := math.Sincos(want)
			b := a.Scale(c).Add(u.Scale(s))
			got := Angle(a, b)
			if rel := math.Abs(got-want) / want; rel > 1e-9 {
				t.Fatalf("Angle = %.17g, want %.17g (relative error %.3g)", got, want, rel)
			}
		}
	}
	a := Vec{0, 0, 1}
	if got := Angle(a, a); got != 0 {
		t.Errorf("Angle(a, a) = %v, want 0", got)
	}
	if got := Angle(a, a.Scale(-1)); got != math.Pi {
		t.Errorf("Angle(a, -a) = %v, want π", got)
	}
	if got := Angle(a.Scale(3), Vec{5, 0, 0}); got != math.Pi/2 {
		t.Errorf("Angle of scaled perpendicular vectors = %v, want π/2", got)
	}
}

func TestTangentIsPerpendicular(t *testing.T) {
	rng := rand.New(rand.NewPCG(2, 2))
	for range 1000 {
		p := randUnit(rng)
		v := randUnit(rng).Scale(1 + 100*rng.Float64())
		tan := v.Tangent(p)
		if d := tan.Dot(p); math.Abs(d) > 1e-12 {
			t.Fatalf("tangent · p = %g, want 0", d)
		}
		// What was removed is parallel to p.
		if c := v.Sub(tan).Cross(p).Len(); c > 1e-12 {
			t.Fatalf("removed part is not parallel to p: |cross| = %g", c)
		}
	}
}

func TestRotationKeepsLength(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 3))
	for range 1000 {
		r := NewRotation(randUnit(rng), (rng.Float64()*4-2)*math.Pi)
		v := randUnit(rng).Scale(0.1 + 10*rng.Float64())
		got, want := r.Apply(v).Len(), v.Len()
		if math.Abs(got-want) > 1e-12*want {
			t.Fatalf("length after rotation = %.17g, want %.17g", got, want)
		}
	}
}

func TestRotationAngle(t *testing.T) {
	rng := rand.New(rand.NewPCG(4, 4))
	for range 1000 {
		axis := randUnit(rng)
		theta := rng.Float64() * math.Pi
		p := perpendicular(axis, rng)
		q := NewRotation(axis, theta).Apply(p)
		if got := Angle(p, q); math.Abs(got-theta) > 1e-12 {
			t.Fatalf("moved through %.17g, want %.17g", got, theta)
		}
		if d := q.Dot(axis); math.Abs(d) > 1e-12 {
			t.Fatalf("rotated point left the plane perpendicular to the axis: %g", d)
		}
	}
}

func TestRotationDirection(t *testing.T) {
	// The right-hand rule: a quarter turn about z takes x to y.
	got := NewRotation(Vec{0, 0, 1}, math.Pi/2).Apply(Vec{1, 0, 0})
	if got.Sub(Vec{0, 1, 0}).Len() > 1e-15 {
		t.Errorf("quarter turn of x about z = %v, want (0, 1, 0)", got)
	}
	// A point on the axis does not move.
	axis := Vec{0.6, 0, 0.8}
	if got := NewRotation(axis, 1.234).Apply(axis); got.Sub(axis).Len() > 1e-15 {
		t.Errorf("point on the axis moved to %v", got)
	}
}

func TestRotationInverse(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 5))
	for range 1000 {
		r := NewRotation(randUnit(rng), (rng.Float64()*4-2)*math.Pi)
		v := randUnit(rng)
		if d := r.Inverse().Apply(r.Apply(v)).Sub(v).Len(); d > 1e-12 {
			t.Fatalf("inverse missed by %g", d)
		}
		if d := r.Then(r.Inverse()).Apply(v).Sub(v).Len(); d > 1e-12 {
			t.Fatalf("r.Then(r.Inverse()) moved a vector by %g", d)
		}
	}
}

func TestRotationThen(t *testing.T) {
	rng := rand.New(rand.NewPCG(6, 6))
	for range 1000 {
		a := NewRotation(randUnit(rng), (rng.Float64()*4-2)*math.Pi)
		b := NewRotation(randUnit(rng), (rng.Float64()*4-2)*math.Pi)
		v := randUnit(rng)
		got, want := a.Then(b).Apply(v), b.Apply(a.Apply(v))
		if d := got.Sub(want).Len(); d > 1e-12 {
			t.Fatalf("a.Then(b) differs from a, then b, by %g", d)
		}
	}
	// Order matters: x then z is not z then x.
	x := NewRotation(Vec{1, 0, 0}, math.Pi/2)
	z := NewRotation(Vec{0, 0, 1}, math.Pi/2)
	v := Vec{1, 0, 0}
	if x.Then(z).Apply(v).Sub(z.Then(x).Apply(v)).Len() < 0.5 {
		t.Error("x.Then(z) and z.Then(x) agree, but should not")
	}
}

func TestRotationCompositionStaysUnit(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 7))
	r := Identity()
	v := randUnit(rng)
	for range 10_000 {
		r = r.Then(NewRotation(randUnit(rng), rng.Float64()))
	}
	if got := r.Apply(v).Len(); math.Abs(got-1) > 1e-12 {
		t.Errorf("length after 10,000 compositions = %.17g, want 1", got)
	}
}

func TestIdentity(t *testing.T) {
	v := Vec{0.1, -0.2, 0.3}
	if got := Identity().Apply(v); got != v {
		t.Errorf("Identity().Apply(%v) = %v", v, got)
	}
}
