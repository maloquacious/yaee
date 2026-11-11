// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Constants of subduction that the paper leaves to the implementer
// (§4.1). None is in Config; milestone 9 tunes them.
const (
	// controlFraction is the control distance d_0 of the distance
	// transfer f, as a fraction of the subduction distance r_s: 300 km
	// with the default r_s of 1800 km.
	controlFraction = 1.0 / 6

	// foldWeight is β, in My/km: the fold direction of a sample gains
	// β times the relative velocity of the plates, in km/My, per My. With
	// 0.01, the length of a fold direction is the convergence the sample
	// has recorded, in units of 100 km.
	foldWeight = 0.01

	// slabPull is ε, per km of subduction front and per My: the length
	// added to a plate's unit rotation axis by 1 km of front in 1 My. A
	// front of 5000 km adds 0.005 per My, which turns the axis most of
	// the way toward the pull in about 200 My.
	slabPull = 1e-6

	// trenchWidth is the width in km of the oceanic trench, measured on
	// the subducting plate from the subduction front.
	trenchWidth = 150.0

	// edgeReach is the distance, in sample spacings, over which the
	// outward direction of a plate's edge is measured (Fronts), and
	// convergence the least share of the plates' relative speed that must
	// be directed into the edge for it to be a subduction front: 0.3 is
	// convergence within 72.5° of head-on. Over 4 spacings the direction
	// measured for a straight edge is within 11° of the true one (a share
	// of 0.19); over the nearest neighbours alone it is up to 33° off.
	edgeReach   = 4.0
	convergence = 0.3
)

// Overrides reports whether crust A stays on top where it meets crust B
// of another plate, so that B is the crust that subducts (§4.1):
//
//   - continental crust overrides oceanic crust;
//   - of two oceanic crusts, the crust of the plate whose oceanic crust
//     is younger overrides (oceanA < oceanB), and at equal age the crust
//     of the lower-numbered plate overrides;
//   - of two continental crusts, the crust of the lower-numbered plate
//     overrides. This is the paper's "partial subduction" of a
//     continental-continental configuration; until continental collision
//     exists (issue #7) the other plate's continental crust is consumed
//     like oceanic crust.
//
// typeA is the crust type at the place where the two meet, plateA the
// number of the plate that holds the crust, and oceanA that plate's
// oceanic age: the mean oceanic age of its oceanic samples at the last
// resampling (World.PlateOceanAge). The paper says "the older plate"
// subducts. An age for the whole plate, not the age at the contact,
// keeps the choice the same all along a boundary and from step to step.
// With ages compared sample by sample, two plates of mixed ages each
// consumed parts of the other and left holes, which filled with new
// crust younger than both.
//
// For two different plates exactly one of Overrides(A, B) and
// Overrides(B, A) is true, and the relation orders any number of crusts
// of different plates, so three plates meeting at one place need no
// further rule: the crust that overrides both others stays.
func Overrides(typeA uint8, oceanA float64, plateA int32, typeB uint8, oceanB float64, plateB int32) bool {
	if typeA != typeB {
		return typeA == model.Continental
	}
	if typeA == model.Oceanic && oceanA != oceanB {
		return oceanA < oceanB
	}
	return plateA < plateB
}

// PlateOceanAges returns, for each plate, the mean oceanic age in My of
// its samples of oceanic crust, summed in order of sample number. It is
// 0 for a plate without oceanic crust.
func PlateOceanAges(w *model.World) []float64 {
	sum := make([]float64, w.NumPlates)
	count := make([]int, w.NumPlates)
	for i, k := range w.Plate {
		if w.CrustType[i] == model.Oceanic {
			sum[k] += w.OceanicAge[i]
			count[k]++
		}
	}
	for k, n := range count {
		if n > 0 {
			sum[k] /= float64(n)
		}
	}
	return sum
}

// RelativeSpeed returns v(p) = ‖s_i(p) − s_j(p)‖ in km/My: the speed at
// which the surfaces of plates i and j move past each other at the unit
// position p (§4.1).
func RelativeSpeed(w *model.World, i, j int, p sphere.Vec) float64 {
	return Velocity(w, i, p).Sub(Velocity(w, j, p)).Len()
}

// DistanceTransfer returns f(d), the share of the uplift that reaches a
// point d km behind the subduction front, for the subduction distance rs
// in km (§4.1, Figure 6). The paper says only that f is a piecewise cubic
// with a maximum at a control distance that fades to 0 at r_s. Ours:
//
//	d_0 = r_s / 6
//	f(d) = 3t² − 2t³,      t = d / d_0,               for 0 ≤ d ≤ d_0
//	f(d) = 1 − 3t² + 2t³,  t = (d − d_0)/(r_s − d_0), for d_0 ≤ d ≤ r_s
//	f(d) = 0                                          otherwise
//
// It is 0 at the front, 1 at d_0, 0 at r_s, between 0 and 1 everywhere,
// and has no jump in slope.
func DistanceTransfer(d, rs float64) float64 {
	if !(d > 0 && d < rs) {
		return 0
	}
	d0 := rs * controlFraction
	if d <= d0 {
		return smoothstep(d / d0)
	}
	return 1 - smoothstep((d-d0)/(rs-d0))
}

