// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"cmp"
	"context"
	"math"
	"math/rand/v2"
	"slices"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/noise"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Constants of the initial crust. The paper gives the parameters of the
// crust (§3, Table 1) and the elevations z_r, z_a, z_t and z_c
// (Table 3), but no initial values, so every constant here is ours.
const (
	// continentFill is the share of the continental plates' area that
	// continental crust fills. Plates are made continental until their
	// area is the land area divided by continentFill, so that a
	// continent's plates carry ocean around it, as the African and
	// South American plates do, and so that the other plates are wholly
	// oceanic, as the Pacific plate nearly is.
	continentFill = 0.7

	// The coast is the outline of a group of continental plates, moved
	// inward and roughened by fractal noise. The noise's amplitude and
	// its largest feature are these fractions of the plate size
	// √(4πR²/P), the side of the square whose area is the mean area of
	// the P plates. The amplitude decides how unlike its plates a
	// continent is, and how many islands and bays the coast has.
	coastNoiseAmplitude   = 0.5
	coastNoiseFeatureSize = 0.5
	coastNoiseOctaves     = 5
	coastNoiseLacunarity  = 2.0
	coastNoiseGain        = 0.5

	// marginWidth is the distance from the coast, in km, over which
	// the land rises to the height of the interior and the sea floor
	// falls to the abyssal plain. Earth's continental margins, from
	// shelf edge to the foot of the continental rise, are a few hundred
	// km wide.
	marginWidth = 500.0

	// Land starts low and flat: the paper's uplift raises mountains
	// from it (§4.1, §4.2) and its erosion lowers everything toward sea
	// level (§4.5). Elevation is coastElevation · z_c at the coast and
	// interiorElevation · z_c beyond the margin: 0.1 km and 0.5 km with
	// the default z_c of 10 km. Half of Earth's land is below 0.4 km.
	coastElevation    = 0.01
	interiorElevation = 0.05

	// Thickness, in km. The paper never uses the thickness e in a
	// formula, so these are the usual averages for Earth: 7 km for
	// oceanic crust and 35 km for continental crust.
	oceanicThickness     = 7.0
	continentalThickness = 35.0

	// oceanicDampening is ε_o of the paper's Table 3, in km/My (the
	// table gives 4 × 10⁻² mm/y). It is used here only to give the
	// initial ocean an age that agrees with its elevation.
	oceanicDampening = 0.04
)

// Crust is the stage that sets the initial crust: every parameter of the
// paper's Table 1, for every sample. It must run after Plates.
//
// # Which crust is continental
//
//  1. Each sample stands for the area mesh.PointAreas gives it.
//  2. Plates are taken in a random order and made continental plates
//     until their area is at least LandCoverage / continentFill of the
//     sphere, or all are taken. With LandCoverage 0 none is taken.
//  3. Every sample of a continental plate gets a score in km: its
//     distance to the nearest sample that has a neighbour across the
//     edge between continental and other plates (0 if every plate is
//     continental), plus coastNoiseAmplitude · √(4πR²/P) times fractal
//     noise whose largest features are coastNoiseFeatureSize ·
//     √(4πR²/P) across.
//  4. Those samples are made continental in order of score, highest
//     first, until the continental area is as near as it can be to
//     LandCoverage of the sphere. Every other sample is oceanic.
//
// The continental share of the sphere's area therefore differs from
// LandCoverage by less than the area of one sample: at most 1.5/N of the
// sphere for N samples.
//
// Continents are regions within plates, not whole plates. With whole
// plates a continent's coast would be a plate boundary on every side,
// and the land area could not be set more finely than a plate.
// Continental plates that touch carry one continent between them: the
// boundary between two such plates runs through land, as the boundary
// between the Eurasian and North American plates does. The distance in
// step 3 was first measured to the sample's own plate boundary; that
// left a strait along every boundary, and continents shaped like their
// plates.
//
// # Initial values
//
// With d the distance in km from a sample to the nearest sample that has
// a neighbour of the other crust type (0 for such a sample), and
// s(x) = x²(3 − 2x) for x from 0 to 1 and 1 beyond:
//
//	                  oceanic                               continental
//	thickness         7 km                                  35 km
//	elevation         z_r + (z_a − z_r) · s(d/500)          z_c · (0.01 + 0.04 · s(d/500))
//	oceanic age       (z_t/ε_o) · ln((z_a−z_t)/(z_r−z_t))   0
//	ridge direction   zero vector                           zero vector
//	orogeny age       0                                     0
//	orogeny type      none                                  none
//	fold direction    zero vector                           zero vector
//
// The ocean floor is at the abyssal plain z_a, rising to the ridge
// elevation z_r, the highest the paper gives oceanic crust, at the
// coast. Its age is the time the paper's oceanic dampening (§4.5) takes
// to lower crust from z_r to z_a, so that age and elevation agree where
// the simulation takes over: 203 My with the defaults. No ridge exists
// yet, so no sample has a ridge direction; no mountain has been raised,
// so no sample has an orogeny or a fold.
type Crust struct{}

// Name returns "crust".
func (Crust) Name() string { return "crust" }

