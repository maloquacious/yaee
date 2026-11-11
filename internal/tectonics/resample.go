// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Resample rebuilds the world on the fixed samples from the plates as
// they now lie (§6, Meshing: "a global resampling"), and sets every
// plate's rotation since the last resampling back to the identity. Axes
// and angular speeds are kept, except as step 5 says.
//
// For every sample, at its position q:
//
//  1. The plates that hold q are found with Query. A plate holds nothing
//     where its crust has been subducted (World.Subducted). A plate is
//     skipped without a query if q is outside the plate's bounding cap: the
//     smallest cap about the plate's mean position that contains every
//     sample of the plate and every neighbour of one, turned with the
//     plate.
//  2. If one plate holds q, the sample takes that plate's crust,
//     interpolated from the corners of the triangle with the weights
//     Query returns:
//     - crust type and orogeny type are those of the corner with the
//     greatest weight (the lowest-numbered sample among equals);
//     - the weight of every corner whose crust type is not that one is
//     then made 0, and the weights are scaled to sum to 1 again, so
//     that oceanic and continental crust are never blended: the coast
//     stays a step in elevation and thickness, and oceanic age is not
//     averaged with the 0 of continental crust;
//     - thickness, elevation, oceanic age, uplift, and trench are the
//     weighted sums of the corners' values;
//     - orogeny age is the weighted sum over the corners whose orogeny
//     type is not none, with their weights scaled to sum to 1, so that
//     it is not averaged with the 0 of crust that has no orogeny;
//     - ridge direction and fold direction are the weighted sums of the
//     corners' vectors, turned by the plate's rotation and projected
//     onto the tangent plane at q; the ridge direction is then scaled
//     to length 1, or set to zero if it is shorter than 10⁻⁹.
//     Then the values that do not apply are made neutral: oceanic age 0
//     and ridge direction zero on continental crust, and orogeny age 0
//     where the orogeny type is none. If the plate has not turned and q
//     is one of its samples, the sample's values are copied unchanged.
//  3. If several plates hold q, the plates have converged and
//     Subduction.Advance has not yet marked the crust of all but one as
//     subducted. The sample takes its crust as in step 2 from the plate
//     whose crust at q Overrides the crust of each of the others, a
//     plate's crust type at q being that of the corner of step 2 with
//     the greatest weight. The crust of the others is discarded.
//  4. If no plate holds q, the plates have diverged, and the sample is
//     in a gap. It gets new oceanic crust; see "Oceanic crust" below.
//
// Each sample then belongs to the plate its crust came from, and no
// sample is marked as subducted. World.PlateOceanAge is set from the new
// samples (PlateOceanAges); steps 1 to 4 use the ages it held before, or
// those of the old samples if it held none.
//
//  5. A plate whose fastest sample would now move faster than
//     MaxPlateSpeed has its angular speed lowered until that sample
//     moves at exactly MaxPlateSpeed. The paper does not say this; it
//     keeps the limit true when a plate gains samples far from its axis.
//
// After a resampling a plate need not be one connected region, and may
// have no samples at all. A plate without samples holds nothing and
// stays empty.
//
// Interpolation is linear, so each resampling blurs the crust a little:
// the error it adds to a smooth field is proportional to the square of
// the sample spacing. The paper interpolates across the coast as well
// (§6, "barycentric interpolation of crust data"); keeping the two
// crust types apart is ours.
//
// # Oceanic crust
//
// The samples of step 4 get the crust that formed at the ridges since
// the last resampling, elapsed My ago (§4.3). A border sample is a
// sample of steps 2 and 3 with a neighbour in a gap. For a gap sample
// at q:
//
//   - a is the nearest border sample that can be reached through the
//     gap, at the angle d_a from q, and A its plate;
//   - b is the nearest border sample of any other plate that can be
//     reached through the gap, at the angle d_b ≥ d_a;
//   - the ridge Γ is where d_a = d_b: the line midway between the two
//     nearest plates. Where three or more plates border a gap the ridge
//     is therefore the lines along which the two nearest plates are
//     equally near, which meet where three are;
//   - the distance to the nearest plate is d_P = d_a, and the distance
//     to the ridge is taken as d_Γ = (d_b − d_a)/2, which is exact
//     between straight parallel plate edges, so the paper's
//     α = d_Γ/(d_Γ + d_P) is (d_b − d_a)/(d_b + d_a): 0 on the ridge
//     and 1 at plate A;
//   - z̄ = (d_b · z(a) + d_a · z(b))/(d_a + d_b), the elevation
//     interpolated between the plates, with z(a) and z(b) the
//     elevations of a and b, or 0 where they are above sea level;
//   - the age is α · elapsed. The gap opened at a steady speed since
//     the last resampling, so crust at the fraction α of the way from
//     the ridge to the plate formed that fraction of the time ago. This
//     is the distance from the ridge divided by the speed at which the
//     plate leaves the ridge, without needing the speed;
//   - the elevation is α · z̄ + (1 − α) · z_Γ, the paper's blend, with
//     z_Γ = RidgeProfile at the sample's age;
//   - the ridge direction is s × q scaled to length 1, with s the
//     direction of spreading at q: the sum of the unit tangent vector
//     pointing away from b and the unit tangent vector pointing toward
//     a, scaled to length 1, and then averaged three times over the
//     sample and its neighbours in the gap (a neighbour's s is negated
//     first if it points away from the sample's, as it does across a
//     ridge). s is the direction in which d_b − d_a grows fastest, so
//     it is at right angles to the ridge, as the paper's q − q′ is, with
//     q′ the projection of q onto the ridge, and s × q is the paper's
//     r = (q − q′) × q. The averaging is ours: a and b are whichever
//     samples of an uneven plate edge are nearest, and without it the
//     direction is uneven by 5° on average and over 20° at worst. The
//     direction is zero if a vector along the way is shorter than 10⁻⁹;
//   - the crust is oceanic, 7 km thick, with no orogeny, fold, uplift,
//     or trench;
//   - the sample joins plate A, so the ridge becomes the boundary
//     between the plates.
//
// Angles are great-circle angles between sample positions. The nearest
// border samples are found by spreading them outward from the border
// through neighbouring gap samples, nearest first; a gap sample keeps
// the first it is offered and the first of another plate. Ties go to the
// lower-numbered border sample.
//
// If only one plate can be reached through the gap, there is no ridge:
// α is 1, so the sample takes the elevation of a, or 0 if that is above
// sea level, and the age elapsed, and has no ridge direction. If no
// sample is held by any plate, every sample gets the ridge elevation
// z_r, age 0, and plate 0.
//
// New oceanic crust is never above sea level. The paper's z̄ takes the
// border elevations as they are, which next to high land would make
// ocean floor that is land.
//
// This departs from the paper, which resamples by intersecting the
// boundary triangles of the plates, found with a bounding box hierarchy
// (§6). See PLAN.md, "Finding what is at a point".
//
// elapsed is the time in My since the last resampling, or since the
// start. It must be at least 0.
func Resample(ctx context.Context, w *model.World, elapsed float64) error {
	mesh := w.Mesh
	n := len(mesh.Points)

	centre, cosine := plateCaps(w)
	ocean := w.PlateOceanAge
	if len(ocean) != w.NumPlates {
		ocean = PlateOceanAges(w)
	}
	inverse := make([]sphere.Rotation, w.NumPlates)
	turned := make([]bool, w.NumPlates)
	for k, r := range w.PlateRotation {
		centre[k] = r.Apply(centre[k])
		inverse[k] = r.Inverse()
		turned[k] = r != sphere.Identity()
	}

	next := model.World{
		Plate:          make([]int32, n),
		CrustType:      make([]uint8, n),
		Thickness:      make([]float64, n),
		Elevation:      make([]float64, n),
		OceanicAge:     make([]float64, n),
		RidgeDirection: make([]sphere.Vec, n),
		OrogenyAge:     make([]float64, n),
		OrogenyType:    make([]uint8, n),
		FoldDirection:  make([]sphere.Vec, n),
		Uplift:         make([]float64, n),
		Trench:         make([]float64, n),
	}
	err := model.ParallelRange(ctx, n, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			q := mesh.Points[i]

			// Steps 1 and 3: the plate the sample takes its crust from.
			plate := int32(-1)
			var (
				corners []int32
				weights [3]float64
				nearest int // the corner with the greatest weight
			)
			for k := range int32(w.NumPlates) {
				if !(q.Dot(centre[k]) >= cosine[k]) {
					continue
				}
				tri, wts, ok := query(w, k, inverse[k].Apply(q))
				if !ok {
					continue
				}
				cs := mesh.Tri[3*tri : 3*tri+3]
				c := nearestCorner(cs, wts)
				// Step 3: the crust that overrides the others stays.
				if plate >= 0 {
					mine, theirs := cs[c], corners[nearest]
					if !Overrides(w.CrustType[mine], ocean[k], k, w.CrustType[theirs], ocean[plate], plate) {
						continue
					}
				}
				plate, corners, weights, nearest = k, cs, wts, c
			}

			next.Plate[i] = plate
			if plate < 0 {
				continue // step 4, below
			}

			// Step 2.
			s := corners[nearest]
			if weights[nearest] == 1 && !turned[plate] {
				next.CrustType[i] = w.CrustType[s]
				next.Thickness[i] = w.Thickness[s]
				next.Elevation[i] = w.Elevation[s]
				next.OceanicAge[i] = w.OceanicAge[s]
				next.RidgeDirection[i] = w.RidgeDirection[s]
				next.OrogenyAge[i] = w.OrogenyAge[s]
				next.OrogenyType[i] = w.OrogenyType[s]
				next.FoldDirection[i] = w.FoldDirection[s]
				next.Uplift[i] = w.Uplift[s]
				next.Trench[i] = w.Trench[s]
				continue
			}
			a, b, c := corners[0], corners[1], corners[2]
			var kept float64
			for j, corner := range corners {
				if w.CrustType[corner] != w.CrustType[s] {
					weights[j] = 0
				}
				kept += weights[j]
			}
			if kept != 1 {
				for j := range weights {
					weights[j] /= kept
				}
			}
			blend := func(x []float64) float64 {
				return weights[0]*x[a] + weights[1]*x[b] + weights[2]*x[c]
			}
			direction := func(x []sphere.Vec) sphere.Vec {
				v := x[a].Scale(weights[0]).Add(x[b].Scale(weights[1])).Add(x[c].Scale(weights[2]))
				return w.PlateRotation[plate].Apply(v).Tangent(q)
			}
			next.CrustType[i] = w.CrustType[s]
			next.OrogenyType[i] = w.OrogenyType[s]
			next.Thickness[i] = blend(w.Thickness)
			// Rounding can carry a blend a few units in the last place
			// past its corners, and so past the limits they keep.
			next.Elevation[i] = max(w.TrenchElevation, min(w.MaxContinentalElevation, blend(w.Elevation)))
			next.FoldDirection[i] = direction(w.FoldDirection)
			if next.CrustType[i] == model.Oceanic {
				next.OceanicAge[i] = blend(w.OceanicAge)
				if r := direction(w.RidgeDirection); r.Len() >= 1e-9 {
					next.RidgeDirection[i] = r.Normalize()
				}
			}
			next.Uplift[i] = blend(w.Uplift)
			next.Trench[i] = blend(w.Trench)
			if next.OrogenyType[i] != model.OrogenyNone {
				// Among the corners that have an orogeny.
				var age, share float64
				for j, corner := range corners {
					if w.OrogenyType[corner] != model.OrogenyNone {
						age += weights[j] * w.OrogenyAge[corner]
						share += weights[j]
					}
				}
				next.OrogenyAge[i] = age / share
			}
		}
	})
	if err != nil {
		return err
	}

	fillGaps(mesh, w.Params, &next, elapsed) // step 4
	if err := ctx.Err(); err != nil {
		return err
	}

	w.Plate = next.Plate
	w.CrustType = next.CrustType
	w.Thickness = next.Thickness
	w.Elevation = next.Elevation
	w.OceanicAge = next.OceanicAge
	w.RidgeDirection = next.RidgeDirection
	w.OrogenyAge = next.OrogenyAge
	w.OrogenyType = next.OrogenyType
	w.FoldDirection = next.FoldDirection
	w.Uplift = next.Uplift
	w.Trench = next.Trench
	clear(w.Subducted)
	for k := range w.PlateRotation {
		w.PlateRotation[k] = sphere.Identity()
	}
	w.PlateOceanAge = PlateOceanAges(w)
	limitSpeeds(w) // step 5
	return nil
}