// SpeedTransfer returns g(v) = v / v_0 for the relative speed v and the
// maximum plate speed v0, both in km/My (§4.1). It is not limited to 1:
// two plates each at v_0 can meet at 2 v_0. It is 0 if v0 is 0.
func SpeedTransfer(v, v0 float64) float64 {
	if v0 <= 0 {
		return 0
	}
	return v / v0
}

// HeightTransfer returns h(z̃) = z̃², with z̃ = (z − z_t)/(z_c − z_t), for
// the elevation z in km of the crust that is subducting (§4.1). z̃ is
// limited to the range 0 to 1 first.
func HeightTransfer(p model.Params, z float64) float64 {
	n := (z - p.TrenchElevation) / (p.MaxContinentalElevation - p.TrenchElevation)
	n = max(0, min(1, n))
	return n * n
}

// Subduction is the working state of subduction between two
// resamplings (§4.1): which samples lie near a plate boundary, where the
// subduction fronts are, and how far each sample of an overriding plate
// is from its front.
//
// A step of the simulation calls, in order, Advance, Fronts, Uplift,
// Trenches, and SlabPull. Reset must be called after every Resample.
type Subduction struct {
	spacing float64 // km between neighbouring samples, on average

	// As the plates lay at the last resampling.
	near   [2]nearPlate // the two nearest other plates of each sample
	land   []float64    // km; no continental sample is nearer to the sample than this
	centre []sphere.Vec // plateCaps
	angle  []float64    // the cap's angle in radians; -1 for an empty plate
	travel float64      // km; no sample has moved further since the last resampling

	next []bool // scratch for World.Subducted

	// Fronts, per overriding plate and per sample.
	front    [][]int32    // the front samples of each plate, in order of sample number
	under    []int32      // per sample: the plate subducting beneath a front sample, or −1
	slab     []float64    // per sample: the elevation in km of the crust subducting beneath a front sample
	lost     []bool       // per plate: a sample of the plate was subducted since its distances were computed
	dist2    []float64    // per sample: squared chord to the nearest front sample of its plate, +Inf if none within r_s
	site     []int32      // per sample: that front sample's place in front[plate], or −1
	reached  [][]int32    // per plate: the samples with a site
	pulls    [][]int32    // per subducting plate: the front samples it subducts beneath
	trench2  []float64    // per sample: squared chord to the nearest front, +Inf outside a trench
	trenchAt []int32      // scratch of spread
	scratch  [][]int32    // per plate
	heaps    []spreadHeap // per plate
}

// nearPlate is, for each sample, one of the plates near it, and a
// distance in km that no sample of that plate is nearer than. plate is
// −1 where there is none.
type nearPlate struct {
	plate []int32
	km    []float64
}

// clear returns to(q, i) for a sample i: a distance in km within which,
// at the last resampling, no sample of plate q lay. It is 0 for a sample
// of plate q.
func (s *Subduction) clear(w *model.World, q int32, i int32) float64 {
	switch q {
	case w.Plate[i]:
		return 0
	case s.near[0].plate[i]:
		return s.near[0].km[i]
	}
	// The second is the nearest of all the rest.
	return s.near[1].km[i]
}

// NewSubduction returns the state for w, whose plates must not have
// moved since the last resampling.
func NewSubduction(w *model.World) *Subduction {
	n := len(w.Mesh.Points)
	s := &Subduction{
		spacing:  sphere.SampleSpacing(w.Radius, n),
		land:     make([]float64, n),
		next:     make([]bool, n),
		front:    make([][]int32, w.NumPlates),
		under:    make([]int32, n),
		slab:     make([]float64, n),
		lost:     make([]bool, w.NumPlates),
		dist2:    make([]float64, n),
		site:     make([]int32, n),
		reached:  make([][]int32, w.NumPlates),
		pulls:    make([][]int32, w.NumPlates),
		trench2:  make([]float64, n),
		trenchAt: make([]int32, n),
		scratch:  make([][]int32, w.NumPlates),
		heaps:    make([]spreadHeap, w.NumPlates),
	}
	for i := range n {
		s.dist2[i], s.site[i], s.under[i] = math.Inf(1), -1, -1
		s.trench2[i], s.trenchAt[i] = math.Inf(1), -1
	}
	for c := range s.near {
		s.near[c] = nearPlate{make([]int32, n), make([]float64, n)}
	}
	s.Reset(w)
	return s
}

