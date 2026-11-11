// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/rand/v2"
	"sync"
	"testing"
)

// bruteLocate returns the lowest-numbered triangle that contains p, by
// testing every triangle, or -1 if none does.
func bruteLocate(m *Mesh, p Vec) int32 {
	for t := range int32(m.NumTriangles()) {
		a, b, c := m.Points[m.Tri[3*t]], m.Points[m.Tri[3*t+1]], m.Points[m.Tri[3*t+2]]
		if det3(a, b, p) >= 0 && det3(b, c, p) >= 0 && det3(c, a, p) >= 0 {
			return t
		}
	}
	return -1
}

// bruteNearest returns the point with the greatest dot product with p.
func bruteNearest(m *Mesh, p Vec) int32 {
	best, bestDot := int32(0), math.Inf(-1)
	for i, q := range m.Points {
		if d := q.Dot(p); d > bestDot {
			best, bestDot = int32(i), d
		}
	}
	return best
}

// checkLocate locates p and verifies the result against brute force and
// the weight rules.
func checkLocate(t *testing.T, l *Locator, p Vec) (int32, [3]float64) {
	t.Helper()
	m := l.Mesh()
	tri, w := l.Locate(p)
	if want := bruteLocate(m, p); tri != want {
		t.Fatalf("n=%d: Locate(%v) = triangle %d, want %d", len(m.Points), p, tri, want)
	}
	if sum := w[0] + w[1] + w[2]; math.Abs(sum-1) > 1e-12 {
		t.Fatalf("n=%d: weights of %v sum to %.17g", len(m.Points), p, sum)
	}
	var q Vec
	for k, wk := range w {
		if wk < 0 || wk > 1 {
			t.Fatalf("n=%d: weight %d of %v is %g", len(m.Points), k, p, wk)
		}
		q = q.Add(m.Points[m.Tri[3*int(tri)+k]].Scale(wk))
	}
	// The weighted sum of the corners is on the ray through p.
	if d := q.Normalize().Sub(p.Normalize()).Len(); d > 1e-12 {
		t.Fatalf("n=%d: weights of %v give a point %g away", len(m.Points), p, d)
	}
	return tri, w
}

func locatorFor(t testing.TB, points []Vec) *Locator {
	t.Helper()
	return NewLocator(mustTriangulate(t, points))
}

func TestLocateRandomPoints(t *testing.T) {
	rng := rand.New(rand.NewPCG(31, 31))
	for _, points := range [][]Vec{
		FibonacciPoints(4), FibonacciPoints(5), FibonacciPoints(13), FibonacciPoints(100),
		FibonacciPoints(1009), FibonacciPoints(4000), randomPoints(700, 5),
	} {
		l := locatorFor(t, points)
		for range 500 {
			p := randUnit(rng)
			checkLocate(t, l, p)
			// The same point at another length gives the same answer.
			tri, _ := l.Locate(p)
			if far, _ := l.Locate(p.Scale(6370)); far != tri {
				t.Fatalf("scaling the query changed the triangle from %d to %d", tri, far)
			}
			if got, want := l.Nearest(p), bruteNearest(l.Mesh(), p); got != want {
				t.Fatalf("n=%d: Nearest(%v) = %d, want %d", len(points), p, got, want)
			}
		}
	}
}

func TestLocateOwnPositions(t *testing.T) {
	for _, n := range []int{4, 7, 50, 1000, 20_011} {
		l := locatorFor(t, FibonacciPoints(n))
		m := l.Mesh()
		for i, p := range m.Points {
			tri, w := l.Locate(p)
			found := false
			for k := range 3 {
				if int(m.Tri[3*int(tri)+k]) == i {
					found = true
					if math.Abs(w[k]-1) > 1e-9 {
						t.Fatalf("n=%d: sample %d has weight %.17g at its own position", n, i, w[k])
					}
				}
			}
			if !found {
				t.Fatalf("n=%d: sample %d is not a corner of the triangle found at its position", n, i)
			}
			if got := l.Nearest(p); int(got) != i {
				t.Fatalf("n=%d: Nearest of sample %d's position = %d", n, i, got)
			}
		}
	}
	// On a small mesh, also check the lowest-triangle rule at corners.
	l := locatorFor(t, FibonacciPoints(300))
	for _, p := range l.Mesh().Points {
		checkLocate(t, l, p)
	}
}

func TestLocatePoles(t *testing.T) {
	for _, n := range []int{4, 5, 6, 100, 1001, 5000} {
		l := locatorFor(t, FibonacciPoints(n))
		for _, p := range []Vec{{0, 0, 1}, {0, 0, -1}, {1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}} {
			checkLocate(t, l, p)
		}
	}
}

func TestLocateNearEdges(t *testing.T) {
	rng := rand.New(rand.NewPCG(32, 32))
	for _, n := range []int{20, 500, 3000} {
		l := locatorFor(t, FibonacciPoints(n))
		m := l.Mesh()
		for range 300 {
			tri := rng.IntN(m.NumTriangles())
			e := rng.IntN(3)
			a, b := m.Points[m.Tri[3*tri+e]], m.Points[m.Tri[3*tri+(e+1)%3]]
			s := rng.Float64()
			onEdge := a.Scale(s).Add(b.Scale(1 - s)).Normalize()
			for _, offset := range []float64{0, 1e-12, -1e-12, 1e-15, -1e-15} {
				p := onEdge.Add(a.Cross(b).Normalize().Scale(offset)).Normalize()
				checkLocate(t, l, p)
			}
		}
	}
}

