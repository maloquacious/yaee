// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package noise provides seeded, continuous noise at points in three
// dimensions. Sampled at the positions of points on a sphere it has no
// seam and no pole, because it never sees a latitude or a longitude.
//
// Noise is one band of noise; Fractal is a sum of octaves of it. Both
// are fixed by their seed, hold no mutable state, and may be evaluated
// from many goroutines at once.
//
// # Which noise, and why
//
// The paper asks only for "a simple noise function" to warp plate
// boundaries (§3) and for "low frequency coherent noise" (§4.5), and
// names none. This package uses simplex gradient noise (Perlin 2001, in
// the formulation of Gustavson, Simplex noise demystified, 2005), with
// three changes:
//
//   - The kernel radius is √0.5, not the √0.6 of the usual code. With
//     √0.6 a corner's kernel reaches past its neighbouring simplices and
//     the noise has small jumps; with √0.5 it is continuous, with
//     continuous first, second and third derivatives.
//   - Gradients are hashed from the integer lattice coordinates and the
//     seed, not looked up in a 256-entry permutation. The noise therefore
//     does not repeat every 256 cells, which a sphere of radius 6370 km
//     sampled with 25 km features would otherwise reach.
//   - A gradient is one of 256 directions spread evenly over the sphere,
//     not one of the usual 12, so no axis is favoured.
//
// Simplex noise was chosen over classic Perlin noise because its lattice
// is not aligned with the coordinate axes, so its features do not line
// up along them. Fractal also turns each octave by its own random
// rotation, so octaves do not line up with each other.
//
// # Units
//
// The functions take plain coordinates. Coordinates are scaled so that
// features are 1 unit across, meaning that the values of a Noise at two
// points 1 unit apart are unrelated: their correlation falls from 1 at
// distance 0 to about 0.5 at distance 0.5, reaches zero at distance 1,
// and stays within 0.1 of zero beyond. To get features of a given size
// in km on a sphere, multiply the unit position by
// Frequency(radius, featureSize).
package noise

import (
	"math"
	"math/rand/v2"
)

// Frequency returns the factor by which to multiply a unit position on a
// sphere of the given radius so that noise evaluated there has features
// about featureSize across, measured along the surface. radius and
// featureSize are in the same unit, such as km, and must be positive and
// finite.
//
//	f := noise.Frequency(6370, 2000)
//	v := n.At(p.X*f, p.Y*f, p.Z*f) // features about 2000 km across
//
// The mean of a Noise or a Fractal over the whole sphere is not exactly
// zero, because the sphere holds a limited number of features. It is
// within 0.4 · featureSize/radius of zero.
func Frequency(radius, featureSize float64) float64 { return radius / featureSize }

// Noise is one band of simplex gradient noise. The zero value is the
// Noise with seed 0.
type Noise struct {
	seed uint64
}

// New returns the Noise for a seed. Equal seeds give equal values at
// every point; different seeds give unrelated noise.
func New(seed uint64) Noise { return Noise{seed: seed} }

// Constants of the simplex lattice in three dimensions.
const (
	skew   = 1.0 / 3.0 // F3: skews space onto the cubic lattice
	unskew = 1.0 / 6.0 // G3: and back

	// latticeScale converts the caller's coordinates, in which features
	// are 1 unit across, to lattice coordinates. It was chosen so that
	// the correlation between values first reaches zero at distance 1;
	// TestCorrelation checks it.
	latticeScale = 0.48

	// kernelRadius2 is the squared radius of each corner's kernel. 0.5
	// is the squared height of a simplex, the largest value for which a
	// kernel ends before the far face of every simplex it touches.
	kernelRadius2 = 0.5

	// amplitude scales the sum of the corner terms into [−1, 1]. A term
	// is (0.5 − r²)⁴ · (g · d) with |g| = 1 and |d| = r, so it is at most
	// (0.5 − r²)⁴ · r. The largest value of the sum of that bound over
	// every corner in reach, found by a search (TestAmplitudeBound), is
	// 0.009289. amplitude is a little less than its reciprocal, 107.65.
	amplitude = 107.0
)

