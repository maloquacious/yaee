// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"testing"
)

func TestSampleCount(t *testing.T) {
	if got, want := SampleCount(6370, 25), 942_059; got != want {
		t.Errorf("SampleCount(6370, 25) = %d, want %d", got, want)
	}
	// SampleSpacing inverts SampleCount.
	for _, d := range []float64{25, 35, 50, 100, 500} {
		n := SampleCount(6370, d)
		if got := SampleSpacing(6370, n); math.Abs(got-d) > 0.01*d {
			t.Errorf("SampleSpacing(6370, %d) = %g, want %g", n, got, d)
		}
	}
}

// The paper's §7.4 and Table 2 pair sample counts with resolutions on an
// Earth-sized planet. Our formula must agree with them to within 5%.
func TestSampleSpacingMatchesPaper(t *testing.T) {
	for _, tc := range []struct {
		n  int
		km float64
	}{
		{60_000, 102},
		{100_000, 78},
		{250_000, 50},
		{500_000, 35},
	} {
		got := SampleSpacing(6370, tc.n)
		if rel := math.Abs(got-tc.km) / tc.km; rel > 0.05 {
			t.Errorf("SampleSpacing(6370, %d) = %.2f km, paper says %g km (off by %.1f%%)", tc.n, got, tc.km, 100*rel)
		}
	}
}

func TestFibonacciPoints(t *testing.T) {
	for _, n := range []int{4, 5, 7, 10, 50, 317, 1000, 3001, 5000} {
		points := FibonacciPoints(n)
		if len(points) != n {
			t.Fatalf("n=%d: got %d points", n, len(points))
		}

		// Unit length, and the same as FibonacciPoint.
		var sum Vec
		for i, p := range points {
			if l := p.Len(); math.Abs(l-1) > 1e-12 {
				t.Fatalf("n=%d: point %d has length %.17g", n, i, l)
			}
			if q := FibonacciPoint(i, n); q != p {
				t.Fatalf("n=%d: FibonacciPoints[%d] = %v, FibonacciPoint = %v", n, i, p, q)
			}
			sum = sum.Add(p)
		}

		// The smallest angle between any two samples, as a fraction of the
		// nominal spacing. Measured: 0.894 at n = 4, falling to 0.8117 as n
		// grows (0.8117 at n = 20,011). The bound is 0.8. A positive
		// minimum also shows that no two samples coincide.
		smallest := math.Inf(1)
		for i, p := range points {
			for _, q := range points[:i] {
				smallest = min(smallest, Angle(p, q))
			}
		}
		if ratio := smallest / SampleSpacing(1, n); ratio < 0.8 {
			t.Errorf("n=%d: smallest separation is %.4f of the nominal spacing, want at least 0.8", n, ratio)
		}

		// The centroid is near the centre of the sphere. Measured: the
		// sum of all positions has length 0.485 at n = 4 and under 0.03
		// for n >= 50. The bound is a sum of length 0.5: a mean within
		// 0.5/n of the origin.
		if l := sum.Len(); l > 0.5 {
			t.Errorf("n=%d: mean position is %.3g from the origin, want at most %.3g", n, l/float64(n), 0.5/float64(n))
		}
	}
}

func TestFibonacciPointsOrder(t *testing.T) {
	// Samples run from the +z pole to the −z pole in equal steps of z.
	const n = 1000
	points := FibonacciPoints(n)
	for i := 1; i < n; i++ {
		if step := points[i-1].Z - points[i].Z; math.Abs(step-2.0/n) > 1e-12 {
			t.Fatalf("z step at %d = %.17g, want %.17g", i, step, 2.0/n)
		}
	}
	if points[0].Z != 1-1.0/n {
		t.Errorf("z of sample 0 = %.17g, want %.17g", points[0].Z, 1-1.0/n)
	}
}

func TestFibonacciPointsRepeatable(t *testing.T) {
	a, b := FibonacciPoints(2025), FibonacciPoints(2025)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("point %d differs between calls: %v and %v", i, a[i], b[i])
		}
	}
}