// Reset prepares the state for the plates as a resampling has just left
// them. No front is known afterwards until Fronts is called.
func (s *Subduction) Reset(w *model.World) {
	if len(w.PlateOceanAge) != w.NumPlates {
		w.PlateOceanAge = PlateOceanAges(w)
	}
	mesh := w.Mesh
	n := len(mesh.Points)

	// A sample h steps away is at least (h − 1) · 0.6 spacings from the
	// nearest: a straight line that long crosses fewer than h rows of
	// triangles.
	km := func(h int32) float64 { return float64(max(0, h-1)) * 0.6 * s.spacing }
	var wg sync.WaitGroup
	wg.Go(func() {
		// The nearest two other plates of each sample, by the number of
		// neighbour-to-neighbour steps to a sample that has a neighbour in
		// the plate. Plates spread outward from the boundaries, nearest
		// first; a sample keeps the first two it is offered.
		//
		// A sample h steps away is at least (h − 1) · 0.6 spacings from any
		// sample of the plate: a straight line that long crosses fewer than
		// h rows of triangles.
		type item struct{ sample, plate, hops int32 }
		for c := range s.near {
			for i := range n {
				s.near[c].plate[i], s.near[c].km[i] = -1, math.Inf(1)
			}
		}
		offer := func(queue []item, i, plate, hops int32) []item {
			if plate == w.Plate[i] || s.near[0].plate[i] == plate || s.near[1].plate[i] >= 0 {
				return queue
			}
			c := 0
			if s.near[0].plate[i] >= 0 {
				c = 1
			}
			s.near[c].plate[i], s.near[c].km[i] = plate, km(hops)
			return append(queue, item{i, plate, hops})
		}
		queue := make([]item, 0, 2*n)
		for i := range n {
			for _, j := range mesh.Neighbours(i) {
				queue = offer(queue, int32(i), w.Plate[j], 0)
			}
		}
		for head := 0; head < len(queue); head++ {
			it := queue[head]
			for _, j := range mesh.Neighbours(int(it.sample)) {
				queue = offer(queue, j, it.plate, it.hops+1)
			}
		}
	})
	wg.Go(func() {
		// The same for the nearest continental crust of any plate.
		hops := make([]int32, n)
		samples := make([]int32, 0, n)
		for i := range n {
			hops[i] = -1
			if w.CrustType[i] == model.Continental {
				hops[i] = 0
				samples = append(samples, int32(i))
			}
		}
		for head := 0; head < len(samples); head++ {
			i := samples[head]
			for _, j := range mesh.Neighbours(int(i)) {
				if hops[j] < 0 {
					hops[j] = hops[i] + 1
					samples = append(samples, j)
				}
			}
		}
		for i, h := range hops {
			s.land[i] = math.Inf(1)
			if h >= 0 {
				s.land[i] = km(h)
			}
		}
	})
	wg.Go(func() {
		var cosine []float64
		s.centre, cosine = plateCaps(w)
		s.angle = make([]float64, len(cosine))
		for k, c := range cosine {
			s.angle[k] = -1
			if !math.IsInf(c, 1) {
				s.angle[k] = math.Acos(max(-1, min(1, c)))
			}
		}
	})
	wg.Wait()
	s.travel = 0

	for k := range s.front {
		s.clearDistances(k)
		s.front[k] = s.front[k][:0]
		s.pulls[k] = s.pulls[k][:0]
		s.lost[k] = false
	}
	for i := range s.under {
		s.under[i] = -1
	}
}

// clearDistances forgets the front distances of plate k.
func (s *Subduction) clearDistances(k int) {
	for _, i := range s.reached[k] {
		s.dist2[i], s.site[i] = math.Inf(1), -1
	}
	s.reached[k] = s.reached[k][:0]
}

// Front returns the samples of plate k that form its subduction fronts,
// in order of sample number, as the latest call of Fronts found them.
// The caller must not change the slice.
func (s *Subduction) Front(k int) []int32 { return s.front[k] }

// Under returns the plate that is subducting beneath the front sample i,
// or −1 if i is not a front sample.
func (s *Subduction) Under(i int) int { return int(s.under[i]) }

// FrontDistance returns d(p) for sample i: the distance in km from the
// sample to the subduction front of its plate, and the front sample it
// was measured to. ok is false if the sample is not in a subduction
// region: its plate has no front within the subduction distance r_s of
// it that can be reached through the plate, or the sample is subducted.
//
// The distance is the great-circle distance to a front sample, found by
// spreading the front samples outward through the plate's samples,
// nearest first, as sphere.Mesh.Distances does. It is at least the
// distance to the nearest front sample, and no more than half a sample
// spacing above it. A front sample is up to 1.5 sample spacings behind
// the true edge of its plate, so d(p) is within 1.5 sample spacings of
// the true distance to the front. The paper instead overestimates d step by step (§6, "Plate
// boundary tracking"); ours is computed afresh from the neighbour lists
// whenever a plate's front changes.
func (s *Subduction) FrontDistance(w *model.World, i int) (km float64, frontSample int32, ok bool) {
	k := s.site[i]
	if k < 0 || w.Subducted[i] {
		return 0, -1, false
	}
	return chordToKm(s.dist2[i], w.Radius), s.front[w.Plate[i]][k], true
}

// chordToKm converts the squared chord between two unit vectors to the
// great-circle distance between them on a sphere of the given radius.
func chordToKm(chord2, radius float64) float64 {
	return 2 * math.Asin(min(1, math.Sqrt(chord2)/2)) * radius
}

