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
// Each step of dt My does these in order:
//
//  1. Subduction.Advance moves every plate and marks the crust that has
//     gone beneath another plate (§4.1).
//  2. AgeOcean ages and dampens the oceanic crust (§4.5).
//  3. Subduction.Fronts finds the subduction fronts.
//  4. Subduction.Converge tracks how far continents have gone into each
//     other, and finds the pairs of plates that collide (§4.2).
//  5. Subduction.Uplift raises the overriding plates and updates fold
//     direction and orogeny (§4.1).
//  6. Subduction.Trenches fills and forms the oceanic trenches (§4.5).
//  7. Erode applies continental erosion (§4.5).
//  8. Subduction.SlabPull turns the axes of the subducting plates
//     (§4.1), which changes how they move from the next step on.
//
// DrawRifts then decides which plates rift in the step
// (§4.4).
//
// The world is then resampled with Resample if any of these holds:
//
//   - the fastest sample of any plate has moved resampleDistance km or
//     more since the last resampling, counting each step as that step's
//     length times the speed of the fastest sample at its start;
//   - step 4 found a collision;
//   - a plate rifts;
//   - it was the last step, and any plate has moved at all.
//
// After a resampling, in order: Subduction.Collide moves the terranes of
// the collisions and raises the land (§4.2); Rift splits each rifting
// plate, in order of plate number (§4.4); and RemoveEmptyPlates removes
// the plates that have no sample left and numbers the rest again.
//
// The stage therefore always leaves the crust on the fixed samples, with
// every plate's rotation the identity, no sample marked as subducted,
// and, if it resampled at all, every plate number from 0 to NumPlates − 1
// used by at least one sample.
//
// Random numbers are drawn only for rifting: by DrawRifts in every step,
// and by Rift.
type Simulation struct{}

// Name returns "simulation".
func (Simulation) Name() string { return "simulation" }

// Run applies the stage.
func (Simulation) Run(ctx context.Context, w *model.World, rng *rand.Rand) error {
	_, err := simulate(ctx, w, rng, resampleDistance)
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
// has moved distance km, and in every step that has a collision or a
// rift. It returns the number of resamplings.
func simulate(ctx context.Context, w *model.World, rng *rand.Rand, distance float64) (resamplings int, err error) {
	steps := Steps(w.Duration, w.TimeStep)
	sub := NewSubduction(w)
	var moved float64   // km, since the last resampling
	var elapsed float64 // My, since the last resampling
	for s := range steps {
		if err := ctx.Err(); err != nil {
			return resamplings, err
		}
		dt := w.TimeStep
		if s == steps-1 {
			dt = w.Duration - float64(steps-1)*w.TimeStep
		}
		moved += maxSpeed(w) * dt
		elapsed += dt
		if err := sub.Step(ctx, w, dt); err != nil {
			return resamplings, err
		}
		collisions := sub.Pending()
		rifts := DrawRifts(w, sub.areas, rng, dt)
		if moved >= distance || len(collisions) > 0 || len(rifts) > 0 || s == steps-1 && moved > 0 {
			if err := Resample(ctx, w, elapsed); err != nil {
				return resamplings, err
			}
			sub.Collide(w, collisions)
			for _, k := range rifts {
				if _, err := Rift(ctx, w, sub.areas, k, rng); err != nil {
					return resamplings, err
				}
			}
			sub.Renumber(w, RemoveEmptyPlates(w))
			sub.Reset(w)
			resamplings++
			moved, elapsed = 0, 0
		}
	}
	return resamplings, nil
}

// Step carries the world through one time step of dt My: the eight
// actions that Simulation lists, in that order.
func (s *Subduction) Step(ctx context.Context, w *model.World, dt float64) error {
	if err := s.Advance(ctx, w, dt); err != nil {
		return err
	}
	if err := AgeOcean(ctx, w, dt); err != nil {
		return err
	}
	if err := s.Fronts(ctx, w); err != nil {
		return err
	}
	s.Converge(w, dt)
	if err := s.Uplift(ctx, w, dt); err != nil {
		return err
	}
	if err := s.Trenches(ctx, w, dt); err != nil {
		return err
	}
	if err := Erode(ctx, w, dt); err != nil {
		return err
	}
	s.SlabPull(w, dt)
	return nil
}
