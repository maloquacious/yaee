// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package tectonics holds the stages that build and move the plates of
// a world, after the model of Cortial et al., Procedural Tectonic
// Planets (2019). Section references (§) are to that paper.
//
// Units are kilometres and millions of years (My).
package tectonics

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/noise"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Constants of the initial plates. The paper gives none of them (§3,
// Workflow: "distributing a user-prescribed number of plate centroids
// over the sphere", "warping the geodetic distances to the centroids
// using a simple noise function").
const (
	// centroidSeparation is the smallest distance allowed between two
	// plate centroids, as a fraction of √(4πR²/P), the side of the
	// square whose area is the mean area of the P plates. 0.5 keeps
	// centroids from landing close together, which gives sliver plates,
	// while leaving plate areas unequal: closely packed centroids would
	// need 1.07, and random placement stalls near 0.83.
	centroidSeparation = 0.5

	// centroidTries is the number of consecutive rejected positions
	// after which the smallest distance is reduced by centroidRelax, so
	// that placement always ends.
	centroidTries = 100
	centroidRelax = 0.9

	// The warp is fractal noise of warpOctaves octaves, each with
	// warpLacunarity times the frequency and warpGain times the
	// amplitude of the one before. The first octave has warpFirst of the
	// warp's amplitude and bends a boundary at the scale of a plate; the
	// rest roughen it down to 1/128 of that, 20 km with the default
	// feature size, which is below the default sample spacing.
	//
	// The gain is above the usual 0.5 so that boundaries are irregular at
	// every scale. Continents break along the initial plate boundaries
	// when the plates first part, and those breaks become coasts: with
	// four octaves and a gain of 0.5 they were straight lines hundreds of
	// km long. warpFirst is the share the first octave had then, 8/15, so
	// the large bends are as they were.
	warpOctaves    = 8
	warpLacunarity = 2.0
	warpGain       = 0.7
	warpFirst      = 8.0 / 15
)

// Plates is the stage that divides the sphere into the initial plates
// (§3, Workflow). It sets World.NumPlates to Params.Plates and
// World.Plate for every sample.
//
// Method:
//
//  1. Centroids. Plate k's centroid is a point drawn uniformly from the
//     sphere, drawn again if it is closer than centroidSeparation ·
//     √(4πR²/P) km to an earlier centroid.
//  2. Warping. Sample i at unit position p is moved to
//     q = p + (A/R) · (n₁(f·p), n₂(f·p), n₃(f·p)), where A is
//     PlateWarpAmplitude, f = noise.Frequency(R, PlateWarpFeatureSize),
//     and n₁, n₂, n₃ are three independent sums of warpOctaves octaves
//     of noise: octave k, from 0, is noise in [−1, 1] at frequency
//     warpLacunarityᵏ, multiplied by warpFirst · warpGainᵏ. A point
//     moves about 0.41 · A km along the surface (root mean square), and
//     never more than 2.91 · A km.
//  3. Assignment. Sample i goes to the plate whose centroid is nearest
//     to q, that is, with the greatest q · c; the lowest-numbered plate
//     wins a tie. This is the spherical Voronoi partition of the warped
//     positions, which is how the paper's "warping the geodetic
//     distances to the centroids" is carried out: every centroid sees
//     the same moved point, so the plates still cover the sphere without
//     overlapping.
//  4. Seeds. Plate k's seed is its sample with the greatest q · c. A
//     plate that received no sample takes, as its only sample and seed,
//     the sample nearest its centroid by unwarped position that is not
//     already a seed.
//  5. Connection. Warping can cut a piece of a plate off. A plate keeps
//     only the samples connected to its seed through neighbours of the
//     same plate. Every other sample is given to the plate of the
//     nearest kept sample, counted in steps between neighbours, working
//     outward from the kept samples in order of sample number.
//
// The result is that every sample has one plate, every plate has at
// least one sample, and every plate is one connected region. With
// PlateWarpAmplitude 0, step 2 does nothing and each sample belongs to
// the plate with the nearest centroid, except for a sample that step 5
// moves; that happens only where a Voronoi cell narrows to less than
// the sample spacing.
//
// Centroids and noise are functions of position, not of the samples, so
// the same seed gives nearly the same plates at any SampleSpacing.
//
// Params.Plates must be from 1 to the number of samples.
type Plates struct{}

// Name returns "plates".
func (Plates) Name() string { return "plates" }

// Run applies the stage.
func (Plates) Run(ctx context.Context, w *model.World, rng *rand.Rand) error {
	centroids := placeCentroids(rng, w.Plates)
	warp := newWarp(rng, w.Radius, w.PlateWarpAmplitude, w.PlateWarpFeatureSize)
	return partition(ctx, w, centroids, warp)
}

