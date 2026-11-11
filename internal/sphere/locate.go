// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// Locator answers two questions about any point on the sphere: which
// triangle of a Mesh contains it, and which point of the Mesh is nearest
// to it.
//
// A Locator is not changed after NewLocator returns it. Its methods only
// read it and the Mesh, so they may be called from many goroutines at
// once.
//
// # Method
//
// The sphere is projected onto the six faces of a cube, and each face is
// divided into a square grid with about as many cells in total as the
// Mesh has points. Each cell records a triangle at its centre. A query
// starts at the triangle recorded for the query point's cell and walks
// across edges toward the point, which takes a few steps. This departs
// from the paper, which locates points with a bounding volume hierarchy
// over each plate's triangles (§6); a walk on a fixed mesh is simpler and
// has no hierarchy to rebuild.
type Locator struct {
	mesh  *Mesh
	grid  int     // cells along each side of a cube face
	start []int32 // for each cell, a triangle that contains its centre
}

// NewLocator builds the index for m. The cost is proportional to the
// number of points in m.
func NewLocator(m *Mesh) *Locator {
	g := max(1, int(math.Ceil(math.Sqrt(float64(len(m.Points))/6))))
	l := &Locator{mesh: m, grid: g, start: make([]int32, 6*g*g)}
	// Consecutive cells are adjacent, so each walk starts where the last
	// one ended.
	t := int32(0)
	for c := range l.start {
		t, _ = l.walk(l.cellCentre(c), t)
		l.start[c] = t
	}
	return l
}

// Mesh returns the Mesh that l indexes.
func (l *Locator) Mesh() *Mesh { return l.mesh }

// Locate returns the triangle of the Mesh that contains p, and p's
// barycentric weights for that triangle's three corners.
//
// p must be finite and not zero. It need not have length 1: the result
// is that of the point where the ray from the centre through p meets the
// sphere.
//
// weights[k] belongs to corner Tri[3·tri+k]. With a, b and c the
// positions of the three corners, the weights are those of the point q
// where that ray crosses the flat triangle a, b, c:
//
//	q = weights[0]·a + weights[1]·b + weights[2]·c
//
// Each weight is in [0, 1] and they sum to 1. To interpolate a
// per-sample value at p, take the same weighted sum of the value at the
// three corners.
//
// A triangle contains p if p is on its inner side of, or exactly on, the
// great circle through each of its edges. A point exactly on an edge is
// therefore contained by two triangles, and a point exactly at a corner
// by every triangle that has the corner. Locate always returns the
// lowest-numbered triangle that contains p. The edge tests are exact, so
// the result does not depend on where the search started.
func (l *Locator) Locate(p Vec) (tri int32, weights [3]float64) {
	m := l.mesh
	tri = l.find(p)
	a, b, c := m.Points[m.Tri[3*tri]], m.Points[m.Tri[3*tri+1]], m.Points[m.Tri[3*tri+2]]
	// p = α·a + β·b + γ·c has, by Cramer's rule, α proportional to the
	// determinant with p in place of a, and so on. Scaling the three to
	// sum to 1 moves p along its ray onto the plane of the triangle.
	// Rounding can leave a weight a hair below zero; clamp it.
	w0 := max(0, p.Dot(b.Cross(c)))
	w1 := max(0, p.Dot(c.Cross(a)))
	w2 := max(0, p.Dot(a.Cross(b)))
	sum := w0 + w1 + w2
	return tri, [3]float64{w0 / sum, w1 / sum, w2 / sum}
}

// Nearest returns the point of the Mesh that is nearest to p: the one
// whose position has the greatest dot product with p. If neighbouring
// points have equal dot products, the lower index is returned.
//
// p must be finite and not zero. It need not have length 1.
func (l *Locator) Nearest(p Vec) int32 {
	m := l.mesh
	t := l.find(p)
	best := m.Tri[3*t]
	bestDot := m.Points[best].Dot(p)
	for _, i := range m.Tri[3*t+1 : 3*t+3] {
		if d := m.Points[i].Dot(p); d > bestDot || d == bestDot && i < best {
			best, bestDot = i, d
		}
	}
	// The nearest point is not always a corner of the containing
	// triangle. But in a Delaunay triangulation, every point other than
	// the nearest has a neighbour that is nearer, so stepping to the
	// nearest neighbour until none is nearer ends at the nearest point.
	for {
		at := best
		for _, i := range m.Neighbours(int(at)) {
			if d := m.Points[i].Dot(p); d > bestDot || d == bestDot && i < best {
				best, bestDot = i, d
			}
		}
		if best == at {
			return best
		}
	}
}

