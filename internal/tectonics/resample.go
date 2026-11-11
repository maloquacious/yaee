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
//  1. The plates that hold q are found with Query. A plate is skipped
//     without a query if q is outside the plate's bounding cap: the
//     smallest cap about the plate's mean position that contains every
//     sample of the plate and every neighbour of one, turned with the
//     plate.
//  2. If one plate holds q, the sample takes that plate's crust,
//     interpolated from the corners of the triangle with the weights
//     Query returns:
//     - thickness, elevation, oceanic age, and orogeny age are the
//     weighted sums of the corners' values;
//     - ridge direction and fold direction are the weighted sums of the
//     corners' vectors, turned by the plate's rotation and projected
//     onto the tangent plane at q; the ridge direction is then scaled
//     to length 1, or set to zero if it is shorter than 10⁻⁹;
//     - crust type and orogeny type are those of the corner with the
//     greatest weight (the lowest-numbered sample among equals).
//     Then the values that do not apply are made neutral: oceanic age 0
//     and ridge direction zero on continental crust, and orogeny age 0
//     where the orogeny type is none. If the plate has not turned and q
//     is one of its samples, the sample's values are copied unchanged.
//  3. If several plates hold q, the plates have converged. Placeholder,
//     replaced by subduction in issue #6: the sample takes its crust
//     from one of them as in step 2, a plate whose crust at q is
//     continental before one whose crust is oceanic, and the
//     lowest-numbered plate among those.
//  4. If no plate holds q, the plates have diverged. Placeholder,
//     replaced by oceanic crust generation in issue #5: the sample gets
//     new oceanic crust, 7 km thick, at the ridge elevation z_r, of age
//     0, with no ridge direction, orogeny, or fold.
//
// Each sample then belongs to the plate its crust came from. A sample
// of step 4 belongs to the plate of the nearest sample of steps 2 and
// 3, counted in steps between neighbours, working outward in order of
// sample number; if there is no such sample it belongs to plate 0.
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
// the sample spacing.
//
// This departs from the paper, which resamples by intersecting the
// boundary triangles of the plates, found with a bounding box hierarchy
// (§6). See PLAN.md, "Finding what is at a point".
func Resample(ctx context.Context, w *model.World) error {
	mesh := w.Mesh
	n := len(mesh.Points)

	centre, cosine := plateCaps(w)
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
				c := 0
				for j := 1; j < 3; j++ {
					if wts[j] > wts[c] || wts[j] == wts[c] && cs[j] < cs[c] {
						c = j
					}
				}
				// Placeholder for subduction (#6): continental crust
				// stays on top, then the lowest-numbered plate.
				if plate < 0 || w.CrustType[cs[c]] == model.Continental && w.CrustType[corners[nearest]] == model.Oceanic {
					plate, corners, weights, nearest = k, cs, wts, c
				}
			}

			next.Plate[i] = plate
			if plate < 0 {
				// Step 4. Placeholder for oceanic crust generation (#5).
				next.CrustType[i] = model.Oceanic
				next.Thickness[i] = oceanicThickness
				next.Elevation[i] = w.RidgeElevation
				continue // the rest is zero
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
				continue
			}
			a, b, c := corners[0], corners[1], corners[2]
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
			next.Elevation[i] = blend(w.Elevation)
			next.FoldDirection[i] = direction(w.FoldDirection)
			if next.CrustType[i] == model.Oceanic {
				next.OceanicAge[i] = blend(w.OceanicAge)
				if r := direction(w.RidgeDirection); r.Len() >= 1e-9 {
					next.RidgeDirection[i] = r.Normalize()
				}
			}
			if next.OrogenyType[i] != model.OrogenyNone {
				next.OrogenyAge[i] = blend(w.OrogenyAge)
			}
		}
	})
	if err != nil {
		return err
	}

	// Give the samples of step 4 to the plates around them.
	queue := make([]int32, 0, n)
	for i, k := range next.Plate {
		if k >= 0 {
			queue = append(queue, int32(i))
		}
	}
	if len(queue) == 0 {
		clear(next.Plate) // all to plate 0
	}
	for head := 0; head < len(queue) && len(queue) < n; head++ {
		i := queue[head]
		for _, j := range mesh.Neighbours(int(i)) {
			if next.Plate[j] < 0 {
				next.Plate[j] = next.Plate[i]
				queue = append(queue, j)
			}
		}
	}
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
	for k := range w.PlateRotation {
		w.PlateRotation[k] = sphere.Identity()
	}
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
