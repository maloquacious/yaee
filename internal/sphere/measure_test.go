// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

func TestTriangleArea(t *testing.T) {
	x, y, z := Vec{X: 1}, Vec{Y: 1}, Vec{Z: 1}
	// An octant is an eighth of the sphere.
	if got, want := TriangleArea(x, y, z), math.Pi/2; math.Abs(got-want) > 1e-15 {
		t.Errorf("octant: got %v, want %v", got, want)
	}
	if got, want := TriangleArea(x, z, y), math.Pi/2; math.Abs(got-want) > 1e-15 {
		t.Errorf("octant, clockwise: got %v, want %v", got, want)
	}
	// A small triangle is nearly flat: half base times height.
	const e = 1e-4
	a, b, c := Vec{X: 1}, Vec{X: 1, Y: e}.Normalize(), Vec{X: 1, Z: e}.Normalize()
	if got, want := TriangleArea(a, b, c), e*e/2; math.Abs(got-want) > 1e-6*want {
		t.Errorf("small triangle: got %v, want %v", got, want)
	}
	if got := TriangleArea(a, a, b); got != 0 {
		t.Errorf("degenerate triangle: got %v, want 0", got)
	}
}

func TestPointAreas(t *testing.T) {
	for _, n := range []int{4, 12, 500, 20000} {
		m, err := Triangulate(FibonacciPoints(n))
		if err != nil {
			t.Fatal(err)
		}
		areas := m.PointAreas()
		if len(areas) != n {
			t.Fatalf("%d areas for %d points", len(areas), n)
		}
		var sum float64
		for _, a := range areas {
			sum += a
		}
		if math.Abs(sum-4*math.Pi) > 1e-9 {
			t.Errorf("n = %d: areas sum to %v, want 4π = %v", n, sum, 4*math.Pi)
		}
		if n >= 500 {
			mean := 4 * math.Pi / float64(n)
			if lo, hi := slices.Min(areas)/mean, slices.Max(areas)/mean; lo < 0.7 || hi > 1.4 {
				t.Errorf("n = %d: areas are %v to %v of the mean, want 0.7 to 1.4", n, lo, hi)
			}
		}
	}
}

func TestDistances(t *testing.T) {
	const n = 3000
	points := FibonacciPoints(n)
	m, err := Triangulate(points)
	if err != nil {
		t.Fatal(err)
	}
	spacing := SampleSpacing(1, n)
	rng := rand.New(rand.NewPCG(1, 2))

	for trial, count := range []int{1, 2, 5, 40, 150, 1500, n} {
		sources := make([]int32, count)
		for i, s := range rng.Perm(n)[:count] {
			sources[i] = int32(s)
		}
		dist, nearest := m.Distances(sources)

		var wrong int
		var worst float64
		for i, p := range points {
			// Brute force.
			want, wantSrc := math.Inf(1), int32(-1)
			for _, s := range sources {
				d := Angle(p, points[s])
				if s == int32(i) {
					d = 0 // Angle(p, p) is not exactly 0
				}
				if d < want || (d == want && s < wantSrc) {
					want, wantSrc = d, s
				}
			}
			// The distance is always that to the source reported.
			if got := Angle(p, points[nearest[i]]); dist[i] != got && nearest[i] != int32(i) {
				t.Fatalf("trial %d: point %d: distance %v, but its source %d is %v away", trial, i, dist[i], nearest[i], got)
			}
			if dist[i] < want {
				t.Fatalf("trial %d: point %d: distance %v is less than the true %v", trial, i, dist[i], want)
			}
			if nearest[i] != wantSrc {
				wrong++
				worst = max(worst, dist[i]-want)
			}
		}
		if wrong > n/1000 {
			t.Errorf("trial %d (%d sources): %d of %d points took a source that is not the nearest", trial, count, wrong, n)
		}
		if worst > 0.5*spacing {
			t.Errorf("trial %d (%d sources): a distance is too large by %v, more than half a sample spacing (%v)", trial, count, worst, 0.5*spacing)
		}
		for _, s := range sources {
			if dist[s] != 0 || nearest[s] != s {
				t.Fatalf("trial %d: source %d has distance %v and nearest source %d", trial, s, dist[s], nearest[s])
			}
		}

		// The order of the sources, and repeats among them, change
		// nothing.
		shuffled := append(slices.Clone(sources), sources[0])
		slices.Reverse(shuffled)
		dist2, nearest2 := m.Distances(shuffled)
		if !slices.Equal(dist, dist2) || !slices.Equal(nearest, nearest2) {
			t.Errorf("trial %d: the result depends on the order of the sources", trial)
		}
	}

	dist, nearest := m.Distances(nil)
	for i := range dist {
		if !math.IsInf(dist[i], 1) || nearest[i] != -1 {
			t.Fatalf("no sources: point %d has distance %v and source %d", i, dist[i], nearest[i])
		}
	}
}

// Sources along a line, the case the generator has: a boundary.
func TestDistancesFromALine(t *testing.T) {
	const n = 20000
	points := FibonacciPoints(n)
	m, err := Triangulate(points)
	if err != nil {
		t.Fatal(err)
	}
	spacing := SampleSpacing(1, n)
	// The sources are the points within half a spacing of the equator.
	var sources []int32
	for i, p := range points {
		if math.Abs(p.Z) < math.Sin(spacing/2) {
			sources = append(sources, int32(i))
		}
	}
	dist, _ := m.Distances(sources)
	for i, p := range points {
		// The distance to the equator is the latitude; the sources are
		// within half a spacing of it, and about a spacing apart.
		if want := math.Abs(math.Asin(p.Z)); math.Abs(dist[i]-want) > spacing {
			t.Fatalf("point %d at latitude %v has distance %v", i, want, dist[i])
		}
	}
}

func BenchmarkDistancesFullScale(b *testing.B) {
	if testing.Short() {
		b.Skip("full scale")
	}
	n := SampleCount(6370, 25)
	points := FibonacciPoints(n)
	m, err := Triangulate(points)
	if err != nil {
		b.Fatal(err)
	}
	var sources []int32
	for i, p := range points {
		if math.Abs(p.Z) < 0.002 || math.Abs(p.X) < 0.002 {
			sources = append(sources, int32(i))
		}
	}
	for b.Loop() {
		m.Distances(sources)
	}
}
