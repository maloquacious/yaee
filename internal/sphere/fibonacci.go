// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package sphere

import "math"

// SampleCount returns the number of samples that covers a sphere of the
// given radius with the given spacing between neighbouring samples. Both
// are in kilometres and must be positive and finite.
//
//	N = round(8πR² / (√3 · d²))
//
// Each of N evenly spread samples owns an area of 4πR²/N, and d is the
// edge of the equilateral triangle lattice with that area per point. For
// R = 6370 and d = 25 the result is 942,059.
func SampleCount(radius, spacing float64) int {
	return int(math.Round(8 * math.Pi * radius * radius / (math.Sqrt(3) * spacing * spacing)))
}

// SampleSpacing is the inverse of SampleCount: the spacing in kilometres
// between neighbouring samples when n samples cover a sphere of the given
// radius in kilometres.
//
//	d = √(8πR² / (√3 · N))
func SampleSpacing(radius float64, n int) float64 {
	return math.Sqrt(8 * math.Pi * radius * radius / (math.Sqrt(3) * float64(n)))
}

// invPhi is 1/φ = φ − 1 = (√5 − 1)/2, where φ is the golden ratio,
// rounded to the nearest float64.
const invPhi = 0.6180339887498949

// FibonacciPoint returns the position of sample i of an n-sample
// spherical Fibonacci point set, as a unit vector. i must be in [0, n).
//
// This is the sampling of the paper's §6 (Keinert et al., Spherical
// Fibonacci Mapping, 2015). The position depends only on i and n, so
// anything that knows n can recompute every position:
//
//	t = (2i + 1) / n
//	z = 1 − t
//	r = √(t · (2 − t))
//	f = i · 0.6180339887498949
//	θ = 2π · (f − floor(f))
//	position = (r · cos θ, r · sin θ, z)
//
// All arithmetic is in IEEE 754 double precision. r equals √(1 − z²),
// written so that it stays accurate near the poles. Sample 0 is nearest
// the +z pole and sample n−1 is nearest the −z pole.
func FibonacciPoint(i, n int) Vec {
	t := float64(2*i+1) / float64(n)
	z := 1 - t
	r := math.Sqrt(t * (2 - t))
	f := float64(i) * invPhi
	sin, cos := math.Sincos(2 * math.Pi * (f - math.Floor(f)))
	return Vec{r * cos, r * sin, z}
}

// FibonacciPoints returns the positions of all n samples: element i is
// FibonacciPoint(i, n).
func FibonacciPoints(n int) []Vec {
	points := make([]Vec, n)
	for i := range points {
		points[i] = FibonacciPoint(i, n)
	}
	return points
}