// find returns the lowest-numbered triangle that contains p.
func (l *Locator) find(p Vec) int32 {
	m := l.mesh
	t, zero := l.walk(p, l.start[l.cell(p)])
	// zero[e] reports that p is exactly on the great circle of edge e.
	var corner int32
	switch {
	case zero[0] && zero[1] && zero[2]:
		return t // only the zero vector, which is not a valid query
	case zero[2] && zero[0]:
		corner = 0
	case zero[0] && zero[1]:
		corner = 1
	case zero[1] && zero[2]:
		corner = 2
	case zero[0]:
		return min(t, m.Adj[3*t])
	case zero[1]:
		return min(t, m.Adj[3*t+1])
	case zero[2]:
		return min(t, m.Adj[3*t+2])
	default:
		return t
	}
	// p is at a corner. Visit every triangle around that corner: the
	// edge that ends at the corner leads to the next one.
	v := m.Tri[3*t+corner]
	lowest := t
	for u := m.Adj[3*t+(corner+2)%3]; u != t; {
		lowest = min(lowest, u)
		k := int32(0)
		for m.Tri[3*u+k] != v {
			k++
		}
		u = m.Adj[3*u+(k+2)%3]
	}
	return lowest
}

// walk returns a triangle that contains p, starting the search at
// triangle t, and reports which of that triangle's edges have p exactly
// on their great circle.
func (l *Locator) walk(p Vec, t int32) (int32, [3]bool) {
	m := l.mesh
	for range m.NumTriangles() + 1 {
		a, b, c := m.Points[m.Tri[3*t]], m.Points[m.Tri[3*t+1]], m.Points[m.Tri[3*t+2]]
		s0 := det3(a, b, p)
		if s0 < 0 {
			t = m.Adj[3*t]
			continue
		}
		s1 := det3(b, c, p)
		if s1 < 0 {
			t = m.Adj[3*t+1]
			continue
		}
		s2 := det3(c, a, p)
		if s2 < 0 {
			t = m.Adj[3*t+2]
			continue
		}
		return t, [3]bool{s0 == 0, s1 == 0, s2 == 0}
	}
	// The walk did not arrive. With exact tests on a Delaunay
	// triangulation it always does, so this is a safeguard: test every
	// triangle.
	for t := range int32(m.NumTriangles()) {
		a, b, c := m.Points[m.Tri[3*t]], m.Points[m.Tri[3*t+1]], m.Points[m.Tri[3*t+2]]
		s0, s1, s2 := det3(a, b, p), det3(b, c, p), det3(c, a, p)
		if s0 >= 0 && s1 >= 0 && s2 >= 0 {
			return t, [3]bool{s0 == 0, s1 == 0, s2 == 0}
		}
	}
	panic("sphere: no triangle contains the point")
}

// cell returns the grid cell that p projects into. Cells are numbered
// face·grid² + row·grid + column, with faces in the order +x, −x, +y,
// −y, +z, −z.
func (l *Locator) cell(p Vec) int {
	ax, ay, az := math.Abs(p.X), math.Abs(p.Y), math.Abs(p.Z)
	var face int
	var u, v float64
	switch {
	case ax >= ay && ax >= az:
		face, u, v = 0, p.Y/ax, p.Z/ax
		if p.X < 0 {
			face = 1
		}
	case ay >= az:
		face, u, v = 2, p.Z/ay, p.X/ay
		if p.Y < 0 {
			face = 3
		}
	default:
		face, u, v = 4, p.X/az, p.Y/az
		if p.Z < 0 {
			face = 5
		}
	}
	g := l.grid
	col := min(g-1, max(0, int((u+1)/2*float64(g))))
	row := min(g-1, max(0, int((v+1)/2*float64(g))))
	return (face*g+row)*g + col
}

// cellCentre returns a point that projects to the centre of cell c. It
// does not have length 1.
func (l *Locator) cellCentre(c int) Vec {
	g := l.grid
	face, row, col := c/(g*g), c/g%g, c%g
	u := (float64(col)+0.5)/float64(g)*2 - 1
	v := (float64(row)+0.5)/float64(g)*2 - 1
	switch face {
	case 0:
		return Vec{1, u, v}
	case 1:
		return Vec{-1, u, v}
	case 2:
		return Vec{v, 1, u}
	case 3:
		return Vec{v, -1, u}
	case 4:
		return Vec{u, v, 1}
	default:
		return Vec{u, v, -1}
	}
}