// plateCaps returns a bounding cap for each plate, as the plate lay at
// the last resampling: every point the plate holds is within the angle
// whose cosine is cosine[k] of the unit vector centre[k]. For a plate
// without samples cosine[k] is +Inf, which no point satisfies.
//
// A plate holds only points of triangles that have one of its samples as
// a corner, and a point of a triangle is no further from a corner than
// the triangle's longest edge. The cap's angle is therefore the greatest
// angle from the centre to a sample of the plate, plus the length of the
// longest edge at any sample of the plate. (The cap through the corners
// alone is not enough: a triangle bulges out of a cap that is larger
// than a hemisphere.)
func plateCaps(w *model.World) (centre []sphere.Vec, cosine []float64) {
	mesh := w.Mesh
	centre = make([]sphere.Vec, w.NumPlates)
	cosine = make([]float64, w.NumPlates)
	for i, p := range mesh.Points {
		centre[w.Plate[i]] = centre[w.Plate[i]].Add(p)
	}
	reach := make([]float64, w.NumPlates) // least cosine from the centre to a sample
	edge := make([]float64, w.NumPlates)  // least cosine between a sample and a neighbour
	for k, c := range centre {
		reach[k], edge[k] = math.Inf(1), 1
		// The mean position of a plate that surrounds the sphere can be
		// the centre of the sphere. Any direction will do for the cap.
		if c.Len() < 1e-9 {
			c = sphere.Vec{Z: 1}
		}
		centre[k] = c.Normalize()
	}
	for i, p := range mesh.Points {
		k := w.Plate[i]
		reach[k] = min(reach[k], centre[k].Dot(p))
		for _, j := range mesh.Neighbours(i) {
			edge[k] = min(edge[k], p.Dot(mesh.Points[j]))
		}
	}
	for k := range cosine {
		cosine[k] = math.Inf(1)
		if !math.IsInf(reach[k], 1) {
			angle := math.Acos(max(-1, min(1, reach[k]))) + math.Acos(max(-1, min(1, edge[k])))
			// A little room for rounding when the cap is turned.
			cosine[k] = math.Cos(min(math.Pi, angle)) - 1e-12
		}
	}
	return centre, cosine
}