// At returns the noise at (x, y, z), in [−1, 1]. The coordinates must be
// finite, and each must be less than 2⁵⁰ in magnitude.
// Each unit of distance is one feature size; see the package
// documentation.
//
// Values are spread about zero with a standard deviation of about 0.39.
func (n Noise) At(x, y, z float64) float64 {
	x, y, z = x*latticeScale, y*latticeScale, z*latticeScale

	// Skew to find the cube that holds the point, and unskew that
	// cube's origin to get the offset from the first corner.
	s := (x + y + z) * skew
	fi, fj, fk := math.Floor(x+s), math.Floor(y+s), math.Floor(z+s)
	t := (fi + fj + fk) * unskew
	x0, y0, z0 := x-(fi-t), y-(fj-t), z-(fk-t)

	// The cube is six simplices. The order of the offsets says which
	// one holds the point, and so which two corners lie between the
	// first and the last.
	var i1, j1, k1, i2, j2, k2 int64
	switch {
	case x0 >= y0 && y0 >= z0:
		i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 1, 0
	case x0 >= z0 && z0 >= y0:
		i1, j1, k1, i2, j2, k2 = 1, 0, 0, 1, 0, 1
	case z0 >= x0 && x0 >= y0:
		i1, j1, k1, i2, j2, k2 = 0, 0, 1, 1, 0, 1
	case z0 >= y0 && y0 >= x0:
		i1, j1, k1, i2, j2, k2 = 0, 0, 1, 0, 1, 1
	case y0 >= z0 && z0 >= x0:
		i1, j1, k1, i2, j2, k2 = 0, 1, 0, 0, 1, 1
	default: // y0 >= x0 && x0 >= z0
		i1, j1, k1, i2, j2, k2 = 0, 1, 0, 1, 1, 0
	}

	i, j, k := int64(fi), int64(fj), int64(fk)
	sum := n.corner(i, j, k, x0, y0, z0)
	sum += n.corner(i+i1, j+j1, k+k1,
		x0-float64(i1)+unskew, y0-float64(j1)+unskew, z0-float64(k1)+unskew)
	sum += n.corner(i+i2, j+j2, k+k2,
		x0-float64(i2)+2*unskew, y0-float64(j2)+2*unskew, z0-float64(k2)+2*unskew)
	sum += n.corner(i+1, j+1, k+1,
		x0-1+3*unskew, y0-1+3*unskew, z0-1+3*unskew)
	return amplitude * sum
}

// corner returns the term of the lattice corner (i, j, k) at the offset
// (dx, dy, dz) from it.
func (n Noise) corner(i, j, k int64, dx, dy, dz float64) float64 {
	t := kernelRadius2 - dx*dx - dy*dy - dz*dz
	if t <= 0 {
		return 0
	}
	g := &gradients[n.hash(i, j, k)>>56]
	t *= t
	return t * t * (g[0]*dx + g[1]*dy + g[2]*dz)
}

// hash mixes the lattice coordinates and the seed into 64 bits: a
// weighted sum, then the finalizer of SplitMix64.
func (n Noise) hash(i, j, k int64) uint64 {
	h := n.seed + uint64(i)*0x9E3779B97F4A7C15 + uint64(j)*0xC2B2AE3D27D4EB4F + uint64(k)*0x165667B19E3779F9
	h ^= h >> 30
	h *= 0xBF58476D1CE4E5B9
	h ^= h >> 27
	h *= 0x94D049BB133111EB
	h ^= h >> 31
	return h
}

// gradients holds 256 unit vectors spread evenly over the sphere: a
// spherical Fibonacci point set.
var gradients = func() (g [256][3]float64) {
	const invPhi = 0.6180339887498949
	for i := range g {
		z := 1 - float64(2*i+1)/float64(len(g))
		r := math.Sqrt(1 - z*z)
		f := float64(i) * invPhi
		sin, cos := math.Sincos(2 * math.Pi * (f - math.Floor(f)))
		g[i] = [3]float64{r * cos, r * sin, z}
	}
	return g
}()

