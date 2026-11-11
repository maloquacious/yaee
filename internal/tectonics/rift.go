// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Constants of rifting. The paper gives none of them (§4.4).
const (
	// riftOceanic is f(0), the value of the transfer function RiftTransfer
	// for a plate with no continental crust; f(1) is 1. A wholly oceanic
	// plate rifts a quarter as often as a wholly continental plate of the
	// same area. It is above 0 so that an oceanic plate that has grown to
	// cover much of the sphere still breaks up.
	riftOceanic = 0.25

	// riftMinArea is the least area a plate must have to rift, as a
	// fraction of 𝒜_0, the mean area of the plates at the start. Without
	// it fragments would go on splitting into slivers. With 1, the number
	// of plates in a default run stays near the number at the start; with
	// 0.5 it grew from 40 to 67 in 250 My.
	riftMinArea = 1.0

	// A plate rifts into riftMinFragments to riftMaxFragments fragments,
	// each number being as likely as the others (§4.4: "n ∈ [2, 4]").
	riftMinFragments = 2
	riftMaxFragments = 4
)

// PlateAreas returns, for each plate, the area in km² of its samples and
// of its continental samples. Samples marked as subducted
// (World.Subducted) are not counted.
//
// areas is the area each sample stands for in steradians, as
// sphere.Mesh.PointAreas returns it.
func PlateAreas(w *model.World, areas []float64) (area, continental []float64) {
	area = make([]float64, w.NumPlates)
	continental = make([]float64, w.NumPlates)
	scale := w.Radius * w.Radius
	for i, k := range w.Plate {
		if w.Subducted[i] {
			continue
		}
		a := areas[i] * scale
		area[k] += a
		if w.CrustType[i] == model.Continental {
			continental[k] += a
		}
	}
	return area, continental
}

// RiftTransfer returns f(x_P), "the influence of the continental crust
// type ratio of the plate" on how often it rifts (§4.4), for the share x
// of the plate's area that is continental, from 0 to 1:
//
//	f(x) = riftOceanic + (1 − riftOceanic) · x
//
// The paper does not give f. This one is linear, and 1 for a wholly
// continental plate.
func RiftTransfer(x float64) float64 {
	return riftOceanic + (1-riftOceanic)*max(0, min(1, x))
}

// RiftRate returns the mean number of times per My that a plate rifts
// (§4.4): λ = λ_0 · f(x_P) · 𝒜 / 𝒜_0, with λ_0 the parameter
// RiftingRate, f RiftTransfer, 𝒜 the plate's area in km², x_P the share
// of that area that is continental (continental / area), and 𝒜_0 the
// mean area of the plates at the start, 4πR² / Plates.
//
// It is 0 for a plate whose area is less than riftMinArea · 𝒜_0.
//
// The paper's λ_0 is "the average number of rifting events in the
// considered time window" and it does not say what the window is. Ours
// is a rate per My, so that the number of rifts does not depend on the
// time step.
func RiftRate(p model.Params, area, continental float64) float64 {
	a0 := 4 * math.Pi * p.Radius * p.Radius / float64(p.Plates)
	if !(area > 0) || area < riftMinArea*a0 {
		return 0
	}
	return p.RiftingRate * RiftTransfer(continental/area) * area / a0
}

// RiftProbability returns the probability that a plate which rifts at
// the given rate per My does so in a step of dt My: 1 − e^(−rate · dt),
// the probability of at least one event of a Poisson process.
//
// The paper's P = λ e^(−λ) is the probability of exactly one event. The
// two agree to first order in λ; ours does not fall again as λ grows,
// and a plate rifts at most once in a step either way.
func RiftProbability(rate, dt float64) float64 {
	return -math.Expm1(-rate * dt)
}