// placeCentroids draws n plate centroids. See step 1 of Plates.
func placeCentroids(rng *rand.Rand, n int) []sphere.Vec {
	centroids := make([]sphere.Vec, 0, n)
	separation := centroidSeparation * math.Sqrt(4*math.Pi/float64(n)) // radians
	minCos := math.Cos(min(separation, math.Pi))
	rejected := 0
	for len(centroids) < n {
		z := 1 - 2*rng.Float64()
		r := math.Sqrt(max(0, 1-z*z))
		sin, cos := math.Sincos(2 * math.Pi * rng.Float64())
		c := sphere.Vec{X: r * cos, Y: r * sin, Z: z}

		ok := true
		for _, o := range centroids {
			if c.Dot(o) > minCos {
				ok = false
				break
			}
		}
		if !ok {
			if rejected++; rejected == centroidTries {
				rejected = 0
				separation *= centroidRelax
				minCos = math.Cos(separation)
			}
			continue
		}
		rejected = 0
		centroids = append(centroids, c)
	}
	return centroids
}

// warp moves positions before they are compared with the centroids. See
// step 2 of Plates. The zero value moves nothing.
type warp struct {
	scale float64 // amplitude / radius; 0 turns the warp off
	freq  float64
	n     [3]*noise.Fractal
}

// newWarp draws the three noise sources from rng. It draws the same
// numbers whatever the amplitude, so that turning the warp off does not
// change anything drawn after it.
func newWarp(rng *rand.Rand, radius, amplitude, featureSize float64) warp {
	// A Fractal divides by the sum of its octaves' amplitudes; multiply
	// by it again so that the first octave has warpFirst of the amplitude.
	sum := (1 - math.Pow(warpGain, warpOctaves)) / (1 - warpGain)
	wp := warp{scale: amplitude / radius * warpFirst * sum, freq: noise.Frequency(radius, featureSize)}
	for k := range wp.n {
		wp.n[k] = noise.NewFractal(rng.Uint64(), warpOctaves, warpLacunarity, warpGain)
	}
	return wp
}

// apply returns the moved position of p. It does not have length 1.
func (wp *warp) apply(p sphere.Vec) sphere.Vec {
	if wp.scale == 0 {
		return p
	}
	x, y, z := p.X*wp.freq, p.Y*wp.freq, p.Z*wp.freq
	return sphere.Vec{
		X: p.X + wp.scale*wp.n[0].At(x, y, z),
		Y: p.Y + wp.scale*wp.n[1].At(x, y, z),
		Z: p.Z + wp.scale*wp.n[2].At(x, y, z),
	}
}

// partition carries out steps 3 to 5 of Plates.
func partition(ctx context.Context, w *model.World, centroids []sphere.Vec, wp warp) error {
	mesh := w.Mesh
	n := len(mesh.Points)
	plate := w.Plate

	// Step 3. best[i] is q · c for the chosen centroid, divided by |q|
	// so that it can be compared between samples.
	best := make([]float64, n)
	err := model.ParallelRange(ctx, n, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			q := wp.apply(mesh.Points[i])
			k, d := 0, q.Dot(centroids[0])
			for j := 1; j < len(centroids); j++ {
				if dj := q.Dot(centroids[j]); dj > d {
					k, d = j, dj
				}
			}
			plate[i] = int32(k)
			best[i] = d / q.Len()
		}
	})
	if err != nil {
		return err
	}

	// Step 4.
	seed := make([]int32, len(centroids))
	for k := range seed {
		seed[k] = -1
	}
	for i := range n {
		if k := plate[i]; seed[k] < 0 || best[i] > best[seed[k]] {
			seed[k] = int32(i)
		}
	}
	isSeed := make([]bool, n)
	for _, s := range seed {
		if s >= 0 {
			isSeed[s] = true
		}
	}
	for k, c := range centroids {
		if seed[k] >= 0 {
			continue
		}
		s, d := -1, math.Inf(-1)
		for i, p := range mesh.Points {
			if !isSeed[i] {
				if di := p.Dot(c); di > d {
					s, d = i, di
				}
			}
		}
		seed[k], isSeed[s], plate[s] = int32(s), true, int32(k)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	connect(mesh, plate, seed)
	w.NumPlates = len(centroids)
	return ctx.Err()
}

// connect carries out step 5 of Plates. seed[k] is a sample of plate k.
// Every sample that is not joined to its plate's seed through
// neighbours of the same plate is moved to another plate, so that each
// plate becomes one connected region.
func connect(mesh *sphere.Mesh, plate, seed []int32) {
	// Mark what each seed reaches through its own plate.
	n := len(mesh.Points)
	kept := make([]bool, n)
	queue := make([]int32, 0, n)
	for _, s := range seed {
		kept[s] = true
		queue = append(queue, s)
	}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		for _, j := range mesh.Neighbours(int(i)) {
			if !kept[j] && plate[j] == plate[i] {
				kept[j] = true
				queue = append(queue, j)
			}
		}
	}
	if len(queue) == n {
		return
	}

	// Grow the kept samples over the rest, nearest first.
	queue = queue[:0]
	for i := range n {
		if kept[i] {
			queue = append(queue, int32(i))
		}
	}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		for _, j := range mesh.Neighbours(int(i)) {
			if !kept[j] {
				kept[j] = true
				plate[j] = plate[i]
				queue = append(queue, j)
			}
		}
	}
}
