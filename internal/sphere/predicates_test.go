// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestOrientConvention(t *testing.T) {
	// x, y, z are counter-clockwise seen from outside the sphere.
	x, y, z := Vec{1, 0, 0}, Vec{0, 1, 0}, Vec{0, 0, 1}
	if got := orient(x, y, z, Vec{}); got != 1 {
		t.Errorf("orient with the centre = %d, want 1", got)
	}
	if got := orient(x, y, z, Vec{1, 1, 1}); got != -1 {
		t.Errorf("orient with an outer point = %d, want -1", got)
	}
	if got := orient(x, y, z, Vec{0.25, 0.25, 0.5}); got != 0 {
		t.Errorf("orient with a point on the plane = %d, want 0", got)
	}
	if got := orient(y, x, z, Vec{}); got != -1 {
		t.Errorf("orient of a clockwise triangle with the centre = %d, want -1", got)
	}
	if got := det3(x, y, z); got != 1 {
		t.Errorf("det3(x, y, z) = %d, want 1", got)
	}
	if got := det3(y, x, z); got != -1 {
		t.Errorf("det3(y, x, z) = %d, want -1", got)
	}
	if got := det3(x, y, Vec{0.6, 0.8, 0}); got != 0 {
		t.Errorf("det3 of three points on a great circle = %d, want 0", got)
	}
	if got := det3(x, y, x); got != 0 {
		t.Errorf("det3 with a repeated row = %d, want 0", got)
	}
}

// Points displaced from a plane by amounts far below the rounding error
// of the floating point determinant must still get the right sign.
func TestOrientNearlyDegenerate(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 11))
	for range 2000 {
		a, b, c := randUnit(rng), randUnit(rng), randUnit(rng)
		// d = a + s(b−a) + u(c−a) is on the plane only up to rounding;
		// whatever it is, the filtered and exact answers must agree.
		s, u := rng.Float64(), rng.Float64()
		d := a.Add(b.Sub(a).Scale(s)).Add(c.Sub(a).Scale(u))
		for _, q := range []Vec{
			d,
			{math.Nextafter(d.X, 2), d.Y, d.Z},
			{d.X, math.Nextafter(d.Y, -2), d.Z},
		} {
			if got, want := orient(a, b, c, q), orientExact(a, b, c, q); got != want {
				t.Fatalf("orient = %d, exact = %d", got, want)
			}
		}
		// A point on the great circle through a and b, up to rounding.
		g := a.Scale(s).Add(b.Scale(u))
		if got, want := det3(a, b, g), orientExact(a, b, g, Vec{}); got != want {
			t.Fatalf("det3 = %d, exact = %d", got, want)
		}
	}
}

func TestOrientExactTinyOffsets(t *testing.T) {
	a, b, c := Vec{1, 0, 0}, Vec{0, 1, 0}, Vec{0, 0, 0}
	for _, tc := range []struct {
		z    float64
		want int
	}{
		{1e-300, -1}, {-1e-300, 1}, {5e-324, -1}, {-5e-324, 1}, {0, 0},
	} {
		if got := orient(a, b, c, Vec{0.25, 0.25, tc.z}); got != tc.want {
			t.Errorf("orient with z = %g: got %d, want %d", tc.z, got, tc.want)
		}
	}
}

func TestOrientAgreesWithExactOnRandomInput(t *testing.T) {
	rng := rand.New(rand.NewPCG(12, 12))
	for range 2_000 {
		a, b, c, d := randUnit(rng), randUnit(rng), randUnit(rng), randUnit(rng)
		if got, want := orient(a, b, c, d), orientExact(a, b, c, d); got != want {
			t.Fatalf("orient = %d, exact = %d", got, want)
		}
		if got, want := det3(a, b, c), orientExact(a, b, c, Vec{}); got != want {
			t.Fatalf("det3 = %d, exact = %d", got, want)
		}
	}
}
