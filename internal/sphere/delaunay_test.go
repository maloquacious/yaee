// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"errors"
	"math"
	"math/rand/v2"
	"slices"
	"testing"
)

// checkMesh verifies every structural property of a triangulation. When
// brute is set it also verifies the Delaunay property against every
// point, which costs O(n²).
func checkMesh(t *testing.T, m *Mesh, brute bool) {
	t.Helper()
	n := len(m.Points)
	nt := m.NumTriangles()
	if nt != 2*n-4 {
		t.Fatalf("n=%d: %d triangles, want %d", n, nt, 2*n-4)
	}
	if len(m.Tri) != 3*nt || len(m.Adj) != 3*nt {
		t.Fatalf("n=%d: len(Tri) = %d, len(Adj) = %d, want %d", n, len(m.Tri), len(m.Adj), 3*nt)
	}
	// Each edge appears twice in the neighbour lists.
	if len(m.Nbr) != 2*(3*n-6) {
		t.Fatalf("n=%d: %d edges, want %d", n, len(m.Nbr)/2, 3*n-6)
	}
	if len(m.NbrOff) != n+1 || m.NbrOff[0] != 0 || int(m.NbrOff[n]) != len(m.Nbr) {
		t.Fatalf("n=%d: bad NbrOff", n)
	}

	for tr := range nt {
		a, b, c := m.Tri[3*tr], m.Tri[3*tr+1], m.Tri[3*tr+2]
		if a == b || b == c || a == c {
			t.Fatalf("n=%d: triangle %d repeats a corner: %d %d %d", n, tr, a, b, c)
		}
		// Counter-clockwise seen from outside.
		if det3(m.Points[a], m.Points[b], m.Points[c]) <= 0 {
			t.Fatalf("n=%d: triangle %d is not counter-clockwise", n, tr)
		}
		// Adjacency is symmetric, across the same edge.
		for e := range 3 {
			u, v := m.Tri[3*tr+e], m.Tri[3*tr+(e+1)%3]
			other := int(m.Adj[3*tr+e])
			if other == tr || other < 0 || other >= nt {
				t.Fatalf("n=%d: triangle %d edge %d has neighbour %d", n, tr, e, other)
			}
			found := false
			for f := range 3 {
				if m.Tri[3*other+f] == v && m.Tri[3*other+(f+1)%3] == u {
					found = int(m.Adj[3*other+f]) == tr
				}
			}
			if !found {
				t.Fatalf("n=%d: triangle %d lists %d across edge %d–%d, but not the reverse", n, tr, other, u, v)
			}
			// The edge is in the neighbour lists.
			if !slices.Contains(m.Neighbours(int(u)), v) {
				t.Fatalf("n=%d: edge %d–%d is missing from the neighbours of %d", n, u, v, u)
			}
		}
	}

	for i := range n {
		list := m.Neighbours(i)
		if len(list) < 3 {
			t.Fatalf("n=%d: point %d has %d neighbours", n, i, len(list))
		}
		if list[0] != slices.Min(list) {
			t.Fatalf("n=%d: neighbours of %d do not start with the lowest: %v", n, i, list)
		}
		for k, j := range list {
			if int(j) == i || slices.Contains(list[:k], j) {
				t.Fatalf("n=%d: bad neighbour list for %d: %v", n, i, list)
			}
			if !slices.Contains(m.Neighbours(int(j)), int32(i)) {
				t.Fatalf("n=%d: %d lists %d as a neighbour, but not the reverse", n, i, j)
			}
			// Consecutive neighbours turn counter-clockwise about i.
			next := list[(k+1)%len(list)]
			if det3(m.Points[i], m.Points[j], m.Points[next]) <= 0 {
				t.Fatalf("n=%d: neighbours of %d are not in counter-clockwise order: %v", n, i, list)
			}
		}
	}

	if brute {
		for tr := range nt {
			corners := m.Tri[3*tr : 3*tr+3]
			a, b, c := m.Points[corners[0]], m.Points[corners[1]], m.Points[corners[2]]
			for i, p := range m.Points {
				if slices.Contains(corners, int32(i)) {
					continue
				}
				if orient(a, b, c, p) < 0 {
					t.Fatalf("n=%d: point %d is inside the circumcircle of triangle %d", n, i, tr)
				}
			}
		}
	}
}

func mustTriangulate(t testing.TB, points []Vec) *Mesh {
	t.Helper()
	m, err := Triangulate(points)
	if err != nil {
		t.Fatalf("Triangulate(%d points): %v", len(points), err)
	}
	return m
}

func randomPoints(n int, seed uint64) []Vec {
	rng := rand.New(rand.NewPCG(seed, seed))
	points := make([]Vec, n)
	for i := range points {
		points[i] = randUnit(rng)
	}
	return points
}

func TestTriangulateFibonacciSmall(t *testing.T) {
	for n := 4; n <= 300; n++ {
		checkMesh(t, mustTriangulate(t, FibonacciPoints(n)), true)
	}
}

