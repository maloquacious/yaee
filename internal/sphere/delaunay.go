// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"cmp"
	"fmt"
	"math"
	"slices"

	"github.com/maloquacious/yaee/internal/cerrs"
)

// Errors returned by Triangulate. Test for them with errors.Is.
const (
	// ErrTooFewPoints: fewer than 4 points were given.
	ErrTooFewPoints = cerrs.Error("sphere: fewer than 4 points")
	// ErrTooManyPoints: more than MaxPoints points were given.
	ErrTooManyPoints = cerrs.Error("sphere: too many points")
	// ErrNotOnSphere: a point's squared length differs from 1 by more
	// than 1e-9, or is not finite.
	ErrNotOnSphere = cerrs.Error("sphere: point is not on the unit sphere")
	// ErrDuplicatePoint: two points have identical coordinates.
	ErrDuplicatePoint = cerrs.Error("sphere: duplicate point")
	// ErrCoplanar: all the points lie on one plane.
	ErrCoplanar = cerrs.Error("sphere: points are coplanar")
	// ErrHemisphere: the centre of the sphere is not strictly inside the
	// convex hull of the points, so the triangles cannot cover the sphere.
	ErrHemisphere = cerrs.Error("sphere: points do not surround the centre of the sphere")
)

// MaxPoints is the largest number of points Triangulate accepts. It keeps
// every index into Mesh.Tri within an int32.
const MaxPoints = math.MaxInt32 / 6

// Mesh is a Delaunay triangulation of points on the unit sphere. For N
// points it has 2N − 4 triangles and 3N − 6 edges.
//
// Points, triangles and edges are identified by number. A Mesh is not
// changed after Triangulate returns it, so it may be read from many
// goroutines at once.
type Mesh struct {
	// Points holds the positions that were triangulated. It is the slice
	// passed to Triangulate, not a copy.
	Points []Vec

	// Tri holds three point indices per triangle: triangle t has corners
	// Tri[3t], Tri[3t+1] and Tri[3t+2], in counter-clockwise order seen
	// from outside the sphere.
	Tri []int32

	// Adj holds three triangle indices per triangle. Edge e (0, 1 or 2)
	// of triangle t runs from corner Tri[3t+e] to corner Tri[3t+(e+1)%3],
	// and Adj[3t+e] is the triangle on the other side of that edge.
	Adj []int32

	// NbrOff and Nbr hold each point's neighbours: the points it shares
	// an edge with. The neighbours of point i are
	// Nbr[NbrOff[i]:NbrOff[i+1]], in counter-clockwise order seen from
	// outside the sphere, starting with the lowest index. NbrOff has one
	// more element than Points.
	NbrOff []int32
	Nbr    []int32
}

// NumTriangles returns the number of triangles.
func (m *Mesh) NumTriangles() int { return len(m.Tri) / 3 }

// Neighbours returns the neighbours of point i. The result is part of
// m.Nbr and must not be modified.
func (m *Mesh) Neighbours(i int) []int32 { return m.Nbr[m.NbrOff[i]:m.NbrOff[i+1]] }

