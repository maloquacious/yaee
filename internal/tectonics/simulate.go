// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/maloquacious/yaee/internal/model"
)

// resampleDistance is the distance in km that the fastest sample of any
// plate moves between two resamplings.
//
// The paper resamples "every 10 − 60 iterations", "depending on the
// observed maximum plate speed" (§4.3, §6), which with its 2 My step and
// its v0 of 100 km/My is every 2000 km or more. It gives no rule. Ours
// is a distance, not a count of steps, so that it holds for any time
// step and any sample spacing. 500 km is shorter than the paper's
// because between resamplings a gap between diverging plates holds no
// crust and overlapping plates hold two: the shorter the distance, the
// narrower those bands when they are resolved. The cost is one
// resampling per 500 km, each of which blurs the crust slightly (see
// Resample); a default run of 250 My has at most 50.
const resampleDistance = 500.0

// Simulation is the stage that runs the tectonic simulation over
// Params.Duration. It must run after Motion.
//
// Time advances in steps of Params.TimeStep; if Duration is not a whole
// number of steps, the last step is the shorter remainder, so that the
// steps add up to Duration exactly. With Duration 0 the stage does
// nothing.
//
// Each step moves every plate with Advance. The world is then resampled
// with Resample if the fastest sample of any plate has moved
// resampleDistance km or more since the last resampling, counting each
// step as that step's length times the speed of the fastest sample at
// its start. After the last step the world is resampled if any plate
// has moved at all, so the stage always leaves the crust on the fixed
// samples, with every plate's rotation the identity.
//
// The stage draws no random numbers yet.
type Simulation struct{}

// Name returns "simulation".
func (Simulation) Name() string { return "simulation" }

// Run applies the stage.
func (Simulation) Run(ctx context.Context, w *model.World, _ *rand.Rand) error {
	_, err := simulate(ctx, w, resampleDistance)
	return err
}

// Steps returns the number of time steps in a run of the given duration
// and time step, both in My: the smallest whole number of steps that
// covers the duration. Every step but the last is timeStep long, and the
// last is duration − (Steps − 1) · timeStep, which is greater than 0 and
// at most timeStep. duration must be at least 0 and timeStep greater
// than 0.
func Steps(duration, timeStep float64) int {
	n := int(math.Ceil(duration / timeStep))
	// Rounding in the division can give one step too many or too few.
	for n > 0 && float64(n-1)*timeStep >= duration {
		n--
	}
	for float64(n)*timeStep < duration {
		n++
	}
	return n
}

// simulate runs the simulation, resampling each time the fastest sample
// has moved distance km. It returns the number of resamplings.
func simulate(ctx context.Context, w *model.World, distance float64) (resamplings int, err error) {
	steps := Steps(w.Duration, w.TimeStep)
	speed := maxSpeed(w)
	var moved float64 // km, since the last resampling
	for s := range steps {
		if err := ctx.Err(); err != nil {
			return resamplings, err
		}
		dt := w.TimeStep
		if s == steps-1 {
			dt = w.Duration - float64(steps-1)*w.TimeStep
		}
		Advance(w, dt)
		moved += speed * dt
		if moved >= distance || s == steps-1 && moved > 0 {
			if err := Resample(ctx, w); err != nil {
				return resamplings, err
			}
			resamplings++
			moved = 0
			speed = maxSpeed(w)
		}
	}
	return resamplings, nil
}