func TestTriangulateFibonacciLarger(t *testing.T) {
	for _, n := range []int{317, 512, 777, 1000, 1999, 2000} {
		checkMesh(t, mustTriangulate(t, FibonacciPoints(n)), true)
	}
	sizes := []int{4097, 12_345, 65_537}
	if !testing.Short() {
		sizes = append(sizes, 200_003)
	}
	for _, n := range sizes {
		checkMesh(t, mustTriangulate(t, FibonacciPoints(n)), false)
	}
}

func TestTriangulateRandom(t *testing.T) {
	for seed := range uint64(20) {
		n := 8 + int(seed)*53
		checkMesh(t, mustTriangulate(t, randomPoints(n, seed+100)), true)
	}
	checkMesh(t, mustTriangulate(t, randomPoints(30_011, 7)), false)
}

// Points packed into small clusters far apart give long thin triangles
// and long walks.
func TestTriangulateClustered(t *testing.T) {
	rng := rand.New(rand.NewPCG(21, 21))
	var points []Vec
	for range 12 {
		centre := randUnit(rng)
		for range 60 {
			points = append(points, centre.Add(randUnit(rng).Scale(1e-3*rng.Float64())).Normalize())
		}
	}
	checkMesh(t, mustTriangulate(t, points), true)
}