// On an octahedron the edges lie on the coordinate planes, so points
// exactly on an edge can be written down.
func TestLocateTieRule(t *testing.T) {
	l := locatorFor(t, []Vec{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}})
	m := l.Mesh()
	containing := func(p Vec) []int32 {
		var list []int32
		for t := range int32(m.NumTriangles()) {
			a, b, c := m.Points[m.Tri[3*t]], m.Points[m.Tri[3*t+1]], m.Points[m.Tri[3*t+2]]
			if det3(a, b, p) >= 0 && det3(b, c, p) >= 0 && det3(c, a, p) >= 0 {
				list = append(list, t)
			}
		}
		return list
	}
	for _, tc := range []struct {
		p     Vec
		count int // triangles that contain p
	}{
		{Vec{0.6, 0.8, 0}, 2},
		{Vec{0, -0.6, 0.8}, 2},
		{Vec{-0.8, 0, -0.6}, 2},
		{Vec{1, 0, 0}, 4},
		{Vec{0, 0, -1}, 4},
		{Vec{0.5, 0.5, 0.5}, 1},
	} {
		list := containing(tc.p)
		if len(list) != tc.count {
			t.Fatalf("%v is contained by %d triangles, want %d", tc.p, len(list), tc.count)
		}
		if tri, _ := checkLocate(t, l, tc.p); tri != list[0] {
			t.Errorf("Locate(%v) = %d, want the lowest of %v", tc.p, tri, list)
		}
	}
	// A point on an edge has zero weight for the corner opposite it.
	tri, w := l.Locate(Vec{0.6, 0.8, 0})
	for k, wk := range w {
		if m.Points[m.Tri[3*int(tri)+k]].Z != 0 && wk != 0 {
			t.Errorf("weight of the corner off the edge = %g, want 0", wk)
		}
	}
}

func TestLocateWeightsInterpolate(t *testing.T) {
	// A function that is linear in position is reproduced exactly on the
	// flat triangle, so interpolating it at p gives its value at the
	// point where p's ray meets the triangle.
	l := locatorFor(t, FibonacciPoints(2000))
	m := l.Mesh()
	rng := rand.New(rand.NewPCG(33, 33))
	f := func(v Vec) float64 { return 2*v.X - 3*v.Y + 0.5*v.Z }
	for range 1000 {
		p := randUnit(rng)
		tri, w := l.Locate(p)
		a, b, c := m.Points[m.Tri[3*tri]], m.Points[m.Tri[3*tri+1]], m.Points[m.Tri[3*tri+2]]
		got := w[0]*f(a) + w[1]*f(b) + w[2]*f(c)
		// Where the ray meets the plane through a, b and c.
		normal := b.Sub(a).Cross(c.Sub(a))
		q := p.Scale(normal.Dot(a) / normal.Dot(p))
		if want := f(q); math.Abs(got-want) > 1e-9 {
			t.Fatalf("interpolated %g, want %g", got, want)
		}
	}
}

func TestLocateConcurrent(t *testing.T) {
	l := locatorFor(t, FibonacciPoints(10_007))
	rng := rand.New(rand.NewPCG(34, 34))
	queries := make([]Vec, 20_000)
	for i := range queries {
		queries[i] = randUnit(rng)
	}
	type answer struct {
		tri     int32
		weights [3]float64
		nearest int32
	}
	ask := func(p Vec) answer {
		tri, w := l.Locate(p)
		return answer{tri, w, l.Nearest(p)}
	}
	want := make([]answer, len(queries))
	for i, p := range queries {
		want[i] = ask(p)
	}
	const workers = 8
	got := make([][]answer, workers)
	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			got[w] = make([]answer, len(queries))
			for i, p := range queries {
				got[w][i] = ask(p)
			}
		})
	}
	wg.Wait()
	for w := range workers {
		for i := range queries {
			if got[w][i] != want[i] {
				t.Fatalf("worker %d, query %d: got %v, want %v", w, i, got[w][i], want[i])
			}
		}
	}
}

// fullScale builds the target world's mesh and locator: radius 6370 km
// at 25 km spacing, 942,059 samples.
func fullScale(b *testing.B) (*Locator, []Vec) {
	l := locatorFor(b, FibonacciPoints(SampleCount(6370, 25)))
	rng := rand.New(rand.NewPCG(35, 35))
	queries := make([]Vec, 1<<20)
	for i := range queries {
		queries[i] = randUnit(rng)
	}
	return l, queries
}

// BenchmarkBuildFullScale samples, triangulates and indexes the target
// world.
func BenchmarkBuildFullScale(b *testing.B) {
	for b.Loop() {
		locatorFor(b, FibonacciPoints(SampleCount(6370, 25)))
	}
}

// BenchmarkLocateFullScale locates uniformly random points, one
// goroutine. Each iteration is one query.
func BenchmarkLocateFullScale(b *testing.B) {
	l, queries := fullScale(b)
	i := 0
	for b.Loop() {
		l.Locate(queries[i&(len(queries)-1)])
		i++
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/s")
}

// BenchmarkNearestFullScale finds the nearest sample to uniformly random
// points, one goroutine. Each iteration is one query.
func BenchmarkNearestFullScale(b *testing.B) {
	l, queries := fullScale(b)
	i := 0
	for b.Loop() {
		l.Nearest(queries[i&(len(queries)-1)])
		i++
	}
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/s")
}

// BenchmarkLocateFullScaleParallel is BenchmarkLocateFullScale on every
// processor.
func BenchmarkLocateFullScaleParallel(b *testing.B) {
	l, queries := fullScale(b)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			l.Locate(queries[i&(len(queries)-1)])
			i++
		}
	})
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "queries/s")
}