// Triangulate returns the Delaunay triangulation of points on the unit
// sphere.
//
// Requirements, each reported by the error named:
//   - at least 4 and at most MaxPoints points (ErrTooFewPoints,
//     ErrTooManyPoints);
//   - every point has a squared length within 1e-9 of 1 (ErrNotOnSphere);
//   - no two points are identical (ErrDuplicatePoint);
//   - the points are not all on one plane (ErrCoplanar);
//   - the centre of the sphere is strictly inside the convex hull of the
//     points (ErrHemisphere).
//
// Points closer together than about 1e-5 radians are not supported: at
// that distance a point's rounding error can put it inside the hull of
// its neighbours, and the result is then undefined.
//
// Where four or more points lie on one circle, more than one Delaunay
// triangulation exists, and one of them is returned. The result depends
// only on the input: the same points in the same order always give the
// same Mesh, element for element.
//
// # Method
//
// The Delaunay triangulation of points on a sphere is their convex hull
// in three dimensions, so the hull is what is built. Starting from a
// tetrahedron of four of the points, each remaining point is added by
// finding the hull face below it, splitting that face in three, and
// flipping edges until the hull is convex again (Lawson's algorithm,
// with the in-circle test replaced by a plane-side test).
//
// This was chosen over projecting the points stereographically and
// triangulating in the plane because it needs no projection point that
// must be stitched back in, the sphere has no boundary to treat as a
// special case, and the only geometric test is the sign of one
// determinant of the original coordinates. That sign is computed exactly
// (see orient), so nearly degenerate input cannot corrupt the mesh.
//
// Points are added in an order that keeps each one close to the one
// before, so the search for the face below a point starts nearby and
// takes constant time on average. The whole build is O(n log n), the
// cost of sorting the points into that order.
func Triangulate(points []Vec) (*Mesh, error) {
	n := len(points)
	if n < 4 {
		return nil, fmt.Errorf("%w: got %d", ErrTooFewPoints, n)
	}
	if n > MaxPoints {
		return nil, fmt.Errorf("%w: got %d, the limit is %d", ErrTooManyPoints, n, MaxPoints)
	}
	for i, p := range points {
		if l2 := p.Dot(p); !(math.Abs(l2-1) <= 1e-9) {
			return nil, fmt.Errorf("%w: point %d has squared length %g", ErrNotOnSphere, i, l2)
		}
	}

	b := &builder{
		pts:  points,
		tri:  make([]int32, 0, 3*(2*n-4)),
		twin: make([]int32, 0, 3*(2*n-4)),
	}
	seed, err := b.seed()
	if err != nil {
		return nil, err
	}

	// Insert in rounds, each a pass over the sorted order that takes the
	// points at a stride not taken by an earlier round. The early rounds
	// lay down a coarse triangulation of the whole sphere, so that later
	// insertions only disturb small triangles.
	order := insertionOrder(points, seed)
	top := 1
	for top*8 < len(order) {
		top *= 8
	}
	for stride := top; stride >= 1; stride /= 8 {
		for i := 0; i < len(order); i += stride {
			if stride != top && i%(stride*8) == 0 {
				continue // taken by an earlier round
			}
			if err := b.insert(order[i]); err != nil {
				return nil, err
			}
		}
	}

	// Every face must be wound counter-clockwise about the sphere's
	// centre; if one is not, the centre is outside the hull.
	for i := 0; i < len(b.tri); i += 3 {
		if orient(points[b.tri[i]], points[b.tri[i+1]], points[b.tri[i+2]], Vec{}) <= 0 {
			return nil, ErrHemisphere
		}
	}

	m := &Mesh{Points: points, Tri: b.tri}
	m.NbrOff, m.Nbr = b.neighbours()
	// Turn each half-edge's twin into the triangle that holds the twin.
	m.Adj = b.twin
	for i, e := range m.Adj {
		m.Adj[i] = e / 3
	}
	return m, nil
}

// builder holds the hull while it is being built.
//
// The hull is stored as half-edges. Half-edge e belongs to triangle e/3
// and runs from point tri[e] to point tri[next(e)]. twin[e] is the
// half-edge that runs the other way along the same edge, in the
// neighbouring triangle.
type builder struct {
	pts    []Vec
	tri    []int32
	twin   []int32
	centre Vec     // a point strictly inside the hull
	hint   int32   // the triangle where the next search starts
	stack  []int32 // scratch space for legalize
}

// next returns the half-edge after e in its triangle; prev, the one before.
func next(e int32) int32 { return e - e%3 + (e+1)%3 }
func prev(e int32) int32 { return e - e%3 + (e+2)%3 }

func (b *builder) link(e, f int32) {
	b.twin[e] = f
	b.twin[f] = e
}

