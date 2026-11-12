// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"math"
	"slices"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// A Terrane is a connected region of continental crust within one plate
// (§3): anything from one sample to a whole continent.
type Terrane struct {
	// Plate is the plate that holds the terrane.
	Plate int32
	// Samples are the terrane's samples, in order of sample number.
	Samples []int32
	// Area is the area of the terrane in km²: the sum of the areas its
	// samples stand for (sphere.Mesh.PointAreas).
	Area float64
	// Centroid is the mean position of the samples, each weighted by its
	// area, scaled to length 1. It is the zero vector if the mean is
	// shorter than 10⁻⁹, as for a terrane that circles the sphere.
	Centroid sphere.Vec
}

// Terranes returns the terranes of the world, and for every sample the
// number of its terrane in that list, or −1 if it has none.
//
// Two samples are in the same terrane when both hold continental crust,
// belong to the same plate, and are joined by a chain of neighbouring
// samples (the Mesh's neighbour lists) that all do. A sample marked as
// subducted (World.Subducted) is in no terrane and joins nothing.
//
// Terranes are numbered in order of their lowest sample number, so the
// same world always gives the same list. Nothing is kept between calls:
// call it again after the plates or the crust change.
//
// areas is the area each sample stands for in steradians, as
// sphere.Mesh.PointAreas returns it; nil computes it.
func Terranes(w *model.World, areas []float64) (terranes []Terrane, of []int32) {
	mesh := w.Mesh
	n := len(mesh.Points)
	if areas == nil {
		areas = mesh.PointAreas()
	}
	of = make([]int32, n)
	for i := range of {
		of[i] = -1
	}
	member := func(i int32) bool { return w.CrustType[i] == model.Continental && !w.Subducted[i] }
	var all []int32 // the samples of every terrane, one terrane after another
	var start []int
	for i := range int32(n) {
		if of[i] >= 0 || !member(i) {
			continue
		}
		t := int32(len(start))
		start = append(start, len(all))
		of[i] = t
		all = append(all, i)
		for head := len(all) - 1; head < len(all); head++ {
			at := all[head]
			for _, j := range mesh.Neighbours(int(at)) {
				if of[j] < 0 && member(j) && w.Plate[j] == w.Plate[i] {
					of[j] = t
					all = append(all, j)
				}
			}
		}
	}
	terranes = make([]Terrane, len(start))
	scale := w.Radius * w.Radius
	for t := range terranes {
		end := len(all)
		if t+1 < len(start) {
			end = start[t+1]
		}
		samples := all[start[t]:end:end]
		slices.Sort(samples)
		var area float64
		var sum sphere.Vec
		for _, i := range samples {
			area += areas[i]
			sum = sum.Add(mesh.Points[i].Scale(areas[i]))
		}
		var centroid sphere.Vec
		if sum.Len() >= 1e-9 {
			centroid = sum.Normalize()
		}
		terranes[t] = Terrane{Plate: w.Plate[samples[0]], Samples: samples, Area: area * scale, Centroid: centroid}
	}
	return terranes, of
}

// CollisionRadius returns r, the radius of influence in km of a
// continental collision (§4.2), for a terrane of the given area in km²
// whose plate meets the other at the relative speed v km/My, measured at
// the terrane's centroid:
//
//	r = r_c · √((v / v_0) · (𝒜 / 𝒜_0)), and no more than r_c
//
// with r_c the parameter CollisionDistance, v_0 MaxPlateSpeed, and 𝒜_0
// the mean area of the plates at the start, 4πR² / Plates.
//
// This departs from the paper, which prints
//
//	r = r_c · √(v(q)/v_0) · 𝒜/𝒜_0
//
// with the bar of the root over the speed ratio only and the area ratio
// outside it, and without a limit. We put both ratios under the root: a
// length that grows as the square root of an area keeps its proportion
// to the terrane. The limit at r_c is ours too, from the paper's
// description of r_c as "the global maximum influence distance": two
// plates each at v_0 meet at 2 v_0, and a terrane can be larger than
// 𝒜_0. r is 0 if v_0 is 0.
//
// The formula as printed was tried on 2026-11-11 with the default
// configuration, seeds 1 to 5. It has no limit and grows with the area
// itself, so the collision of two large continents raised both from
// coast to coast: continental crust above 2 km went from 9.4% to 27.9%
// in seed 2. Ours keeps the raised land a range along the suture.
func CollisionRadius(p model.Params, v, area float64) float64 {
	if !(p.MaxPlateSpeed > 0) || p.Plates < 1 {
		return 0
	}
	mean := 4 * math.Pi * p.Radius * p.Radius / float64(p.Plates)
	return p.CollisionDistance * min(1, math.Sqrt(max(0, v/p.MaxPlateSpeed*area/mean)))
}