// Run applies the stage.
func (Crust) Run(ctx context.Context, w *model.World, rng *rand.Rand) error {
	mesh := w.Mesh
	n := len(mesh.Points)
	plateSize := w.Radius * math.Sqrt(4*math.Pi/float64(w.NumPlates)) // km

	// Draw everything random first, in a fixed order.
	order := rng.Perm(w.NumPlates)
	coast := noise.NewFractal(rng.Uint64(), coastNoiseOctaves, coastNoiseLacunarity, coastNoiseGain)

	// Steps 1 and 2.
	areas := mesh.PointAreas()
	plateArea := make([]float64, w.NumPlates)
	var sphereArea float64
	for i, a := range areas {
		plateArea[w.Plate[i]] += a
		sphereArea += a
	}
	target := w.LandCoverage * sphereArea
	continental := make([]bool, w.NumPlates)
	if target > 0 {
		var taken float64
		for _, k := range order {
			continental[k] = true
			if taken += plateArea[k]; taken >= target/continentFill {
				break
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Step 3.
	inland := distances(mesh, w.Radius, func(i int, j int32) bool { return continental[w.Plate[i]] != continental[w.Plate[j]] })
	if err := ctx.Err(); err != nil {
		return err
	}
	score := make([]float64, n)
	freq := noise.Frequency(w.Radius, coastNoiseFeatureSize*plateSize)
	amplitude := coastNoiseAmplitude * plateSize
	err := model.ParallelRange(ctx, n, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			if !continental[w.Plate[i]] {
				continue
			}
			d := inland[i]
			if math.IsInf(d, 1) {
				d = 0 // every plate is continental: there is no edge
			}
			p := mesh.Points[i]
			score[i] = d + amplitude*coast.At(p.X*freq, p.Y*freq, p.Z*freq)
		}
	})
	if err != nil {
		return err
	}

	// Step 4.
	candidates := make([]int32, 0, n)
	for i := range n {
		if continental[w.Plate[i]] {
			candidates = append(candidates, int32(i))
		}
	}
	slices.SortFunc(candidates, func(a, b int32) int {
		return cmp.Or(cmp.Compare(score[b], score[a]), cmp.Compare(a, b))
	})
	clear(w.CrustType) // all oceanic
	var land float64
	for _, i := range candidates {
		// Stop when adding the sample would leave the area further
		// from the target than it is now.
		if land+areas[i]/2 > target {
			break
		}
		land += areas[i]
		w.CrustType[i] = model.Continental
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	// Initial values.
	fromCoast := distances(mesh, w.Radius, func(i int, j int32) bool { return w.CrustType[i] != w.CrustType[j] })
	oceanicAge := InitialOceanicAge(w.Params)
	return model.ParallelRange(ctx, n, func(lo, hi int) {
		for i := lo; i < hi; i++ {
			s := smoothstep(fromCoast[i] / marginWidth)
			if w.CrustType[i] == model.Continental {
				w.Thickness[i] = continentalThickness
				w.Elevation[i] = w.MaxContinentalElevation * (coastElevation + (interiorElevation-coastElevation)*s)
				w.OceanicAge[i] = 0
			} else {
				w.Thickness[i] = oceanicThickness
				w.Elevation[i] = w.RidgeElevation + (w.AbyssalElevation-w.RidgeElevation)*s
				w.OceanicAge[i] = oceanicAge
			}
			w.RidgeDirection[i] = sphere.Vec{}
			w.OrogenyAge[i] = 0
			w.OrogenyType[i] = model.OrogenyNone
			w.FoldDirection[i] = sphere.Vec{}
		}
	})
}

// InitialOceanicAge returns the age in My given to all oceanic crust at
// the start: the time the paper's oceanic dampening (§4.5),
//
//	dz/dt = −(1 − z/z_t) · ε_o,
//
// takes to lower crust from the ridge elevation z_r to the abyssal
// elevation z_a, which is (z_t/ε_o) · ln((z_a − z_t)/(z_r − z_t)). With
// the paper's constants it is 202.7 My.
func InitialOceanicAge(p model.Params) float64 {
	return p.TrenchElevation / oceanicDampening *
		math.Log((p.AbyssalElevation-p.TrenchElevation)/(p.RidgeElevation-p.TrenchElevation))
}

// distances returns each sample's distance in km to the nearest edge
// sample: a sample i with a neighbour j for which differ(i, j) is true.
// An edge sample is at distance 0. With no edge samples every distance
// is +Inf.
func distances(mesh *sphere.Mesh, radius float64, differ func(i int, j int32) bool) []float64 {
	var edge []int32
	for i := range mesh.Points {
		for _, j := range mesh.Neighbours(i) {
			if differ(i, j) {
				edge = append(edge, int32(i))
				break
			}
		}
	}
	dist, _ := mesh.Distances(edge)
	for i := range dist {
		dist[i] *= radius
	}
	return dist
}

// smoothstep returns x²(3 − 2x) for x in [0, 1], 0 below, and 1 above,
// including at +Inf.
func smoothstep(x float64) float64 {
	x = max(0, min(1, x))
	return x * x * (3 - 2*x)
}