// kmToChord2 is the inverse of chordToKm.
func kmToChord2(km, radius float64) float64 {
	c := 2 * math.Sin(min(math.Pi, km/radius)/2)
	return c * c
}

// Advance moves every plate through dt My, as the function Advance does,
// and marks as subducted (World.Subducted) every sample whose crust was
// beneath overriding crust of another plate at any time during the step
// (§4.1; issue #23).
//
// A sample i of plate P goes beneath plate Q when Q holds, at the place
// where i is, crust that Overrides the crust of i. Q's crust there is
// that of the sample of Q that owns the place (Query): its crust type
// is compared with that of i, and the plates as Overrides says.
// Crust that is already subducted overrides nothing.
//
// The test is made where the sample is at the end of the step and, when
// the sample has moved more than half a sample spacing across Q during
// the step, at points no more than half a sample spacing apart along the
// way. A fragment of a plate narrower than the distance it moves in a
// step therefore consumes everything it passes over, and leaves one
// continuous wake behind it, not a row of separate footprints.
//
// This is the paper's subduction "at every time step". The paper tracks
// the intersections of the plates' boundary triangles (§6); we test
// samples with the plate query. See PLAN.md, "Finding what is at a
// point".
//
// The marks made depend only on the world at the start of the step, not
// on the order in which samples are examined.
func (s *Subduction) Advance(ctx context.Context, w *model.World, dt float64) error {
	mesh := w.Mesh
	plates := w.NumPlates
	speed := maxSpeed(w)

	// The rotations at the start and end of the step, and at slices
	// evenly spaced between: rot[0] is the start, rot[parts] the end.
	half := s.spacing / 2
	parts := max(1, min(64, int(math.Ceil(2*speed*dt/half))))
	rot := make([][]sphere.Rotation, parts+1)
	inv := make([][]sphere.Rotation, parts+1)
	for j := range rot {
		rot[j] = make([]sphere.Rotation, plates)
		inv[j] = make([]sphere.Rotation, plates)
		for k := range plates {
			r := w.PlateRotation[k]
			if j > 0 {
				r = r.Then(sphere.NewRotation(w.PlateAxis[k], w.PlateSpeed[k]*dt*float64(j)/float64(parts)))
			}
			rot[j][k], inv[j][k] = r, r.Inverse()
		}
	}
	Advance(w, dt)
	s.travel += speed * dt
	if speed == 0 {
		return ctx.Err()
	}
	now, back := rot[parts], inv[parts]

	// Bounding caps, widened by the furthest two plates can move past
	// each other in the step.
	centre := make([]sphere.Vec, plates)
	cosine := make([]float64, plates)
	for k := range plates {
		centre[k] = now[k].Apply(s.centre[k])
		cosine[k] = math.Inf(1)
		if s.angle[k] >= 0 {
			cosine[k] = math.Cos(min(math.Pi, s.angle[k]+2*speed*dt/w.Radius)) - 1e-12
		}
	}

	reach := 2*s.travel + s.spacing
	err := model.ParallelRange(ctx, len(mesh.Points), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			s.next[i] = w.Subducted[i]
			if w.Subducted[i] || s.near[0].km[i] > reach {
				continue
			}
			p := w.Plate[i]
			at := mesh.Points[i]
			x0, x1 := rot[0][p].Apply(at), now[p].Apply(at)
		plates:
			for q := range int32(plates) {
				if q == p || !(x1.Dot(centre[q]) >= cosine[q]) {
					continue
				}
				// Where the sample is in the frame of plate q: at the
				// start of the step, and at its end.
				b0, b1 := inv[0][q].Apply(x0), back[q].Apply(x1)
				if (max(b0.Sub(at).Len(), b1.Sub(at).Len())*w.Radius + s.spacing) < s.clear(w, q, int32(i)) {
					continue // plate q held nothing this near
				}
				// Whether oceanic crust of q overrides the sample, and
				// whether continental crust of q does.
				ocean := Overrides(model.Oceanic, w.PlateOceanAge[q], q, w.CrustType[i], w.PlateOceanAge[p], p)
				if !ocean && !Overrides(model.Continental, w.PlateOceanAge[q], q, w.CrustType[i], w.PlateOceanAge[p], p) {
					continue
				}
				per := b1.Sub(b0).Len() * w.Radius / float64(parts) // km per slice
				least := parts                                      // slices to go back by, at least
				if per*float64(parts) > half {
					least = max(1, int(half/per))
				}
				for j := parts; j >= 1; {
					b := b1
					if j < parts {
						b = inv[j][q].Apply(rot[j][p].Apply(at))
					}
					o := ownerAt(w, b)
					// Nothing of q that overrides the sample is nearer
					// than this.
					safe := s.clear(w, q, o)
					if w.Plate[o] == q {
						if !w.Subducted[o] && (ocean || w.CrustType[o] == model.Continental) {
							s.next[i] = true
							break plates
						}
						if !ocean {
							safe = s.land[o]
						}
					} else if !ocean {
						safe = max(safe, s.land[o])
					}
					skip := least
					if safe -= 1.5 * s.spacing; safe > per {
						skip = max(skip, int(min(float64(parts), safe/per)))
					}
					j -= skip
				}
			}
		}
	})
	if err != nil {
		return err
	}
	for i, gone := range s.next {
		if gone && !w.Subducted[i] {
			s.lost[w.Plate[i]] = true
		}
	}
	w.Subducted, s.next = s.next, w.Subducted
	return nil
}

