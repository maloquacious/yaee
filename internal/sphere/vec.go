// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// Vec is a vector in three dimensions. A position on the sphere is a Vec
// of length 1.
type Vec struct {
	X, Y, Z float64
}

// Add returns a + b.
func (a Vec) Add(b Vec) Vec { return Vec{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }

// Sub returns a − b.
func (a Vec) Sub(b Vec) Vec { return Vec{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }

// Scale returns s·a.
func (a Vec) Scale(s float64) Vec { return Vec{a.X * s, a.Y * s, a.Z * s} }

// Dot returns the dot product a · b.
func (a Vec) Dot(b Vec) float64 { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }

// Cross returns the cross product a × b.
func (a Vec) Cross(b Vec) Vec {
	return Vec{
		a.Y*b.Z - a.Z*b.Y,
		a.Z*b.X - a.X*b.Z,
		a.X*b.Y - a.Y*b.X,
	}
}

// Len returns the length of a.
func (a Vec) Len() float64 { return math.Sqrt(a.Dot(a)) }

// Normalize returns a scaled to length 1. The zero vector is returned
// unchanged.
func (a Vec) Normalize() Vec {
	n := a.Len()
	if n == 0 {
		return a
	}
	return a.Scale(1 / n)
}

// Tangent returns the component of a that is perpendicular to p: the
// projection of a onto the plane tangent to the sphere at p. p must have
// length 1.
func (a Vec) Tangent(p Vec) Vec { return a.Sub(p.Scale(a.Dot(p))) }

// Angle returns the geodesic angle between a and b in radians, in [0, π].
// Neither vector needs to have length 1.
//
// It is computed as atan2(|a × b|, a · b), which keeps its precision for
// angles near 0 and near π, where acos(a · b) loses it.
func Angle(a, b Vec) float64 { return math.Atan2(a.Cross(b).Len(), a.Dot(b)) }