// Fractal is a sum of octaves of Noise, each at a higher frequency and a
// lower amplitude than the one before.
type Fractal struct {
	octaves []octave
	norm    float64 // 1 / the sum of the octaves' amplitudes
}

// octave is one term of a Fractal: Noise evaluated at the point scaled
// by freq, turned by the rotation matrix rot, and moved by offset.
type octave struct {
	noise  Noise
	freq   float64
	amp    float64
	rot    [9]float64 // row-major
	offset [3]float64
}

// MaxOctaves is the largest number of octaves NewFractal accepts.
const MaxOctaves = 32

// NewFractal returns the Fractal for a seed. Equal arguments give equal
// values at every point.
//
// octaves is the number of terms, from 1 to MaxOctaves. Octave k
// (counting from 0) has frequency lacunarityᵏ and amplitude gainᵏ, so
// its features are 1/lacunarityᵏ across. lacunarity must be finite and
// at least 1; gain must be finite and greater than 0. The usual values
// are 2 and 0.5. NewFractal panics if an argument is out of range.
//
// Each octave has its own Noise, its own rotation of space, and its own
// offset, all drawn from the seed.
func NewFractal(seed uint64, octaves int, lacunarity, gain float64) *Fractal {
	if octaves < 1 || octaves > MaxOctaves {
		panic("noise: octaves out of range")
	}
	if !(lacunarity >= 1) || math.IsInf(lacunarity, 0) {
		panic("noise: lacunarity out of range")
	}
	if !(gain > 0) || math.IsInf(gain, 0) {
		panic("noise: gain out of range")
	}
	rng := rand.New(rand.NewPCG(seed, 0x6E6F697365)) // "noise"
	f := &Fractal{octaves: make([]octave, octaves)}
	freq, amp, total := 1.0, 1.0, 0.0
	for k := range f.octaves {
		o := &f.octaves[k]
		o.noise = New(rng.Uint64())
		o.freq, o.amp = freq, amp
		o.rot = randomRotation(rng)
		for c := range o.offset {
			o.offset[c] = rng.Float64() * 256
		}
		total += amp
		freq *= lacunarity
		amp *= gain
	}
	f.norm = 1 / total
	return f
}

// randomRotation returns a rotation matrix drawn uniformly from all
// rotations: that of a unit quaternion with normally distributed parts.
func randomRotation(rng *rand.Rand) [9]float64 {
	var w, x, y, z, n float64
	for n < 1e-12 {
		w, x, y, z = rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()
		n = math.Sqrt(w*w + x*x + y*y + z*z)
	}
	w, x, y, z = w/n, x/n, y/n, z/n
	return [9]float64{
		1 - 2*(y*y+z*z), 2 * (x*y - w*z), 2 * (x*z + w*y),
		2 * (x*y + w*z), 1 - 2*(x*x+z*z), 2 * (y*z - w*x),
		2 * (x*z - w*y), 2 * (y*z + w*x), 1 - 2*(x*x+y*y),
	}
}

// At returns the fractal noise at (x, y, z), in [−1, 1]: the sum of the
// octaves' values multiplied by their amplitudes, divided by the sum of
// the amplitudes. The coordinates must be finite, and each multiplied by
// the highest octave's frequency must be less than 2⁴⁸ in magnitude.
//
// With lacunarity 2 and gain 0.5 the standard deviation of the values is
// about 0.39 for one octave, 0.29 for two, 0.24 for four, and 0.22 for
// eight.
func (f *Fractal) At(x, y, z float64) float64 {
	var sum float64
	for k := range f.octaves {
		o := &f.octaves[k]
		px, py, pz := x*o.freq, y*o.freq, z*o.freq
		sum += o.amp * o.noise.At(
			o.rot[0]*px+o.rot[1]*py+o.rot[2]*pz+o.offset[0],
			o.rot[3]*px+o.rot[4]*py+o.rot[5]*pz+o.offset[1],
			o.rot[6]*px+o.rot[7]*py+o.rot[8]*pz+o.offset[2],
		)
	}
	return sum * f.norm
}