// Solids whose faces have four or more corners on one circle: the
// plane-side test returns zero, and any of the possible triangulations is
// acceptable.
func TestTriangulateCocircular(t *testing.T) {
	octahedron := []Vec{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	checkMesh(t, mustTriangulate(t, octahedron), true)

	var cube []Vec
	s := 1 / math.Sqrt(3)
	for _, x := range []float64{-s, s} {
		for _, y := range []float64{-s, s} {
			for _, z := range []float64{-s, s} {
				cube = append(cube, Vec{x, y, z})
			}
		}
	}
	checkMesh(t, mustTriangulate(t, cube), true)

	// A latitude–longitude grid: every ring of points is on one circle,
	// and so is every quadrilateral cell.
	grid := []Vec{{0, 0, 1}, {0, 0, -1}}
	for i := 1; i < 12; i++ {
		lat := math.Pi * float64(i) / 12
		for j := range 24 {
			lon := 2 * math.Pi * float64(j) / 24
			grid = append(grid, Vec{math.Sin(lat) * math.Cos(lon), math.Sin(lat) * math.Sin(lon), math.Cos(lat)})
		}
	}
	checkMesh(t, mustTriangulate(t, grid), true)
}

func TestTriangulateRepeatable(t *testing.T) {
	for _, points := range [][]Vec{FibonacciPoints(5003), randomPoints(5003, 9)} {
		a := mustTriangulate(t, points)
		b := mustTriangulate(t, slices.Clone(points))
		if !slices.Equal(a.Tri, b.Tri) || !slices.Equal(a.Adj, b.Adj) ||
			!slices.Equal(a.NbrOff, b.NbrOff) || !slices.Equal(a.Nbr, b.Nbr) {
			t.Fatal("two triangulations of the same points differ")
		}
	}
}

func TestTriangulateErrors(t *testing.T) {
	north := Vec{0, 0, 1}
	ring := func(z float64, n int) []Vec {
		var points []Vec
		r := math.Sqrt(1 - z*z)
		for i := range n {
			a := 2 * math.Pi * float64(i) / float64(n)
			points = append(points, Vec{r * math.Cos(a), r * math.Sin(a), z})
		}
		return points
	}
	for _, tc := range []struct {
		name   string
		points []Vec
		want   error
	}{
		{"none", nil, ErrTooFewPoints},
		{"three", FibonacciPoints(3), ErrTooFewPoints},
		{"too long", []Vec{north, {1, 0, 0}, {0, 1, 0}, {0, 0, 1.1}}, ErrNotOnSphere},
		{"zero", []Vec{north, {1, 0, 0}, {0, 1, 0}, {}}, ErrNotOnSphere},
		{"not a number", []Vec{north, {1, 0, 0}, {0, 1, 0}, {math.NaN(), 0, 0}}, ErrNotOnSphere},
		{"all equal", []Vec{north, north, north, north}, ErrDuplicatePoint},
		{"duplicate", append(FibonacciPoints(50), FibonacciPoint(17, 50)), ErrDuplicatePoint},
		{"great circle", ring(0, 8), ErrCoplanar},
		{"small circle", ring(0.5, 8), ErrCoplanar},
		{"hemisphere", append(ring(0.2, 8), north), ErrHemisphere},
		{"centre on the hull", append(ring(0, 8), north), ErrHemisphere},
	} {
		m, err := Triangulate(tc.points)
		if !errors.Is(err, tc.want) {
			t.Errorf("%s: error = %v, want %v", tc.name, err, tc.want)
		}
		if err != nil && m != nil {
			t.Errorf("%s: a mesh was returned with an error", tc.name)
		}
	}
}

// BenchmarkTriangulateFullScale triangulates the target world: radius
// 6370 km at 25 km spacing, 942,059 samples.
func BenchmarkTriangulateFullScale(b *testing.B) {
	points := FibonacciPoints(SampleCount(6370, 25))
	for b.Loop() {
		mustTriangulate(b, points)
	}
}

// The triangles of a Fibonacci point set cover the sphere exactly once
// and are nearly equal in area. Measured, as a fraction of the mean
// area: smallest 0.821 to 0.823 and largest 1.304 to 1.306, for n from
// 100 to 942,059. The bounds are 0.8 and 1.35.
func TestTriangleAreas(t *testing.T) {
	for _, n := range []int{100, 1000, 10_007, 50_021} {
		m := mustTriangulate(t, FibonacciPoints(n))
		mean := 4 * math.Pi / float64(m.NumTriangles())
		total := 0.0
		for i := 0; i < len(m.Tri); i += 3 {
			a, b, c := m.Points[m.Tri[i]], m.Points[m.Tri[i+1]], m.Points[m.Tri[i+2]]
			// The area of a spherical triangle is its solid angle (Van
			// Oosterom and Strackee, 1983).
			area := 2 * math.Atan2(a.Dot(b.Cross(c)), 1+a.Dot(b)+b.Dot(c)+c.Dot(a))
			if ratio := area / mean; ratio < 0.8 || ratio > 1.35 {
				t.Fatalf("n=%d: triangle %d has %.4f of the mean area, want 0.8 to 1.35", n, i/3, ratio)
			}
			total += area
		}
		if math.Abs(total/(4*math.Pi)-1) > 1e-9 {
			t.Errorf("n=%d: triangle areas sum to %.12f of the sphere", n, total/(4*math.Pi))
		}
	}
}

// NewMesh rebuilds, from the points and triangles alone, the same Mesh
// that Triangulate returned.
func TestNewMeshMatchesTriangulate(t *testing.T) {
	for _, points := range [][]Vec{FibonacciPoints(4), FibonacciPoints(6), FibonacciPoints(1000), randomPoints(500, 3)} {
		want := mustTriangulate(t, points)
		got, err := NewMesh(points, slices.Clone(want.Tri))
		if err != nil {
			t.Fatalf("n=%d: %v", len(points), err)
		}
		checkMesh(t, got, false)
		if !slices.Equal(got.Adj, want.Adj) || !slices.Equal(got.NbrOff, want.NbrOff) || !slices.Equal(got.Nbr, want.Nbr) {
			t.Fatalf("n=%d: NewMesh and Triangulate disagree", len(points))
		}
	}
}

func TestNewMeshRejectsBadTriangles(t *testing.T) {
	points := FibonacciPoints(50)
	good := mustTriangulate(t, points).Tri
	tests := []struct {
		name   string
		change func(tri []int32) []int32
	}{
		{"no triangles", func(tri []int32) []int32 { return nil }},
		{"not a multiple of three", func(tri []int32) []int32 { return tri[:len(tri)-1] }},
		{"a triangle missing", func(tri []int32) []int32 { return tri[3:] }},
		{"a triangle twice", func(tri []int32) []int32 { return append(tri, tri[:3]...) }},
		{"corner out of range", func(tri []int32) []int32 { tri[4] = 50; return tri }},
		{"negative corner", func(tri []int32) []int32 { tri[4] = -1; return tri }},
		{"repeated corner", func(tri []int32) []int32 { tri[1] = tri[0]; return tri }},
		{"one triangle reversed", func(tri []int32) []int32 { tri[0], tri[1] = tri[1], tri[0]; return tri }},
		{"every triangle reversed", func(tri []int32) []int32 {
			for i := 0; i < len(tri); i += 3 {
				tri[i], tri[i+1] = tri[i+1], tri[i]
			}
			return tri
		}},
		{"a point in no triangle", func(tri []int32) []int32 {
			// Replace the fan about point 0 by nothing: retarget its
			// corners to a neighbour, which breaks the surface.
			for i, p := range tri {
				if p == 0 {
					tri[i] = 1
				}
			}
			return tri
		}},
	}
	for _, tt := range tests {
		if _, err := NewMesh(points, tt.change(slices.Clone(good))); !errors.Is(err, ErrInvalidMesh) {
			t.Errorf("%s: got %v, want ErrInvalidMesh", tt.name, err)
		}
	}

	// Two tetrahedra that touch at point 0 only: every edge has its twin,
	// but the triangles about point 0 form two fans.
	pinch := []Vec{{0, 0, 1}, {1, 0, 0}, {0, 1, 0}, {-1, -1, -1}, {-1, 0, 0}, {0, -1, 0}, {1, 1, -1}}
	for i := range pinch {
		pinch[i] = pinch[i].Normalize()
	}
	tetra := func(a, b, c, d int32) []int32 { return []int32{a, b, c, a, c, d, a, d, b, b, d, c} }
	tri := append(tetra(0, 1, 2, 3), tetra(0, 4, 5, 6)...)
	if _, err := NewMesh(pinch, tri); !errors.Is(err, ErrInvalidMesh) {
		t.Errorf("pinched surface: got %v, want ErrInvalidMesh", err)
	}
}