// Fronts finds the subduction fronts as the plates now lie, and the
// distance of every sample of an overriding plate to its front (§4.1;
// issues #23 and #24).
//
// A sample i of plate Q is a front sample, with plate P subducting
// beneath it, when all of these hold:
//
//   - i is not subducted, and has a neighbour that is subducted or is in
//     another plate: it is on the edge of what Q holds;
//   - the place where i now is belonged to P at the last resampling (the
//     sample of P that owns it may have been subducted since), and the
//     crust of P there is subducted or is crust that the crust of i
//     Overrides;
//   - P moves into Q there: (s_P − s_Q) · n < −convergence · ‖s_P − s_Q‖,
//     with s the velocities of the plates at the place and n the outward
//     direction of Q's edge at i, of length 1: the sum, scaled, of the
//     directions from i to every sample within edgeReach sample spacings
//     of it, and joined to it through such samples, that is subducted or
//     is in another plate. This leaves out
//     the trailing edge of a fragment that is crossing P, and plates
//     that slide past each other: an edge is jagged at the scale of one
//     sample, and its teeth do overlap the other plate as they slide.
//
// If several plates qualify, P is the lowest-numbered. The elevation of
// the subducting crust at the front, z_i of the height transfer, is the
// elevation of the sample of P that owns the place, without its trench.
//
// A front therefore appears once P has gone about half a sample spacing
// beneath Q since the last resampling, and plates that move apart or
// slide past each other have none.
func (s *Subduction) Fronts(ctx context.Context, w *model.World) error {
	mesh := w.Mesh
	plates := w.NumPlates
	centre := make([]sphere.Vec, plates)
	cosine := make([]float64, plates)
	inverse := make([]sphere.Rotation, plates)
	for k := range plates {
		centre[k] = w.PlateRotation[k].Apply(s.centre[k])
		inverse[k] = w.PlateRotation[k].Inverse()
		cosine[k] = math.Inf(1)
		if s.angle[k] >= 0 {
			cosine[k] = math.Cos(s.angle[k]) - 1e-12
		}
	}
	reach2 := kmToChord2(edgeReach*s.spacing, w.Radius)
	err := model.ParallelRange(ctx, len(mesh.Points), func(lo, hi int) {
		var ring []int32
		for i := lo; i < hi; i++ {
			s.under[i] = -1
			if w.Subducted[i] {
				continue
			}
			q := w.Plate[i]
			at := mesh.Points[i]
			edge := false
			for _, j := range mesh.Neighbours(i) {
				if w.Plate[j] != q || w.Subducted[j] {
					edge = true
					break
				}
			}
			if !edge {
				continue
			}
			// The outward direction, over the samples within edgeReach
			// spacings: ring holds them, each once, in the order found.
			var out sphere.Vec
			ring = append(ring[:0], int32(i))
			for head := 0; head < len(ring); head++ {
				for _, j := range mesh.Neighbours(int(ring[head])) {
					d := mesh.Points[j].Sub(at)
					if d.Dot(d) > reach2 || slices.Contains(ring, j) {
						continue
					}
					ring = append(ring, j)
					if w.Plate[j] != q || w.Subducted[j] {
						out = out.Add(d)
					}
				}
			}
			x := w.PlateRotation[q].Apply(at)
			out = w.PlateRotation[q].Apply(out.Tangent(at))
			if out.Len() < 1e-12 {
				continue
			}
			out = out.Normalize()
			for p := range int32(plates) {
				if p == q || !(x.Dot(centre[p]) >= cosine[p]) {
					continue
				}
				o := ownerAt(w, inverse[p].Apply(x))
				if w.Plate[o] != p {
					continue
				}
				if !w.Subducted[o] && !Overrides(w.CrustType[i], w.PlateOceanAge[q], q, w.CrustType[o], w.PlateOceanAge[p], p) {
					continue
				}
				rel := Velocity(w, int(p), x).Sub(Velocity(w, int(q), x))
				if !(rel.Dot(out) < -convergence*rel.Len()) {
					continue
				}
				s.under[i] = p
				s.slab[i] = w.Elevation[o] + w.Trench[o]
				break
			}
		}
	})
	if err != nil {
		return err
	}

	// Gather the fronts of each plate, and for each subducting plate the
	// front samples it goes beneath, both in order of sample number.
	fronts := make([][]int32, plates)
	for k := range plates {
		fronts[k] = s.scratch[k][:0]
		s.pulls[k] = s.pulls[k][:0]
	}
	for i, p := range s.under {
		if p >= 0 {
			fronts[w.Plate[i]] = append(fronts[w.Plate[i]], int32(i))
			s.pulls[p] = append(s.pulls[p], int32(i))
		}
	}
	changed := make([]bool, plates)
	for k := range plates {
		changed[k] = s.lost[k] || !slices.Equal(fronts[k], s.front[k])
		s.front[k], s.scratch[k] = fronts[k], s.front[k]
		s.lost[k] = false
	}

	// Distances within each plate whose front has changed.
	limit := kmToChord2(w.SubductionDistance, w.Radius)
	eachPlate(plates, func(k int) {
		if !changed[k] {
			return
		}
		s.clearDistances(k)
		front := s.front[k]
		if len(front) == 0 {
			return
		}
		s.reached[k] = spread(mesh, &s.heaps[k], len(front),
			func(src int) sphere.Vec { return mesh.Points[front[src]] },
			func(src int, seed func(int32)) { seed(front[src]) },
			func(j int32) bool { return w.Plate[j] == int32(k) && !w.Subducted[j] },
			limit, s.dist2, s.site, s.reached[k])
	})
	return ctx.Err()
}