// CollisionSurge returns Δz, the rise in km that a continental collision
// gives a point d km from a terrane of the given area in km², for the
// radius of influence r in km (§4.2):
//
//	Δz = Δ_c · 𝒜 · (1 − (d/r)²)²   for d < r, and 0 otherwise
//
// with Δ_c the parameter CollisionCoefficient.
func CollisionSurge(p model.Params, area, d, r float64) float64 {
	if !(d < r) {
		return 0
	}
	x := d / r
	return p.CollisionCoefficient * area * (1 - x*x) * (1 - x*x)
}

// Raise returns the elevation in km of crust at z km to which a
// collision gives the surge dz km, when the surge is peak = Δ_c · 𝒜 km at
// the terrane (CollisionSurge at d = 0).
//
// The paper adds the surge to the elevation (§4.2) and gives no limit,
// and with its constants a terrane of a million km² gives a surge of 13
// km, above the highest continental altitude z_c
// (MaxContinentalElevation). Ours:
//
//	dz′ = dz · z_c · (1 − exp(−peak / z_c)) / peak
//	z   ← z + dz′ · (1 − max(z, 0) / z_c), and no more than max(z, z_c)
//
// The first line limits the surge at the terrane to z_c while keeping
// its shape: the whole profile is scaled, so that it still falls
// smoothly to 0 at the radius of influence. Cutting each point's surge
// off at z_c instead would turn the region into a plateau at z_c with a
// cliff at its rim. A peak far below z_c is not changed: 1 km becomes
// 0.95 km, 13 km becomes 7.3 km, and no peak gives more than z_c. The
// second line is the limit of Subduction.Uplift: the rise fades as land
// nears z_c.
//
// A dz or peak of 0 or less changes nothing.
func Raise(p model.Params, z, dz, peak float64) float64 {
	zc := p.MaxContinentalElevation
	if !(dz > 0) || !(peak > 0) || !(zc > 0) {
		return z
	}
	dz *= -zc * math.Expm1(-peak/zc) / peak
	return min(z+dz*max(0, 1-max(z, 0)/zc), max(z, zc))
}

// A Collision is a continental collision that Subduction.Converge has
// found between two plates: the continental crust of Under has gone the
// distance CollisionPenetration beneath the continental crust of Over.
type Collision struct {
	Over, Under int32
	// At are the unit positions, as the plates lay when the collision was
	// found, of the front samples of Over that had continental crust of
	// Under beneath them, in order of sample number.
	At []sphere.Vec
}