// DrawRifts decides which plates rift in a step of dt My, and returns
// their numbers in ascending order (§4.4).
//
// For each plate in order of plate number whose RiftRate, for the areas
// PlateAreas gives, is above 0, one number is drawn uniformly from
// [0, 1); the plate rifts if it is below RiftProbability. Nothing is
// drawn for the other plates.
//
// No plate rifts, and nothing is drawn, if RiftingRate or MaxPlateSpeed
// is 0: fragments that cannot move cannot part.
func DrawRifts(w *model.World, areas []float64, rng *rand.Rand, dt float64) []int32 {
	if !(w.RiftingRate > 0) || !(w.MaxPlateSpeed > 0) {
		return nil
	}
	var rifts []int32
	area, continental := PlateAreas(w, areas)
	for k := range w.NumPlates {
		rate := RiftRate(w.Params, area[k], continental[k])
		if rate > 0 && rng.Float64() < RiftProbability(rate, dt) {
			rifts = append(rifts, int32(k))
		}
	}
	return rifts
}

// Rift fractures plate k into fragments, each of which becomes a plate
// (§4.4), and returns their number: 2 to 4, or 0 if the plate has fewer
// than two samples, in which case nothing is done and nothing drawn.
//
// No plate may have turned since the last resampling, and no sample may
// be marked as subducted. Simulation therefore resamples in the step in
// which a plate rifts, then calls Rift, and then Subduction.Reset.
//
// Method, with 𝒜 the area of the plate and S its samples:
//
//  1. Number. n is drawn uniformly from riftMinFragments to
//     riftMaxFragments, and lowered to the number of samples if that is
//     less.
//  2. Speed. v is drawn uniformly from 0 to MaxPlateSpeed (v_0), in
//     km/My.
//  3. Warp. Three noise sources are drawn as for the initial plates
//     (Plates, step 2), with the amplitude PlateWarpAmplitude · s and the
//     feature size PlateWarpFeatureSize · s, s = √(𝒜 / (n · 𝒜_0)), 𝒜_0
//     being the mean area of the plates at the start. A fracture is
//     therefore as irregular, for the size of the fragments, as the
//     boundaries of the initial plates.
//  4. Centroids. Each of n centroids is a sample drawn uniformly from S,
//     drawn again if it is an earlier centroid or closer than
//     centroidSeparation · √(𝒜 / n) km to one; the distance is relaxed
//     as in Plates, step 1.
//  5. Fragments. The samples of S are divided among the centroids as
//     Plates divides the sphere (steps 3 to 5): each sample goes to the
//     centroid nearest its warped position; a fragment's seed is its
//     sample nearest its centroid; a fragment keeps only the samples
//     joined to its seed through neighbours of the same fragment, and
//     the rest go to the fragment of the nearest kept sample, counted in
//     steps between neighbours within S. If the plate is one connected
//     region, every fragment therefore is. Samples of S that are not
//     joined to any seed through S, which a plate in several pieces can
//     have, stay with the nearest centroid.
//  6. Numbers. The fragment of the first centroid keeps the number k.
//     The others take the numbers NumPlates, NumPlates + 1, … in the
//     order of their centroids, and NumPlates grows by n − 1.
//  7. Motion. Each fragment gets a new movement in place of the plate's:
//     with g the mean position of S and g_i that of fragment i, each
//     weighted by sample area and scaled to length 1, the axis is
//     g × g_i scaled to length 1 and the angular speed v / R. The
//     fragment's centroid then moves straight away from g, and no sample
//     moves faster than v. If g × g_i is shorter than 10⁻⁹ (the fragment
//     surrounds g, or the plate circles the sphere) the fragment keeps
//     the plate's axis and has the angular speed 0.
//
// All fragments move away from the same point at the same angular
// speed, so any two of them move apart: at a place p on the boundary of
// fragments i and j, the velocity of i relative to j is
// (v / R) · (a_i − a_j) × p with a the unit axes, which points from g_j
// toward g_i for fragments that are small beside the sphere. The
// paper's "random diverging directions" are not specified further.
//
// No crust attribute of any sample changes. World.PlateOceanAge is set
// from the samples of the new plates.
func Rift(ctx context.Context, w *model.World, areas []float64, k int32, rng *rand.Rand) (int, error) {
	mesh := w.Mesh
	points := mesh.Points
	var (
		samples []int32
		area    float64 // steradians
		sum     sphere.Vec
	)
	for i, p := range w.Plate {
		if p == k {
			samples = append(samples, int32(i))
			area += areas[i]
			sum = sum.Add(points[i].Scale(areas[i]))
		}
	}
	if len(samples) < 2 {
		return 0, nil
	}

	// Steps 1 to 3.
	n := min(riftMinFragments+rng.IntN(riftMaxFragments-riftMinFragments+1), len(samples))
	speed := w.MaxPlateSpeed * rng.Float64()
	scale := math.Sqrt(area * float64(w.Plates) / (float64(n) * 4 * math.Pi))
	wp := newWarp(rng, w.Radius, w.PlateWarpAmplitude*scale, w.PlateWarpFeatureSize*scale)

	// Step 4.
	centroids := make([]sphere.Vec, 0, n)
	chosen := make([]int32, 0, n)
	separation := centroidSeparation * math.Sqrt(area/float64(n)) // radians
	minCos := math.Cos(min(separation, math.Pi))
	rejected := 0
	for len(chosen) < n {
		i := samples[rng.IntN(len(samples))]
		ok := !slices.Contains(chosen, i)
		for _, c := range centroids {
			if points[i].Dot(c) > minCos {
				ok = false
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
		chosen = append(chosen, i)
		centroids = append(centroids, points[i])
	}

	// Step 5. fragment[i] is −1 outside the plate.
	fragment := make([]int32, len(points))
	for i := range fragment {
		fragment[i] = -1
	}
	best := make([]float64, len(samples)) // q · c / |q| for the chosen centroid
	err := model.ParallelRange(ctx, len(samples), func(lo, hi int) {
		for x := lo; x < hi; x++ {
			i := samples[x]
			q := wp.apply(points[i])
			f, d := 0, q.Dot(centroids[0])
			for j := 1; j < n; j++ {
				if dj := q.Dot(centroids[j]); dj > d {
					f, d = j, dj
				}
			}
			fragment[i] = int32(f)
			best[x] = d / q.Len()
		}
	})
	if err != nil {
		return 0, err
	}
	seed := make([]int32, n) // as a place in samples
	for f := range seed {
		seed[f] = -1
	}
	for x, i := range samples {
		if f := fragment[i]; seed[f] < 0 || best[x] > best[seed[f]] {
			seed[f] = int32(x)
		}
	}
	isSeed := make([]bool, len(samples))
	for _, x := range seed {
		if x >= 0 {
			isSeed[x] = true
		}
	}
	for f, c := range centroids {
		if seed[f] >= 0 {
			continue
		}
		at, d := -1, math.Inf(-1)
		for x, i := range samples {
			if !isSeed[x] {
				if di := points[i].Dot(c); di > d {
					at, d = x, di
				}
			}
		}
		seed[f], isSeed[at], fragment[samples[at]] = int32(at), true, int32(f)
	}

	kept := make([]bool, len(points))
	queue := make([]int32, 0, len(samples))
	for _, x := range seed {
		kept[samples[x]] = true
		queue = append(queue, samples[x])
	}
	for head := 0; head < len(queue); head++ {
		i := queue[head]
		for _, j := range mesh.Neighbours(int(i)) {
			if !kept[j] && fragment[j] == fragment[i] {
				kept[j] = true
				queue = append(queue, j)
			}
		}
	}
	if len(queue) < len(samples) {
		queue = queue[:0]
		for _, i := range samples {
			if kept[i] {
				queue = append(queue, i)
			}
		}
		for head := 0; head < len(queue); head++ {
			i := queue[head]
			for _, j := range mesh.Neighbours(int(i)) {
				if !kept[j] && fragment[j] >= 0 {
					kept[j] = true
					fragment[j] = fragment[i]
					queue = append(queue, j)
				}
			}
		}
	}

	// Step 6.
	number := make([]int32, n)
	number[0] = k
	for f := 1; f < n; f++ {
		number[f] = int32(w.NumPlates + f - 1)
	}
	mean := make([]sphere.Vec, n)
	for _, i := range samples {
		f := fragment[i]
		w.Plate[i] = number[f]
		mean[f] = mean[f].Add(points[i].Scale(areas[i]))
	}

	// Step 7.
	var g sphere.Vec
	if sum.Len() >= 1e-9 {
		g = sum.Normalize()
	}
	parent := w.PlateAxis[k]
	for f := range n {
		axis, omega := parent, 0.0
		if mean[f].Len() >= 1e-9 {
			if a := g.Cross(mean[f].Normalize()); a.Len() >= 1e-9 {
				axis, omega = a.Normalize(), speed/w.Radius
			}
		}
		if f == 0 {
			w.PlateAxis[k], w.PlateSpeed[k] = axis, omega
			continue
		}
		w.PlateAxis = append(w.PlateAxis, axis)
		w.PlateSpeed = append(w.PlateSpeed, omega)
		w.PlateRotation = append(w.PlateRotation, sphere.Identity())
	}
	w.NumPlates += n - 1
	w.PlateOceanAge = PlateOceanAges(w)
	return n, nil
}

// RemoveEmptyPlates removes every plate that has no sample, and numbers
// the rest again from 0 in the order they had. It returns, for each
// plate number as it was, the plate's new number, or −1 for a plate
// that was removed.
//
// A plate loses its last sample when the whole of it has gone beneath
// other plates, or when a collision moves all of it to another plate.
// The paper does not say what becomes of such a plate. Nothing can
// return to it: resampling gives new crust to the plates beside a gap.
func RemoveEmptyPlates(w *model.World) (remap []int32) {
	used := make([]bool, w.NumPlates)
	for _, k := range w.Plate {
		used[k] = true
	}
	remap = make([]int32, w.NumPlates)
	ocean := len(w.PlateOceanAge) == w.NumPlates
	next := 0
	for k, u := range used {
		if !u {
			remap[k] = -1
			continue
		}
		remap[k] = int32(next)
		w.PlateAxis[next] = w.PlateAxis[k]
		w.PlateSpeed[next] = w.PlateSpeed[k]
		w.PlateRotation[next] = w.PlateRotation[k]
		if ocean {
			w.PlateOceanAge[next] = w.PlateOceanAge[k]
		}
		next++
	}
	if next == w.NumPlates {
		return remap
	}
	w.PlateAxis = w.PlateAxis[:next]
	w.PlateSpeed = w.PlateSpeed[:next]
	w.PlateRotation = w.PlateRotation[:next]
	if ocean {
		w.PlateOceanAge = w.PlateOceanAge[:next]
	}
	for i, k := range w.Plate {
		w.Plate[i] = remap[k]
	}
	w.NumPlates = next
	return remap
}

// Renumber prepares the state for plates that have been split (Rift) or
// removed (RemoveEmptyPlates) since the state was made or last
// renumbered. remap is what RemoveEmptyPlates returned. What Converge
// holds for a pair of plates follows the plates to their new numbers; a
// new plate starts with none. Reset must be called afterwards.
func (s *Subduction) Renumber(w *model.World, remap []int32) {
	old, plates := len(s.front), w.NumPlates
	same := old == plates
	for k := 0; same && k < old; k++ {
		same = remap[k] == int32(k)
	}
	if same {
		return
	}
	for k := range old {
		s.clearDistances(k)
	}
	s.front = make([][]int32, plates)
	s.lost = make([]bool, plates)
	s.reached = make([][]int32, plates)
	s.pulls = make([][]int32, plates)
	s.scratch = make([][]int32, plates)
	s.heaps = make([]spreadHeap, plates)
	if len(s.closed) != old*old {
		return
	}
	closed := make([]float64, plates*plates)
	closing := make([]float64, plates*plates)
	apart := make([]float64, plates*plates)
	for a := range old {
		for b := range old {
			if na, nb := remap[a], remap[b]; na >= 0 && nb >= 0 {
				from, to := a*old+b, int(na)*plates+int(nb)
				closed[to], closing[to], apart[to] = s.closed[from], s.closing[from], s.apart[from]
			}
		}
	}
	s.closed, s.closing, s.apart = closed, closing, apart
	s.measured = make([]float64, plates*plates)
}
