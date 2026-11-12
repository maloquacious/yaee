// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package rocks is the stage that gives every sample a rock type.
//
// Rock type is not in Procedural Tectonic Planets. It is derived from
// the crust that the tectonic stages leave, so that a client can place
// minerals by it. Units are kilometres.
package rocks

import (
	"context"
	"math"
	"math/rand/v2"
	"sync"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/noise"
)

// Parameters of the rock noise that are not in Params. They are the
// elevation noise's, which have not needed tuning.
const (
	noiseOctaves    = 3
	noiseLacunarity = 2.0
	noiseGain       = 0.5
)

// classes are the rock types the stage assigns, in the order their
// noise seeds are drawn.
var classes = [3]uint8{model.RockIgneous, model.RockMetamorphic, model.RockSedimentary}

// RockType is the stage that sets World.RockType. It must run after the
// tectonic stages; it changes nothing else.
//
// Each sample first gets a base class from its crust, by the first rule
// that applies, with U the parameter RockArcUplift and M
// RockOrogenyMaxAge:
//
//	oceanic crust                                  igneous
//	continental, uplift > U                        igneous     (active arc)
//	continental, OrogenyAndean, orogeny age ≤ M    igneous     (volcanic arc)
//	continental, OrogenyHimalayan, orogeny age ≤ M metamorphic (suture)
//	any other continental crust                    sedimentary
//
// An orogeny older than M counts as none: the belt is taken to be worn
// down and covered by sediment. The uplift rule finds the crust behind a
// subduction front that is being raised now, which is often Himalayan or
// older Andean: the orogeny type records only the first orogeny of crust
// above sea level (see model.World.OrogenyType).
//
// Oceanic samples keep their base class. A continental sample i at the
// position p then takes the class c with the greatest score
//
//	S_c(i) = −min(d_c(i), D) + A · F_c(p · R / s)
//
// where d_c(i) is the distance in km to the nearest continental sample
// whose base class is c (0 for c = i's own base class, and taken to be D
// when there is none), D is the parameter RockReach, A is
// RockNoiseAmplitude, s is RockNoiseFeatureSize, R is the radius, and F_c
// is a noise.Fractal of 3 octaves with lacunarity 2 and gain 0.5, one
// for each class: values from −1 to 1 whose largest features are s km
// across. Ties go to the lower class value.
//
// So the edge between two classes moves by up to 2A km either way, and a
// sample farther than D from every other class changes class only where
// A · (F_c − F_own) > D: never if 2A ≤ D. With A = 0 every sample keeps
// its base class. Only continental samples are sources, so oceanic crust
// does not make the continental crust of a coast igneous. Distances are
// great-circle distances, found by sphere.Mesh.Distances.
//
// The scheme departs from ProcGenesis WorldGen's (grift ALGORITHMS.md
// §7.2), which classifies by plate boundary stress from random plate
// velocities; here the tectonic simulation's orogeny records where the
// stress was.
//
// The stage draws three numbers from its random stream in every case.
type RockType struct{}

// Name returns "rock-type".
func (RockType) Name() string { return "rock-type" }

// Run applies the stage.
func (RockType) Run(ctx context.Context, w *model.World, rng *rand.Rand) error {
	var seeds [len(classes)]uint64
	for k := range seeds {
		seeds[k] = rng.Uint64()
	}
	for i := range w.RockType {
		w.RockType[i] = base(w, i)
	}
	if w.RockNoiseAmplitude == 0 {
		return ctx.Err()
	}

	// The distances to each class are independent: each is computed by
	// one goroutine into its own slice.
	var (
		dist [len(classes)][]float64
		wg   sync.WaitGroup
	)
	for k, c := range classes {
		wg.Go(func() { dist[k] = distances(w, c) })
	}
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}

	var fields [len(classes)]*noise.Fractal
	for k := range fields {
		fields[k] = noise.NewFractal(seeds[k], noiseOctaves, noiseLacunarity, noiseGain)
	}
	freq := noise.Frequency(w.Radius, w.RockNoiseFeatureSize)
	return model.ParallelRange(ctx, len(w.RockType), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if w.CrustType[i] != model.Continental {
				continue
			}
			p := w.Mesh.Points[i]
			best, bestScore := uint8(0), math.Inf(-1)
			for k, c := range classes {
				s := -dist[k][i] + w.RockNoiseAmplitude*fields[k].At(p.X*freq, p.Y*freq, p.Z*freq)
				if s > bestScore {
					best, bestScore = c, s
				}
			}
			w.RockType[i] = best
		}
	})
}

// base returns the rock type of sample i before any noise.
func base(w *model.World, i int) uint8 {
	young := w.OrogenyAge[i] <= w.RockOrogenyMaxAge
	switch {
	case w.CrustType[i] != model.Continental:
		return model.RockIgneous
	case w.Uplift[i] > w.RockArcUplift:
		return model.RockIgneous
	case w.OrogenyType[i] == model.OrogenyAndean && young:
		return model.RockIgneous
	case w.OrogenyType[i] == model.OrogenyHimalayan && young:
		return model.RockMetamorphic
	default:
		return model.RockSedimentary
	}
}

// distances returns, for every sample, min(d, RockReach) where d is the
// distance in km to the nearest continental sample whose rock type is c.
// It is RockReach for every sample if there is none.
func distances(w *model.World, c uint8) []float64 {
	var sources []int32
	for i, crust := range w.CrustType {
		if crust == model.Continental && w.RockType[i] == c {
			sources = append(sources, int32(i))
		}
	}
	d, _ := w.Mesh.Distances(sources)
	for i := range d {
		d[i] = min(d[i]*w.Radius, w.RockReach) // Inf with no source
	}
	return d
}

// Compile-time check that the stage satisfies the interface.
var _ model.Stage = RockType{}