// Converge tracks how far the continents of each pair of plates have
// gone into each other, through a step of dt My, and finds the pairs
// that collide (§4.2; §6, "Continental collisions"; issue #28). Fronts
// must have been called since the plates last moved.
//
// A collision front sample is a front sample (Fronts) of continental
// crust on a plate Q whose subducting plate P holds, or held before it
// was subducted, continental crust at that place. Its closing speed is
// −(s_P − s_Q) · n in km/My, with s and n as Fronts defines them: the
// speed at which P moves into Q's edge.
//
// For each pair of plates the interpenetration distance D, in km, starts
// at 0. In each step:
//
//   - if the pair has collision front samples, its closing speed c
//     becomes the greatest of theirs;
//   - D grows by c · dt;
//   - if the pair has had no collision front sample while the fastest
//     sample of any plate moved resampleDistance km, c and D return to 0:
//     the continents have parted. Until then c keeps its last value,
//     because a front cannot be found in the first steps after a
//     resampling, before the plates overlap again;
//   - if the pair has collision front samples and D is at least the
//     parameter CollisionPenetration, the pair collides: c and D return
//     to 0 and a Collision is added to the list that Pending returns.
//
// The paper measures the interpenetration of the plates' triangles (§6).
// Ours is the distance the continents have closed since they met, which
// is the same thing while crust is not removed; we remove the overlapped
// crust at every resampling, so there is no overlap left to measure.
//
// Every meeting of continental with continental crust is tracked,
// whatever the sizes of the continents: this is our reading of the
// paper's "two plates with sufficient continental mass". A terrane
// narrower than CollisionPenetration is consumed before it can collide,
// which is the paper's forced subduction of small terranes (§4.1).
//
// Until a pair collides, the continental crust of the higher-numbered
// plate goes beneath the other and is consumed (Overrides).
func (s *Subduction) Converge(w *model.World, dt float64) {
	plates := w.NumPlates
	if len(s.closed) != plates*plates {
		s.closed = make([]float64, plates*plates)
		s.closing = make([]float64, plates*plates)
		s.apart = make([]float64, plates*plates)
		s.measured = make([]float64, plates*plates)
	}
	clear(s.measured)
	for q := range plates {
		for _, i := range s.front[q] {
			if c := s.approach[i]; c > 0 {
				pair := q*plates + int(s.under[i])
				s.measured[pair] = max(s.measured[pair], c)
			}
		}
	}
	moved := maxSpeed(w) * dt
	s.pending = s.pending[:0]
	for pair, c := range s.measured {
		if c > 0 {
			s.closing[pair], s.apart[pair] = c, 0
		} else if s.closing[pair] > 0 {
			if s.apart[pair] += moved; s.apart[pair] >= resampleDistance {
				s.closing[pair], s.closed[pair], s.apart[pair] = 0, 0, 0
			}
		}
		s.closed[pair] += s.closing[pair] * dt
		if !(c > 0) || s.closed[pair] < w.CollisionPenetration {
			continue
		}
		s.closing[pair], s.closed[pair] = 0, 0
		q, p := pair/plates, pair%plates
		ev := Collision{Over: int32(q), Under: int32(p)}
		for _, i := range s.front[q] {
			if s.approach[i] > 0 && int(s.under[i]) == p {
				ev.At = append(ev.At, w.PlateRotation[q].Apply(w.Mesh.Points[i]))
			}
		}
		s.pending = append(s.pending, ev)
	}
}

// Penetration returns the interpenetration distance in km that Converge
// holds for the continents of plates a and b.
func (s *Subduction) Penetration(a, b int) float64 {
	plates := len(s.front)
	if len(s.closed) != plates*plates {
		return 0
	}
	return s.closed[min(a, b)*plates+max(a, b)]
}

// Pending returns the collisions that the latest call of Converge found,
// in order of the overriding plate's number and then the other's. The
// caller must not change the slice.
func (s *Subduction) Pending() []Collision { return s.pending }