// seed builds the first tetrahedron and returns its four points.
func (b *builder) seed() ([4]int32, error) {
	pts := b.pts
	// Point 0; the point farthest from it; the point farthest from the
	// line through those two; the point farthest from the plane through
	// those three. This gives a large tetrahedron if any exists.
	p0 := pts[0]
	i1, best := 0, math.Inf(1)
	for i, p := range pts {
		if d := p0.Dot(p); d < best {
			i1, best = i, d
		}
	}
	u := pts[i1].Sub(p0)
	i2, best := 0, -1.0
	for i, p := range pts {
		c := u.Cross(p.Sub(p0))
		if d := c.Dot(c); d > best {
			i2, best = i, d
		}
	}
	normal := u.Cross(pts[i2].Sub(p0))
	i3, best := 0, -1.0
	for i, p := range pts {
		if d := math.Abs(normal.Dot(p.Sub(p0))); d > best {
			i3, best = i, d
		}
	}

	s := [4]int32{0, int32(i1), int32(i2), int32(i3)}
	switch orient(pts[s[0]], pts[s[1]], pts[s[2]], pts[s[3]]) {
	case 0:
		// The point farthest from the plane is on it. Either every
		// point is, or two of the chosen points coincide.
		if s[1] == s[0] || pts[s[1]] == pts[s[0]] {
			return s, fmt.Errorf("%w: every point equals point 0", ErrDuplicatePoint)
		}
		return s, ErrCoplanar
	case -1:
		s[1], s[2] = s[2], s[1]
	}
	// Now point s[3] is on the inner side of triangle s[0], s[1], s[2].
	faces := [4][3]int32{
		{s[0], s[1], s[2]},
		{s[0], s[3], s[1]},
		{s[1], s[3], s[2]},
		{s[0], s[2], s[3]},
	}
	b.centre = pts[s[0]].Add(pts[s[1]]).Add(pts[s[2]]).Add(pts[s[3]]).Scale(0.25)
	for _, f := range faces {
		if orient(pts[f[0]], pts[f[1]], pts[f[2]], b.centre) <= 0 {
			// The tetrahedron is so thin that its rounded centroid is
			// not inside it.
			return s, ErrCoplanar
		}
		b.tri = append(b.tri, f[0], f[1], f[2])
		b.twin = append(b.twin, -1, -1, -1)
	}
	for e := range int32(12) {
		for f := range int32(12) {
			if b.tri[e] == b.tri[next(f)] && b.tri[next(e)] == b.tri[f] {
				b.twin[e] = f
			}
		}
	}
	return s, nil
}

// insertionOrder returns every point except the seed points, sorted so
// that consecutive points are close together: the sphere is cut into
// bands of equal area by planes of constant z, and the bands are walked
// in turn, eastward and westward alternately.
func insertionOrder(points []Vec, seed [4]int32) []int32 {
	type entry struct {
		key   float64
		index int32
	}
	// About 2√n points per band makes a band about one point thick.
	bands := max(1, int(math.Sqrt(float64(len(points)))/2))
	entries := make([]entry, 0, len(points))
	for i, p := range points {
		if slices.Contains(seed[:], int32(i)) {
			continue
		}
		band := min(bands-1, max(0, int((1-p.Z)/2*float64(bands))))
		lon := (math.Atan2(p.Y, p.X) + math.Pi) / (2 * math.Pi) // in [0, 1]
		if band%2 == 1 {
			lon = 1 - lon
		}
		entries = append(entries, entry{float64(band) + lon/2, int32(i)})
	}
	slices.SortFunc(entries, func(a, b entry) int {
		return cmp.Or(cmp.Compare(a.key, b.key), cmp.Compare(a.index, b.index))
	})
	order := make([]int32, len(entries))
	for i, e := range entries {
		order[i] = e.index
	}
	return order
}

// locate returns a triangle that the ray from the hull's centre through p
// passes through. It walks from the hint triangle, at each step crossing
// an edge that has p on its far side.
func (b *builder) locate(p Vec) int32 {
	t := b.hint
	for range len(b.tri)/3 + 1 {
		e := 3 * t
		p0, p1, p2 := b.pts[b.tri[e]], b.pts[b.tri[e+1]], b.pts[b.tri[e+2]]
		switch {
		case orient(p0, p1, p, b.centre) < 0:
			t = b.twin[e] / 3
		case orient(p1, p2, p, b.centre) < 0:
			t = b.twin[e+1] / 3
		case orient(p2, p0, p, b.centre) < 0:
			t = b.twin[e+2] / 3
		default:
			return t
		}
	}
	// The walk did not arrive. With exact tests on a convex hull it
	// always does, so this is a safeguard: test every triangle.
	for e := int32(0); e < int32(len(b.tri)); e += 3 {
		p0, p1, p2 := b.pts[b.tri[e]], b.pts[b.tri[e+1]], b.pts[b.tri[e+2]]
		if orient(p0, p1, p, b.centre) >= 0 && orient(p1, p2, p, b.centre) >= 0 && orient(p2, p0, p, b.centre) >= 0 {
			return e / 3
		}
	}
	panic("sphere: no hull face lies under the point")
}

