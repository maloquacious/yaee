// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import (
	"fmt"
	"math"

	"github.com/maloquacious/yaee/internal/sphere"
)

// Locator finds, for any direction from the centre of a World's sphere,
// the triangle of samples that contains it and the sample nearest to
// it. A client uses it to read the world at points of its own choosing:
// the cells of a map, of any shape and projection.
//
// A direction is a vector (x, y, z) on the axes of World.Positions. It
// need not have length 1. It is invalid if any component is NaN or
// infinite, or if all three are 0.
//
// A Locator is not changed after NewLocator returns it, and its methods
// may be called from many goroutines at once. It keeps its own copy of
// the world's positions and triangles: changing the World afterwards
// does not affect it.
type Locator struct {
	loc *sphere.Locator
}

// NewLocator builds a Locator for w. The time and memory are
// proportional to the number of samples: about 0.3 seconds and 95 MB
// for the default world of 942,059 samples. A call of Locate or Nearest
// then takes under a microsecond.
//
// w must be a World that Generate or Read returned. NewLocator returns
// an error wrapping ErrInvalidWorld if w's triangles are not a closed
// surface over its samples. It does not check that they are the
// Delaunay triangulation that Generate makes; if they are not, Nearest
// may return a sample that is not the nearest.
func NewLocator(w *World) (*Locator, error) {
	if w == nil {
		return nil, fmt.Errorf("%w: nil world", ErrInvalidWorld)
	}
	points := make([]sphere.Vec, len(w.Positions))
	for i, p := range w.Positions {
		points[i] = sphere.Vec{X: p[0], Y: p[1], Z: p[2]}
	}
	tri := make([]int32, 0, 3*len(w.Triangles))
	for _, t := range w.Triangles {
		tri = append(tri, t[0], t[1], t[2])
	}
	mesh, err := sphere.NewMesh(points, tri)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidWorld, err)
	}
	return &Locator{loc: sphere.NewLocator(mesh)}, nil
}

// Locate returns the triangle that contains the direction (x, y, z), as
// an index into World.Triangles, and the direction's weights for that
// triangle's three corners.
//
// weights[k] belongs to the sample World.Triangles[triangle][k]. Each
// weight is from 0 to 1 and the three sum to 1. To interpolate a
// per-sample value at the direction, take the weighted sum of its
// values at the three corners:
//
//	t, wt := loc.Locate(x, y, z)
//	c := world.Triangles[t]
//	v := wt[0]*value[c[0]] + wt[1]*value[c[1]] + wt[2]*value[c[2]]
//
// The weights are barycentric: with a, b, and c the positions of the
// corners, wt[0]·a + wt[1]·b + wt[2]·c is the point where the ray from
// the centre in the direction crosses the flat triangle a, b, c. At a
// sample's own position the sample has weight 1.
//
// A direction exactly on an edge or at a corner lies in more than one
// triangle; Locate returns the one with the lowest index.
//
// Interpolate only values that vary continuously, such as elevation.
// For a value that names a kind, such as crust_type, orogeny_type, or
// plate, use Nearest. Elevation steps at a coast: interpolating between
// samples of different crust_type gives heights that neither has.
//
// For an invalid direction Locate returns −1 and zero weights.
func (l *Locator) Locate(x, y, z float64) (triangle int, weights [3]float64) {
	p, ok := direction(x, y, z)
	if !ok {
		return -1, weights
	}
	t, weights := l.loc.Locate(p)
	return int(t), weights
}

// Nearest returns the index of the sample nearest to the direction
// (x, y, z): the one whose position makes the smallest angle with it.
// Of samples at equal angles it returns the lowest index.
//
// For an invalid direction Nearest returns −1.
func (l *Locator) Nearest(x, y, z float64) int {
	p, ok := direction(x, y, z)
	if !ok {
		return -1
	}
	return int(l.loc.Nearest(p))
}

// direction returns (x, y, z) scaled to length 1, and whether it is a
// valid direction. Scaling by the largest component first keeps a very
// long or very short vector from overflowing or vanishing.
func direction(x, y, z float64) (sphere.Vec, bool) {
	m := max(math.Abs(x), math.Abs(y), math.Abs(z))
	if !(m > 0) || math.IsInf(m, 0) || math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(z) {
		return sphere.Vec{}, false
	}
	x, y, z = x/m, y/m, z/m
	n := math.Sqrt(x*x + y*y + z*z)
	return sphere.Vec{X: x / n, Y: y / n, Z: z / n}, true
}