// Collide carries out continental collisions (§4.2; issue #28): for each
// it moves the terranes of one plate to the other and raises the land
// around them. It returns the number of terranes moved.
//
// No plate may have turned since the last resampling: a sample can
// change plates only while both plates' samples are where the mesh has
// them. Simulation therefore resamples in the step in which Converge
// finds a collision, and then calls Collide and Reset.
//
// For each collision in turn, between plates P and Q:
//
//  1. The terranes in contact are the terranes (Terranes) of P and of Q
//     that have a sample within edgeReach sample spacings of one of the
//     collision's positions. If either plate has none, nothing is done:
//     its continental crust at the contact has been consumed.
//  2. The plate whose terranes in contact have the smaller total area
//     gives them up; if the areas are equal, the plate that was beneath
//     does. The paper takes an oceanic plate that carries a terrane into
//     a continental plate; this is the same rule when both are
//     continents. Call the plate that gives them up F and the other T.
//  3. For each of those terranes R, with area 𝒜 and centroid q: the
//     radius of influence is r = CollisionRadius for the relative speed
//     of F and T at q.
//  4. Every sample of the terranes joins T. So does every sample of F in
//     a region that the moved samples enclose: a connected set of F's
//     samples with no neighbour outside itself but moved samples. Left
//     alone, such a lake would be a second plate inside the continent.
//     Crust is not changed, so the continental area of the world is the
//     same before and after. What is left of F keeps its movement.
//  5. For each terrane R in order of terrane number, every sample p of
//     continental crust at a distance d < r is raised, where
//     - for a sample that belonged to T before step 4, d is the distance
//     to the nearest sample of R;
//     - for a sample of R, d is the distance to the suture: to the
//     nearest sample of continental crust that belonged to T before step
//     4 and has a neighbour in R.
//     Both are great-circle distances, found by spreading the samples
//     measured to outward through all samples, nearest first, as
//     sphere.Mesh.Distances does.
//     The elevation z becomes Raise(z, CollisionSurge(𝒜, d, r), Δ_c · 𝒜);
//     the orogeny type becomes Himalayan and the orogeny age 0; and the
//     fold direction becomes CollisionFold(p, q).
//
// The paper raises the collided plate only: it updates the points of
// that plate and then attaches the terrane. That leaves the terrane's
// side of the suture as it was, beside land raised by the whole surge: a
// cliff of up to z_c along every suture. Raising the terrane by the same
// profile, measured from the suture, is ours. Oceanic crust is not
// raised, which is also ours: the paper's region of influence takes every
// point within r.
//
// Then, as after a resampling, the angular speed of any plate whose
// fastest sample would move faster than MaxPlateSpeed is lowered, and
// World.PlateOceanAge is set from the samples.
func (s *Subduction) Collide(w *model.World, events []Collision) (moved int) {
	mesh := w.Mesh
	n := len(mesh.Points)
	if s.surge2 == nil {
		s.surge2 = make([]float64, n)
		s.surgeAt = make([]int32, n)
		for i := range n {
			s.surge2[i], s.surgeAt[i] = math.Inf(1), -1
		}
	}
	var heap spreadHeap
	var touched []int32
	forget := func() {
		for _, j := range touched {
			s.surge2[j], s.surgeAt[j] = math.Inf(1), -1
		}
		touched = touched[:0]
	}
	everywhere := func(int32) bool { return true }
	reach := kmToChord2(edgeReach*s.spacing, w.Radius)

	for _, ev := range events {
		terranes, of := Terranes(w, s.areas)

		// Step 1.
		touched = spread(mesh, &heap, len(ev.At),
			func(k int) sphere.Vec { return ev.At[k] },
			func(k int, seed func(int32)) {
				tri, _ := w.Locator.Locate(ev.At[k])
				for _, c := range mesh.Tri[3*tri : 3*tri+3] {
					seed(c)
				}
			},
			everywhere, reach, s.surge2, s.surgeAt, touched)
		contact := make([]bool, len(terranes))
		for _, j := range touched {
			if t := of[j]; t >= 0 && (terranes[t].Plate == ev.Over || terranes[t].Plate == ev.Under) {
				contact[t] = true
			}
		}
		forget()
		var area [2]float64 // of Over's terranes in contact, and of Under's
		for t, in := range contact {
			if in {
				side := 0
				if terranes[t].Plate == ev.Under {
					side = 1
				}
				area[side] += terranes[t].Area
			}
		}
		if area[0] == 0 || area[1] == 0 {
			continue
		}

		// Steps 2 and 3.
		from, to := ev.Under, ev.Over
		if area[0] < area[1] {
			from, to = ev.Over, ev.Under
		}
		var moving []int32
		var radius []float64
		for t, in := range contact {
			if in && terranes[t].Plate == from {
				moving = append(moving, int32(t))
				v := RelativeSpeed(w, int(from), int(to), terranes[t].Centroid)
				radius = append(radius, CollisionRadius(w.Params, v, terranes[t].Area))
			}
		}

		// Step 4.
		gone := make([]bool, n)
		for _, t := range moving {
			for _, i := range terranes[t].Samples {
				w.Plate[i], gone[i] = to, true
			}
		}
		enclosed(w, from, to, moving, terranes, gone)
		moved += len(moving)

		// Step 5.
		for k, t := range moving {
			r, tr := radius[k], &terranes[t]
			if !(r > 0) || !(w.CollisionCoefficient*tr.Area > 0) {
				continue
			}
			var edge []int32
			for _, i := range tr.Samples {
				for _, j := range mesh.Neighbours(int(i)) {
					if of[j] != t {
						edge = append(edge, i)
						break
					}
				}
			}
			limit := kmToChord2(r, w.Radius)
			rise := func(j int32) {
				dz := CollisionSurge(w.Params, tr.Area, chordToKm(s.surge2[j], w.Radius), r)
				if !(dz > 0) {
					return
				}
				w.Elevation[j] = Raise(w.Params, w.Elevation[j], dz, w.CollisionCoefficient*tr.Area)
				w.OrogenyType[j], w.OrogenyAge[j] = model.OrogenyHimalayan, 0
				w.FoldDirection[j] = CollisionFold(mesh.Points[j], tr.Centroid)
			}
			touched = spread(mesh, &heap, len(edge),
				func(k int) sphere.Vec { return mesh.Points[edge[k]] },
				func(k int, seed func(int32)) { seed(edge[k]) },
				everywhere, limit, s.surge2, s.surgeAt, touched)
			for _, j := range touched {
				if !gone[j] && w.Plate[j] == to && w.CrustType[j] == model.Continental {
					rise(j)
				}
			}
			forget()

			// The terrane's own side, from the suture.
			edge = edge[:0]
			for _, i := range tr.Samples {
				for _, j := range mesh.Neighbours(int(i)) {
					if !gone[j] && w.Plate[j] == to && w.CrustType[j] == model.Continental {
						edge = append(edge, j)
					}
				}
			}
			slices.Sort(edge)
			edge = slices.Compact(edge)
			touched = spread(mesh, &heap, len(edge),
				func(k int) sphere.Vec { return mesh.Points[edge[k]] },
				func(k int, seed func(int32)) {
					for _, j := range mesh.Neighbours(int(edge[k])) {
						seed(j)
					}
				},
				everywhere, limit, s.surge2, s.surgeAt, touched)
			for _, j := range touched {
				if of[j] == t {
					rise(j)
				}
			}
			forget()
		}
	}
	if moved > 0 {
		limitSpeeds(w)
		w.PlateOceanAge = PlateOceanAges(w)
	}
	return moved
}