// Uplift raises the overriding plates behind their subduction fronts
// through a step of dt My, and updates fold direction and orogeny (§4.1;
// issue #24). Fronts must have been called since the plates last moved.
//
// First the orogeny age of every sample whose orogeny type is not none
// increases by dt. Then, for every sample p that is in a subduction
// region (FrontDistance), with Q its plate, d its distance from the
// front, and P the plate subducting beneath the front sample it was
// measured to:
//
//   - the uplift is ũ = u_0 · f(d) · g(v) · h(z̃), with f, g, and h the
//     functions DistanceTransfer, SpeedTransfer, and HeightTransfer; v
//     the relative speed of P and Q where p now is; and z̃ from the
//     elevation of the subducting crust at that front sample (Fronts).
//     The paper takes z_i(p), the elevation of the subducting plate at p
//     itself, which does not exist once p is further inland than the
//     slab has reached;
//   - the elevation becomes z + ũ · (1 − max(z, 0)/z_c) · dt, and no more
//     than z_c. The factor is ours: the paper gives no limit, and its
//     maximum uplift is 20 times its maximum erosion. Uplift fades as
//     land nears the highest continental altitude z_c, so mountains
//     approach z_c and do not form a flat top at it;
//   - World.Uplift is the rate applied, ũ · (1 − max(z, 0)/z_c), in
//     km/My;
//   - the fold direction gains β · (s_P − s_Q) · dt, with β the constant
//     foldWeight and s the velocities of the plates where p now is, and
//     is then projected onto the tangent plane at p. It is kept, like
//     every direction, as the plate lay at the last resampling;
//   - if ũ > 0 and the new elevation is above sea level, and either the
//     old elevation was not (the crust emerges) or the sample's orogeny
//     type is none, the orogeny type becomes Andean and the orogeny age
//     0. Crust type is not changed: oceanic crust that emerges stays
//     oceanic.
//
// World.Uplift is 0 for every other sample.
func (s *Subduction) Uplift(ctx context.Context, w *model.World, dt float64) error {
	mesh := w.Mesh
	inverse := make([]sphere.Rotation, w.NumPlates)
	for k, r := range w.PlateRotation {
		inverse[k] = r.Inverse()
	}
	return model.ParallelRange(ctx, len(mesh.Points), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if w.OrogenyType[i] != model.OrogenyNone {
				w.OrogenyAge[i] += dt
			}
			w.Uplift[i] = 0
			if s.site[i] < 0 || w.Subducted[i] {
				continue
			}
			q := w.Plate[i]
			fs := s.front[q][s.site[i]]
			p := s.under[fs]
			x := w.PlateRotation[q].Apply(mesh.Points[i])
			rel := Velocity(w, int(p), x).Sub(Velocity(w, int(q), x))
			d := chordToKm(s.dist2[i], w.Radius)
			u := w.SubductionUplift * DistanceTransfer(d, w.SubductionDistance) *
				SpeedTransfer(rel.Len(), w.MaxPlateSpeed) * HeightTransfer(w.Params, s.slab[fs])

			w.FoldDirection[i] = w.FoldDirection[i].Add(inverse[q].Apply(rel).Scale(foldWeight * dt)).Tangent(mesh.Points[i])

			if !(u > 0) {
				continue
			}
			z := w.Elevation[i]
			u *= max(0, 1-max(z, 0)/w.MaxContinentalElevation)
			rose := min(z+u*dt, max(z, w.MaxContinentalElevation))
			w.Elevation[i] = rose
			w.Uplift[i] = u
			if u > 0 && rose > 0 && (z <= 0 || w.OrogenyType[i] == model.OrogenyNone) {
				w.OrogenyType[i] = model.OrogenyAndean
				w.OrogenyAge[i] = 0
			}
		}
	})
}

