// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math/rand/v2"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/noise"
)

// Parameters of the elevation noise that are not in Params.
const (
	elevationNoiseOctaves    = 3
	elevationNoiseLacunarity = 2.0
	elevationNoiseGain       = 0.5
)

// ElevationNoise is the stage that adds low-frequency coherent noise to
// the elevation (§4.5, last sentence: "Finally we apply low frequency
// coherent noise to obtain more realistic variations over the planet").
// It must run after Simulation.
//
// The paper gives neither the noise nor when it is applied. Ours is
// applied once, after the last step of the simulation, so that it does
// not accumulate with the number of steps. For every sample at the
// position p:
//
//	z ← z + A · F(p · R / s), then limited to the range z_t to z_c
//
// with A the parameter ElevationNoiseAmplitude, s the parameter
// ElevationNoiseFeatureSize, R the radius, and F a noise.Fractal of 3
// octaves with lacunarity 2 and gain 0.5: values from −1 to 1 with a
// standard deviation of about 0.26, whose largest features are s km
// across and whose smallest are s/4. Crust type is not considered: the
// noise is the same field over continents and oceans, so it leaves no
// step at a coast. It lowers some continental crust below sea level.
//
// The stage does nothing when Duration is 0, so that the initial world
// is the crust as Crust made it, or when the amplitude is 0. It draws
// one number from its random stream in every case.
type ElevationNoise struct{}

// Name returns "elevation-noise".
func (ElevationNoise) Name() string { return "elevation-noise" }

// Run applies the stage.
func (ElevationNoise) Run(ctx context.Context, w *model.World, rng *rand.Rand) error {
	seed := rng.Uint64()
	if w.Duration == 0 || w.ElevationNoiseAmplitude == 0 {
		return ctx.Err()
	}
	f := noise.NewFractal(seed, elevationNoiseOctaves, elevationNoiseLacunarity, elevationNoiseGain)
	freq := noise.Frequency(w.Radius, w.ElevationNoiseFeatureSize)
	return model.ParallelRange(ctx, len(w.Elevation), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			p := w.Mesh.Points[i]
			z := w.Elevation[i] + w.ElevationNoiseAmplitude*f.At(p.X*freq, p.Y*freq, p.Z*freq)
			w.Elevation[i] = max(w.TrenchElevation, min(w.MaxContinentalElevation, z))
		}
	})
}