// insert adds point p to the hull.
func (b *builder) insert(p int32) error {
	t := b.locate(b.pts[p])
	e0 := 3 * t
	i0, i1, i2 := b.tri[e0], b.tri[e0+1], b.tri[e0+2]
	for _, i := range [3]int32{i0, i1, i2} {
		if b.pts[i] == b.pts[p] {
			return fmt.Errorf("%w: points %d and %d", ErrDuplicatePoint, min(i, p), max(i, p))
		}
	}

	// Replace triangle (i0, i1, i2) with (i0, i1, p), (i1, i2, p) and
	// (i2, i0, p). The first reuses the old triangle's slot.
	h1, h2 := b.twin[e0+1], b.twin[e0+2]
	e1 := int32(len(b.tri))
	e2 := e1 + 3
	b.tri[e0+2] = p
	b.tri = append(b.tri, i1, i2, p, i2, i0, p)
	b.twin = append(b.twin, -1, -1, -1, -1, -1, -1)
	b.link(e1, h1)
	b.link(e2, h2)
	b.link(e0+1, e1+2)
	b.link(e1+1, e2+2)
	b.link(e2+1, e0+2)
	b.hint = t // flips keep p in this triangle

	b.legalize(e0)
	b.legalize(e1)
	b.legalize(e2)
	return nil
}

// legalize restores convexity across half-edge a, whose triangle has the
// newly inserted point as the corner opposite a, and across every edge
// that a flip exposes.
func (b *builder) legalize(a int32) {
	tri, twin := b.tri, b.twin
	stack := b.stack[:0]
	for {
		// Triangle A is (p0, pr, pl) with half-edge a from pr to pl.
		// Triangle B, on the other side, is (pl, pr, p1).
		c := twin[a]
		ar, al := prev(a), next(a)
		cl := prev(c)
		p0, pr, pl, p1 := tri[ar], tri[a], tri[al], tri[cl]

		// The edge is concave if the new point p0 is on the outer side
		// of B. On the sphere, that is p0 inside B's circumcircle.
		if orient(b.pts[pl], b.pts[pr], b.pts[p1], b.pts[p0]) < 0 {
			// Flip: A becomes (p0, p1, pl) and B becomes (p0, pr, p1).
			tri[a] = p1
			tri[c] = p0
			hcl, har := twin[cl], twin[ar]
			b.link(a, hcl)
			b.link(c, har)
			b.link(ar, cl)
			// Both new triangles have an edge opposite p0 to check:
			// a, again, and the one after c.
			stack = append(stack, next(c))
			continue
		}
		if len(stack) == 0 {
			break
		}
		a = stack[len(stack)-1]
		stack = stack[:len(stack)-1]
	}
	b.stack = stack
}

// neighbours returns each point's neighbours as an offsets slice and an
// indices slice; see Mesh.NbrOff.
func (b *builder) neighbours() (off, nbr []int32) {
	n := len(b.pts)
	// Each edge is two half-edges, one leaving each end, so a point has
	// as many neighbours as half-edges that leave it.
	off = make([]int32, n+1)
	leaving := make([]int32, n) // one half-edge that leaves each point
	for e, p := range b.tri {
		off[p+1]++
		leaving[p] = int32(e)
	}
	for i := range n {
		off[i+1] += off[i]
	}
	nbr = make([]int32, len(b.tri))
	for i := range n {
		list := nbr[off[i]:off[i+1]]
		// Turn about the point: from half-edge i→v in triangle (i, v, w),
		// the next one counter-clockwise is i→w, the twin of w→i.
		e := leaving[i]
		lowest := 0
		for k := range list {
			list[k] = b.tri[next(e)]
			if list[k] < list[lowest] {
				lowest = k
			}
			e = b.twin[prev(e)]
		}
		// Start the list at the lowest index.
		rotateLeft(list, lowest)
	}
	return off, nbr
}

// rotateLeft rotates s so that s[k] becomes s[0].
func rotateLeft(s []int32, k int) {
	slices.Reverse(s[:k])
	slices.Reverse(s[k:])
	slices.Reverse(s)
}