// SlabPull turns the rotation axis of every subducting plate toward its
// subduction fronts through a step of dt My (§4.1, "Slab pull"; issue
// #25). Fronts must have been called since the plates last moved.
//
// For a plate P with front samples q_k (the front samples of other
// plates that P is subducting beneath, where they now are) and centroid
// c (the mean position of the plate's samples at the last resampling,
// scaled to length 1 and turned with the plate):
//
//	w ← w + ε · l · Σ_k (c × q_k)/‖c × q_k‖ · dt, then scaled to length 1
//
// with ε the constant slabPull and l the sample spacing in km. Turning
// about c × q_k carries c toward q_k. The factor l is ours: the number
// of front samples along a front of a given length grows as the spacing
// shrinks, and with it the pull is that of the front's length in km. A
// term with ‖c × q_k‖ below 10⁻⁹ is left out, and an axis that the sum
// would cancel exactly is left as it was.
//
// The angular speed of each plate whose axis changed is then scaled so
// that the plate's fastest sample moves exactly as fast as it did
// before: slab pull changes where a plate is going, not how fast. The
// paper changes the axis alone. A plate that lies near its axis turns
// with a high angular speed, and an axis pulled away from it would
// otherwise carry the plate up to the limit MaxPlateSpeed; over a run
// nearly every subducting plate ended there. No sample moves faster
// than MaxPlateSpeed afterwards, since none did before.
func (s *Subduction) SlabPull(w *model.World, dt float64) {
	var before []float64
	pulled := make([]bool, len(s.pulls))
	for p, front := range s.pulls {
		if len(front) == 0 {
			continue
		}
		c := w.PlateRotation[p].Apply(s.centre[p])
		var sum sphere.Vec
		for _, i := range front {
			x := w.PlateRotation[w.Plate[i]].Apply(w.Mesh.Points[i])
			if t := c.Cross(x); t.Len() >= 1e-9 {
				sum = sum.Add(t.Normalize())
			}
		}
		axis := w.PlateAxis[p].Add(sum.Scale(slabPull * s.spacing * dt))
		if axis.Len() < 1e-9 || sum == (sphere.Vec{}) {
			continue
		}
		if before == nil {
			before = fastestSines(w)
		}
		w.PlateAxis[p] = axis.Normalize()
		pulled[p] = true
	}
	if before == nil {
		return
	}
	for p, m := range fastestSines(w) {
		if pulled[p] && m > 0 {
			w.PlateSpeed[p] *= before[p] / m
		}
	}
}

// Trenches forms the oceanic trench in front of every subduction front
// and fills trenches with sediment, through a step of dt My (§4.1, §4.5;
// issue #26). Fronts must have been called since the plates last moved.
//
// The paper names the trench and its depth z_t and gives no rule for
// forming it. Ours keeps, for every sample, how far it has been lowered
// into a trench: World.Trench, T ≥ 0 in km. A sample is in a trench
// while T > 0, and z + T is its elevation without the trench.
//
//  1. Sediment accretion (§4.5): every sample in a trench rises by
//     ε_t · dt, or by T if that is less, and T falls by the same amount.
//     A trench that is no longer being formed therefore fills until it
//     is gone, and no further.
//
//  2. Forming: for every sample of oceanic crust that is not subducted,
//     on a plate P, within the constant trenchWidth w of a front sample
//     that P is subducting beneath, at the distance d from the nearest
//     such front sample (measured as FrontDistance is, through the
//     samples of P):
//
//     T ← max(T, (z + T − z_t) · (1 − d/w)²)
//
//     and z falls by the amount T grew. At the front the trench reaches
//     the trench elevation z_t exactly, and nowhere is it deeper.
//
// Step 2 follows step 1, so an active trench always has its full
// profile whatever the time step, and accretion shows only where a
// trench is no longer being formed. Continental crust has no trench.
func (s *Subduction) Trenches(ctx context.Context, w *model.World, dt float64) error {
	mesh := w.Mesh
	fill := w.SedimentAccretion * dt
	err := model.ParallelRange(ctx, len(mesh.Points), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if t := w.Trench[i]; t > 0 {
				f := min(t, fill)
				w.Elevation[i] += f
				w.Trench[i] = t - f
			}
		}
	})
	if err != nil {
		return err
	}
	inverse := make([]sphere.Rotation, w.NumPlates)
	for k, r := range w.PlateRotation {
		inverse[k] = r.Inverse()
	}
	limit := kmToChord2(trenchWidth, w.Radius)
	eachPlate(w.NumPlates, func(p int) {
		front := s.pulls[p]
		if len(front) == 0 {
			return
		}
		// A front sample, in the frame of the subducting plate.
		at := func(src int) sphere.Vec {
			i := front[src]
			return inverse[p].Apply(w.PlateRotation[w.Plate[i]].Apply(mesh.Points[i]))
		}
		touched := spread(mesh, &s.heaps[p], len(front), at,
			func(src int, seed func(int32)) {
				tri, _ := w.Locator.Locate(at(src))
				for _, c := range mesh.Tri[3*tri : 3*tri+3] {
					seed(c)
				}
			},
			func(j int32) bool { return w.Plate[j] == int32(p) },
			limit, s.trench2, s.trenchAt, s.scratch[p][:0])
		for _, i := range touched {
			if !w.Subducted[i] && w.CrustType[i] == model.Oceanic {
				d := chordToKm(s.trench2[i], w.Radius)
				if share := 1 - d/trenchWidth; share > 0 {
					whole := w.Elevation[i] + w.Trench[i]
					if t := (whole - w.TrenchElevation) * share * share; t > w.Trench[i] {
						w.Trench[i] = t
						w.Elevation[i] = whole - t
					}
				}
			}
			s.trench2[i], s.trenchAt[i] = math.Inf(1), -1
		}
		s.scratch[p] = touched[:0]
	})
	return ctx.Err()
}

