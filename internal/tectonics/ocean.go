// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// AgeOcean carries the oceanic crust through a time step of dt My
// (§4.5, oceanic dampening). For every sample of oceanic crust:
//
//   - the oceanic age a_o increases by dt;
//   - the elevation z becomes z − (1 − z/z_t) · ε_o · dt, with z_t the
//     trench elevation and ε_o the parameter OceanicDampening.
//
// The elevation formula is the paper's, as written. It is the explicit
// Euler step of dz/dt = −(1 − z/z_t) · ε_o, whose solution is
//
//	z(t) = z_t + (z(0) − z_t) · exp(−ε_o · t / |z_t|).
//
// The step moves z toward z_t by the fraction ε_o · dt / |z_t| of its
// distance from z_t: 0.008 with the defaults. One departure: where that
// fraction is greater than 1 it is taken as 1, so that z lands on z_t
// and does not pass it. While the fraction is at most 0.5, the result
// after t My in steps of dt differs from the solution by less than
// (z(0) − z_t) · t · ε_o² · dt / z_t²; for 250 My of the default steps
// on crust that starts at the ridge elevation the difference is
// 0.013 km.
//
// The elevation approaches z_t, not the abyssal elevation z_a: z_a is
// only where crust formed at z_r is after about 200 My.
//
// Continental crust is not changed. dt must be at least 0.
func AgeOcean(ctx context.Context, w *model.World, dt float64) error {
	keep := max(0, 1-w.OceanicDampening*dt/math.Abs(w.TrenchElevation))
	return model.ParallelRange(ctx, len(w.CrustType), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if w.CrustType[i] == model.Oceanic {
				w.OceanicAge[i] += dt
				w.Elevation[i] = w.TrenchElevation + (w.Elevation[i]-w.TrenchElevation)*keep
			}
		}
	})
}

// RidgeProfile returns the template ridge profile z_Γ (§4.3, Figure 9)
// as a function of the age in My of the crust: the elevation in km of
// oceanic crust that formed at the ridge elevation z_r that long ago,
//
//	z_Γ(a) = z_t + (z_r − z_t) · exp(−ε_o · a / |z_t|).
//
// It is z_r at age 0, on the ridge. The paper shows the profile's shape,
// highest at the ridge and falling to either side, and gives no formula.
// This one is the solution of the paper's own dampening (see AgeOcean),
// so new crust has the elevation it would have had if it had formed at
// the ridge and been dampened ever since.
func RidgeProfile(p model.Params, age float64) float64 {
	return p.TrenchElevation + (p.RidgeElevation-p.TrenchElevation)*math.Exp(-p.OceanicDampening*age/math.Abs(p.TrenchElevation))
}

