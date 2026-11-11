// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import (
	"math"
	"math/big"
)

// The two predicates in this file return the exact sign of a determinant
// of float64 coordinates. Each first evaluates the determinant in
// floating point and accepts the sign when the magnitude exceeds a bound
// on the rounding error (the static filter of Shewchuk, Adaptive
// Precision Floating-Point Arithmetic and Fast Robust Geometric
// Predicates, 1997). Otherwise it repeats the calculation in exact
// rational arithmetic. Inputs must be finite.

const (
	// filterRel bounds the relative rounding error of the floating point
	// determinants. Shewchuk's bound for the 3×3 determinant of
	// differences is (7 + 56ε)ε with ε = 2⁻⁵³; 8ε is used to leave a
	// margin.
	filterRel = 8.0 / (1 << 53)

	// filterAbs covers products that underflow, which the relative bound
	// does not.
	filterAbs = 1e-290
)

// orient returns the sign of the determinant whose rows are a−d, b−d and
// c−d: +1, 0 or −1.
//
// When a, b and c are in counter-clockwise order seen from outside the
// sphere, the result is +1 if d is on the inner side of the plane through
// them (the side of the sphere's centre), −1 if d is on the outer side,
// and 0 if d is on the plane. For four points on the sphere this is also
// the in-circle test: −1 means d is strictly inside the circumcircle of
// the triangle a, b, c.
func orient(a, b, c, d Vec) int {
	adx, ady, adz := a.X-d.X, a.Y-d.Y, a.Z-d.Z
	bdx, bdy, bdz := b.X-d.X, b.Y-d.Y, b.Z-d.Z
	cdx, cdy, cdz := c.X-d.X, c.Y-d.Y, c.Z-d.Z

	bdxcdy, cdxbdy := bdx*cdy, cdx*bdy
	cdxady, adxcdy := cdx*ady, adx*cdy
	adxbdy, bdxady := adx*bdy, bdx*ady

	det := adz*(bdxcdy-cdxbdy) + bdz*(cdxady-adxcdy) + cdz*(adxbdy-bdxady)
	permanent := (math.Abs(bdxcdy)+math.Abs(cdxbdy))*math.Abs(adz) +
		(math.Abs(cdxady)+math.Abs(adxcdy))*math.Abs(bdz) +
		(math.Abs(adxbdy)+math.Abs(bdxady))*math.Abs(cdz)
	bound := filterRel*permanent + filterAbs
	if det > bound {
		return 1
	}
	if det < -bound {
		return -1
	}
	return orientExact(a, b, c, d)
}

// det3 returns the sign of the determinant whose rows are a, b and c,
// which is a · (b × c): +1, 0 or −1. It equals orient(a, b, c, origin).
//
// For a point p and an edge from a to b of a triangle wound
// counter-clockwise seen from outside, det3(a, b, p) is +1 if p is on the
// triangle's side of the great circle through a and b, −1 if it is on the
// other side, and 0 if it is on the great circle.
func det3(a, b, c Vec) int {
	// A repeated row makes the determinant exactly zero. This is the case
	// when a sample's own position is located, so skip the exact path.
	if a == b || b == c || a == c {
		return 0
	}
	bxcy, cxby := b.X*c.Y, c.X*b.Y
	cxay, axcy := c.X*a.Y, a.X*c.Y
	axby, bxay := a.X*b.Y, b.X*a.Y

	det := a.Z*(bxcy-cxby) + b.Z*(cxay-axcy) + c.Z*(axby-bxay)
	permanent := (math.Abs(bxcy)+math.Abs(cxby))*math.Abs(a.Z) +
		(math.Abs(cxay)+math.Abs(axcy))*math.Abs(b.Z) +
		(math.Abs(axby)+math.Abs(bxay))*math.Abs(c.Z)
	bound := filterRel*permanent + filterAbs
	if det > bound {
		return 1
	}
	if det < -bound {
		return -1
	}
	return orientExact(a, b, c, Vec{})
}

// orientExact is orient in exact rational arithmetic.
func orientExact(a, b, c, d Vec) int {
	row := func(p Vec) [3]*big.Rat {
		sub := func(x, y float64) *big.Rat {
			r := new(big.Rat).SetFloat64(x)
			return r.Sub(r, new(big.Rat).SetFloat64(y))
		}
		return [3]*big.Rat{sub(p.X, d.X), sub(p.Y, d.Y), sub(p.Z, d.Z)}
	}
	ra, rb, rc := row(a), row(b), row(c)
	// minor returns x[i]·y[j] − x[j]·y[i].
	minor := func(x, y [3]*big.Rat, i, j int) *big.Rat {
		m := new(big.Rat).Mul(x[i], y[j])
		return m.Sub(m, new(big.Rat).Mul(x[j], y[i]))
	}
	det := new(big.Rat).Mul(ra[2], minor(rb, rc, 0, 1))
	det.Add(det, new(big.Rat).Mul(rb[2], minor(rc, ra, 0, 1)))
	det.Add(det, new(big.Rat).Mul(rc[2], minor(ra, rb, 0, 1)))
	return det.Sign()
}