// Erode applies continental erosion through a step of dt My (§4.5). For
// every sample of continental crust above sea level:
//
//	z ← z − (z/z_c) · ε_c · dt
//
// with z_c the highest continental altitude and ε_c the parameter
// ContinentalErosion. This is the paper's formula, as written: the
// explicit Euler step of dz/dt = −z · ε_c/z_c, whose solution is
// z(t) = z(0) · exp(−ε_c · t/z_c). The step multiplies z by
// 1 − ε_c · dt/z_c, which is 0.994 with the defaults; after t My in steps
// of dt the result is below the solution by less than
// z(0) · t · ε_c² · dt/(2 z_c²), 0.0045 km for 10 km of land over 250 My.
// One departure: a factor below 0 is taken as 0, so that land falls to
// sea level and not below it.
//
// Continental crust at or below sea level, and oceanic crust, are not
// changed: the paper's formula would raise submerged crust.
func Erode(ctx context.Context, w *model.World, dt float64) error {
	keep := max(0, 1-w.ContinentalErosion*dt/w.MaxContinentalElevation)
	return model.ParallelRange(ctx, len(w.CrustType), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if w.CrustType[i] == model.Continental && w.Elevation[i] > 0 {
				w.Elevation[i] *= keep
			}
		}
	})
}

// eachPlate calls fn(k) once for every k from 0 to n−1, from several
// goroutines. fn(k) must write only what belongs to plate k.
func eachPlate(n int, fn func(k int)) {
	var (
		next atomic.Int64
		wg   sync.WaitGroup
	)
	for range min(runtime.GOMAXPROCS(0), n) {
		wg.Go(func() {
			for {
				k := int(next.Add(1)) - 1
				if k >= n {
					return
				}
				fn(k)
			}
		})
	}
	wg.Wait()
}

// spread finds, for every admitted sample within a distance of one of n
// sources, the nearest source. Source k is at the unit position
// pos(k), and seeds(k, seed) calls seed for each sample it starts from.
// Sources spread outward from their seeds through neighbouring admitted
// samples, nearest first, as sphere.Mesh.Distances does; a sample keeps
// the nearest source it is offered, and the lower-numbered of two at the
// same distance.
//
// Distances are squared chords between unit vectors, and limit2 is the
// greatest that is kept. On entry dist2 must be +Inf and site −1 for
// every admitted sample. On return dist2[j] and site[j] are the squared
// chord to, and the number of, the source found for sample j; the
// samples that were given one are appended to touched, which is
// returned.
func spread(mesh *sphere.Mesh, h *spreadHeap, n int, pos func(k int) sphere.Vec, seeds func(k int, seed func(int32)),
	admit func(j int32) bool, limit2 float64, dist2 []float64, site []int32, touched []int32) []int32 {
	*h = (*h)[:0]
	// offer is called only for admitted samples: the entries of dist2
	// and site for any other sample may belong to another goroutine.
	offer := func(j int32, k int32, at sphere.Vec) {
		c := mesh.Points[j].Sub(at)
		d := c.Dot(c)
		if d > limit2 || !(d < dist2[j] || d == dist2[j] && k < site[j]) {
			return
		}
		if site[j] < 0 {
			touched = append(touched, j)
		}
		dist2[j], site[j] = d, k
		h.push(spreadItem{d, j})
	}
	for k := range n {
		at := pos(k)
		seeds(k, func(j int32) {
			if admit(j) {
				offer(j, int32(k), at)
			}
		})
	}
	for len(*h) > 0 {
		it := h.pop()
		if it.dist2 > dist2[it.sample] {
			continue // superseded by a nearer source
		}
		k := site[it.sample]
		at := pos(int(k))
		for _, j := range mesh.Neighbours(int(it.sample)) {
			if admit(j) && site[j] != k {
				offer(j, k, at)
			}
		}
	}
	return touched
}

// spreadItem is a sample waiting to be spread from, with the squared
// chord it had when it was queued.
type spreadItem struct {
	dist2  float64
	sample int32
}

func (a spreadItem) less(b spreadItem) bool {
	return a.dist2 < b.dist2 || a.dist2 == b.dist2 && a.sample < b.sample
}

// spreadHeap is a binary min-heap of spreadItems.
type spreadHeap []spreadItem

func (h *spreadHeap) push(it spreadItem) {
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

func (h *spreadHeap) pop() spreadItem {
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