// fillGaps generates oceanic crust (§4.3) for every sample of next that
// no plate holds, marked by a Plate of −1, and gives each a plate.
// Resample documents the rules. mesh is the mesh of the world, p its
// parameters, and elapsed the time in My since the last resampling.
func fillGaps(mesh *sphere.Mesh, p model.Params, next *model.World, elapsed float64) {
	// slot[i] is the place of sample i in gaps, or −1.
	slot := make([]int32, len(next.Plate))
	var gaps []int32
	for i, k := range next.Plate {
		slot[i] = -1
		if k < 0 {
			slot[i] = int32(len(gaps))
			gaps = append(gaps, int32(i))
		}
	}
	if len(gaps) == 0 {
		return
	}

	// near[g] holds up to two border samples of different plates for
	// gap sample g: count[g] of them.
	type border struct {
		site  int32   // the border sample
		angle float64 // from the gap sample, in radians
	}
	near := make([][2]border, len(gaps))
	count := make([]uint8, len(gaps))
	// wants reports whether gap sample g can still take a border sample
	// of plate k.
	wants := func(g int32, k int32) bool {
		return count[g] == 0 || count[g] == 1 && next.Plate[near[g][0].site] != k
	}

	// Border samples spread outward through the gap, nearest first.
	h := make(gapHeap, 0, 4*len(gaps))
	for _, i := range gaps {
		for _, j := range mesh.Neighbours(int(i)) {
			if next.Plate[j] >= 0 {
				h.push(gapItem{sphere.Angle(mesh.Points[i], mesh.Points[j]), j, i})
			}
		}
	}
	for len(h) > 0 {
		it := h.pop()
		g, k := slot[it.sample], next.Plate[it.site]
		if !wants(g, k) {
			continue
		}
		near[g][count[g]] = border{it.site, it.angle}
		count[g]++
		for _, j := range mesh.Neighbours(int(it.sample)) {
			if gj := slot[j]; gj >= 0 && wants(gj, k) {
				h.push(gapItem{sphere.Angle(mesh.Points[j], mesh.Points[it.site]), it.site, j})
			}
		}
	}

	plate := make([]int32, len(gaps))
	spread := make([]sphere.Vec, len(gaps)) // from the ridge toward the nearest plate
	for g, i := range gaps {
		next.CrustType[i] = model.Oceanic
		next.Thickness[i] = oceanicThickness
		// Everything else is zero already.
		if count[g] == 0 {
			// No plate holds anything at all.
			next.Elevation[i] = p.RidgeElevation
			continue
		}
		a, b := near[g][0], near[g][1]
		if count[g] == 2 && (b.angle < a.angle || b.angle == a.angle && b.site < a.site) {
			a, b = b, a
		}
		plate[g] = next.Plate[a.site]
		// New oceanic crust is not above sea level.
		alpha, between := 1.0, min(next.Elevation[a.site], 0)
		if count[g] == 2 {
			alpha = (b.angle - a.angle) / (b.angle + a.angle)
			between = (b.angle*between + a.angle*min(next.Elevation[b.site], 0)) / (a.angle + b.angle)
			// Away from b and toward a, each as a unit tangent vector.
			q := mesh.Points[i]
			fromB := q.Sub(mesh.Points[b.site]).Tangent(q)
			toA := mesh.Points[a.site].Sub(q).Tangent(q)
			if fromB.Len() >= 1e-9 && toA.Len() >= 1e-9 {
				if s := fromB.Normalize().Add(toA.Normalize()); s.Len() >= 1e-9 {
					spread[g] = s.Normalize()
				}
			}
		}
		age := alpha * elapsed
		next.OceanicAge[i] = age
		next.Elevation[i] = alpha*between + (1-alpha)*RidgeProfile(p, age)
	}
	for g, i := range gaps {
		next.Plate[i] = plate[g]
	}

	// Ridge directions. The nearest border samples are scattered along
	// the uneven edge of a plate, so the directions between them are
	// evened out over neighbouring gap samples first.
	smoothed := make([]sphere.Vec, len(gaps))
	for range ridgeSmoothing {
		for g, i := range gaps {
			own := spread[g]
			sum := own
			for _, j := range mesh.Neighbours(int(i)) {
				if gj := slot[j]; gj >= 0 {
					// The direction turns round across a ridge.
					if v := spread[gj]; v.Dot(own) >= 0 {
						sum = sum.Add(v)
					} else {
						sum = sum.Sub(v)
					}
				}
			}
			smoothed[g] = sphere.Vec{}
			if s := sum.Tangent(mesh.Points[i]); own != (sphere.Vec{}) && s.Len() >= 1e-9 {
				smoothed[g] = s.Normalize()
			}
		}
		spread, smoothed = smoothed, spread
	}
	for g, i := range gaps {
		if r := spread[g].Cross(mesh.Points[i]); r.Len() >= 1e-9 {
			next.RidgeDirection[i] = r.Normalize()
		}
	}
}

// ridgeSmoothing is the number of times fillGaps averages the direction
// of spreading over neighbouring samples.
const ridgeSmoothing = 3

// gapItem is a border sample offered to a gap sample.
type gapItem struct {
	angle  float64 // between the two, in radians
	site   int32   // the border sample
	sample int32   // the gap sample
}

// less orders items by angle, then by border sample, then by gap
// sample, so that no two different items are equal.
func (a gapItem) less(b gapItem) bool {
	if a.angle != b.angle {
		return a.angle < b.angle
	}
	if a.site != b.site {
		return a.site < b.site
	}
	return a.sample < b.sample
}

// gapHeap is a binary min-heap of gapItems.
type gapHeap []gapItem

func (h *gapHeap) push(it gapItem) {
	s := append(*h, it)
	i := len(s) - 1
	for i > 0 {
		parent := (i - 1) / 2
		if !s[i].less(s[parent]) {
			break
		}
		s[i], s[parent] = s[parent], s[i]
		i = parent
	}
	*h = s
}

func (h *gapHeap) pop() gapItem {
	s := *h
	top := s[0]
	last := len(s) - 1
	s[0] = s[last]
	s = s[:last]
	for i := 0; ; {
		l, r, least := 2*i+1, 2*i+2, i
		if l < last && s[l].less(s[least]) {
			least = l
		}
		if r < last && s[r].less(s[least]) {
			least = r
		}
		if least == i {
			break
		}
		s[i], s[least] = s[least], s[i]
		i = least
	}
	*h = s
	return top
}