// CollisionFold returns the fold direction that a collision gives the
// crust at the unit position p, for a terrane with centroid q (§4.2):
//
//	f = (n × (p − q)/‖p − q‖) × n
//
// with n = p, the normal to the sphere at p. It is the unit vector from q
// toward p less its part along n: tangent to the sphere at p, pointing
// away from q along the great circle through both, and of length
// cos(θ/2) for the angle θ between p and q. It is the zero vector if p
// and q are closer than 10⁻¹².
func CollisionFold(p, q sphere.Vec) sphere.Vec {
	d := p.Sub(q)
	if d.Len() < 1e-12 {
		return sphere.Vec{}
	}
	return d.Normalize().Tangent(p)
}

// enclosed carries out the second part of step 4 of Collide: it gives to
// plate to every connected set of samples of plate from whose only
// neighbours outside the set are samples marked in gone, and marks
// them. Only sets that touch a moved terrane are looked at.
func enclosed(w *model.World, from, to int32, moving []int32, terranes []Terrane, gone []bool) {
	mesh := w.Mesh
	seen := make([]bool, len(mesh.Points))
	var set []int32
	for _, t := range moving {
		for _, i := range terranes[t].Samples {
			for _, first := range mesh.Neighbours(int(i)) {
				if w.Plate[first] != from || seen[first] {
					continue
				}
				set = append(set[:0], first)
				seen[first] = true
				open := false
				for head := 0; head < len(set); head++ {
					for _, j := range mesh.Neighbours(int(set[head])) {
						switch {
						case gone[j]:
						case w.Plate[j] != from:
							open = true
						case !seen[j]:
							seen[j] = true
							set = append(set, j)
						}
					}
				}
				if !open {
					for _, j := range set {
						w.Plate[j], gone[j] = to, true
					}
				}
			}
		}
	}
}
