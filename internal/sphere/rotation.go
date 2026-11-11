// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// Rotation is a rotation about an axis through the centre of the sphere.
// It is stored as a unit quaternion.
//
// The zero value is not a rotation. Use Identity for the rotation that
// moves nothing.
type Rotation struct {
	w float64 // cos(angle/2)
	v Vec     // axis · sin(angle/2)
}

// Identity returns the rotation that leaves every vector unchanged.
func Identity() Rotation { return Rotation{w: 1} }

// NewRotation returns the rotation by angle radians about axis. axis must
// have length 1. Seen from the tip of axis looking toward the centre, a
// positive angle turns points counter-clockwise (the right-hand rule).
func NewRotation(axis Vec, angle float64) Rotation {
	s, c := math.Sincos(angle / 2)
	return Rotation{w: c, v: axis.Scale(s)}
}

// Apply returns a rotated by r. The length of a is kept.
func (r Rotation) Apply(a Vec) Vec {
	t := r.v.Cross(a).Scale(2)
	return a.Add(t.Scale(r.w)).Add(r.v.Cross(t))
}

// Then returns the rotation that applies r first and s second:
// r.Then(s).Apply(a) equals s.Apply(r.Apply(a)).
func (r Rotation) Then(s Rotation) Rotation {
	// The quaternion product s·r.
	q := Rotation{
		w: s.w*r.w - s.v.Dot(r.v),
		v: r.v.Scale(s.w).Add(s.v.Scale(r.w)).Add(s.v.Cross(r.v)),
	}
	// Renormalize so that rounding does not build up over many compositions.
	n := math.Sqrt(q.w*q.w + q.v.Dot(q.v))
	q.w /= n
	q.v = q.v.Scale(1 / n)
	return q
}

// Inverse returns the rotation that undoes r.
func (r Rotation) Inverse() Rotation { return Rotation{w: r.w, v: r.v.Scale(-1)} }
