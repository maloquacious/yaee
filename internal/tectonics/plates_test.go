// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// earth returns the default parameters of the stages in this package.
func earth() model.Params {
	return model.Params{
		Radius:               6370,
		Duration:             250,
		TimeStep:             2,
		Plates:               40,
		PlateWarpAmplitude:   1500,
		PlateWarpFeatureSize: 2500,

		LandCoverage:            0.3,
		RidgeElevation:          -1,
		AbyssalElevation:        -6,
		TrenchElevation:         -10,
		MaxContinentalElevation: 10,

		MaxPlateSpeed: 100,

		OceanicDampening: 0.04,

		SubductionDistance: 1800,
		SubductionUplift:   0.6,
		ContinentalErosion: 0.03,
		SedimentAccretion:  0.3,
	}
}

// newWorld returns an empty world of n samples.
func newWorld(t testing.TB, n int, params model.Params) *model.World {
	t.Helper()
	mesh, err := sphere.Triangulate(sphere.FibonacciPoints(n))
	if err != nil {
		t.Fatalf("Triangulate(%d points): %v", n, err)
	}
	return model.NewWorld(mesh, params)
}

// run applies the stages to w with the given seed.
func run(t testing.TB, w *model.World, seed uint64, stages ...model.Stage) {
	t.Helper()
	if err := model.Run(t.Context(), w, seed, stages); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

// checkPartition fails the test unless every sample has a plate from 0
// to NumPlates-1, no plate is empty, and every plate is one connected
// region. It returns the number of samples in each plate.
func checkPartition(t testing.TB, w *model.World) []int {
	t.Helper()
	if w.NumPlates != w.Plates {
		t.Fatalf("NumPlates = %d, want %d", w.NumPlates, w.Plates)
	}
	n := len(w.Mesh.Points)
	if len(w.Plate) != n {
		t.Fatalf("len(Plate) = %d, want %d", len(w.Plate), n)
	}
	size := make([]int, w.NumPlates)
	first := make([]int32, w.NumPlates)
	for i, k := range w.Plate {
		if k < 0 || int(k) >= w.NumPlates {
			t.Fatalf("sample %d has plate %d, outside 0 to %d", i, k, w.NumPlates-1)
		}
		if size[k] == 0 {
			first[k] = int32(i)
		}
		size[k]++
	}
	seen := make([]bool, n)
	for k, s := range size {
		if s == 0 {
			t.Fatalf("plate %d is empty", k)
		}
		// Count what the plate's first sample reaches within the plate.
		queue := []int32{first[k]}
		seen[first[k]] = true
		for head := 0; head < len(queue); head++ {
			for _, j := range w.Mesh.Neighbours(int(queue[head])) {
				if !seen[j] && w.Plate[j] == int32(k) {
					seen[j] = true
					queue = append(queue, j)
				}
			}
		}
		if len(queue) != s {
			t.Fatalf("plate %d has %d samples but only %d are connected to sample %d", k, s, len(queue), first[k])
		}
	}
	return size
}

func TestPlatesPartition(t *testing.T) {
	for seed := range uint64(8) {
		w := newWorld(t, 6000, earth())
		run(t, w, seed, Plates{})
		size := checkPartition(t, w)
		// No slivers: the smallest plate is a real region.
		if smallest := slices.Min(size); smallest < 6000/40/20 {
			t.Errorf("seed %d: smallest plate has %d samples of 6000", seed, smallest)
		}
	}
}

func TestPlatesExtremes(t *testing.T) {
	cases := []struct {
		name    string
		samples int
		set     func(*model.Params)
	}{
		{"one plate", 500, func(p *model.Params) { p.Plates = 1 }},
		{"two plates", 500, func(p *model.Params) { p.Plates = 2 }},
		{"a plate per sample", 4, func(p *model.Params) { p.Plates = 4 }},
		{"a plate per sample, more samples", 60, func(p *model.Params) { p.Plates = 60 }},
		{"nearly a plate per sample", 300, func(p *model.Params) { p.Plates = 250 }},
		{"many plates", 20000, func(p *model.Params) { p.Plates = 1000 }},
		{"no warp", 3000, func(p *model.Params) { p.PlateWarpAmplitude = 0 }},
		{"warp far larger than a plate", 3000, func(p *model.Params) { p.PlateWarpAmplitude = 20000 }},
		{"warp features far smaller than a plate", 3000, func(p *model.Params) { p.PlateWarpFeatureSize = 300 }},
		{"warp features smaller than the sample spacing", 3000, func(p *model.Params) { p.PlateWarpFeatureSize = 10 }},
		{"small sphere", 3000, func(p *model.Params) { p.Radius = 100; p.PlateWarpAmplitude = 30; p.PlateWarpFeatureSize = 50 }},
	}
	for _, c := range cases {
		for seed := range uint64(3) {
			params := earth()
			c.set(&params)
			w := newWorld(t, c.samples, params)
			run(t, w, seed, Plates{})
			if t.Failed() {
				return
			}
			func() {
				defer func() {
					if t.Failed() {
						t.Logf("case %q, seed %d", c.name, seed)
					}
				}()
				checkPartition(t, w)
			}()
		}
	}
}

// With the warp off, each sample belongs to the plate with the nearest
// centroid.
func TestPlatesWithoutWarpAreVoronoiCells(t *testing.T) {
	for seed := range uint64(10) {
		params := earth()
		params.PlateWarpAmplitude = 0
		w := newWorld(t, 5000, params)
		run(t, w, seed, Plates{})
		checkPartition(t, w)

		// The stage's first draws are the centroids.
		centroids := placeCentroids(model.Stream(seed, Plates{}.Name()), params.Plates)
		for i, p := range w.Mesh.Points {
			nearest, angle := 0, math.Inf(1)
			for k, c := range centroids {
				if a := sphere.Angle(p, c); a < angle {
					nearest, angle = k, a
				}
			}
			if got := int(w.Plate[i]); got != nearest {
				// A tie between centroids is not an error.
				if a := sphere.Angle(p, centroids[got]); a-angle > 1e-12 {
					t.Fatalf("seed %d: sample %d is on plate %d, %g rad from its centroid, but plate %d's centroid is %g rad away",
						seed, i, got, a, nearest, angle)
				}
			}
		}
	}
}

// The warp must move boundaries, and by about what Config documents.
func TestWarpDisplacement(t *testing.T) {
	const radius, amplitude = 6370.0, 1500.0
	wp := newWarp(model.Stream(3, "warp"), radius, amplitude, 2500)
	var sum2, largest float64
	points := sphere.FibonacciPoints(40000)
	for _, p := range points {
		q := wp.apply(p)
		d := sphere.Angle(p, q) * radius
		sum2 += d * d
		largest = max(largest, d)
	}
	rms := math.Sqrt(sum2 / float64(len(points)))
	if got := rms / amplitude; math.Abs(got-0.41) > 0.04 {
		t.Errorf("RMS displacement = %.3f of the amplitude, want 0.41 ± 0.04", got)
	}
	if got := largest / amplitude; got > 2.91 {
		t.Errorf("largest displacement = %.3f of the amplitude, want at most 2.91", got)
	}

	off := newWarp(model.Stream(3, "warp"), radius, 0, 2500)
	for _, p := range points[:100] {
		if q := off.apply(p); q != p {
			t.Fatalf("a warp of amplitude 0 moved %v to %v", p, q)
		}
	}
}

// The warp changes which plate samples near a boundary belong to, and
// leaves the plates recognisable.
func TestWarpMovesBoundaries(t *testing.T) {
	params := earth()
	warped := newWorld(t, 8000, params)
	run(t, warped, 5, Plates{})
	params.PlateWarpAmplitude = 0
	plain := newWorld(t, 8000, params)
	run(t, plain, 5, Plates{})

	var differ int
	for i := range warped.Plate {
		if warped.Plate[i] != plain.Plate[i] {
			differ++
		}
	}
	if frac := float64(differ) / 8000; frac < 0.05 || frac > 0.6 {
		t.Errorf("the warp changed the plate of %.0f%% of samples, want 5%% to 60%%", 100*frac)
	}
}

func TestCentroids(t *testing.T) {
	for _, n := range []int{1, 2, 7, 40, 300, 1000} {
		for seed := range uint64(5) {
			centroids := placeCentroids(model.Stream(seed, "centroids"), n)
			if len(centroids) != n {
				t.Fatalf("%d centroids, want %d", len(centroids), n)
			}
			want := centroidSeparation * math.Sqrt(4*math.Pi/float64(n))
			for i, a := range centroids {
				if l := a.Len(); math.Abs(l-1) > 1e-12 {
					t.Fatalf("centroid %d has length %v", i, l)
				}
				for j, b := range centroids[:i] {
					// Placement at this density never has to relax
					// the separation.
					if d := sphere.Angle(a, b); d < want-1e-9 {
						t.Fatalf("n = %d, seed %d: centroids %d and %d are %g rad apart, want at least %g", n, seed, j, i, d, want)
					}
				}
			}
		}
	}
}

// Centroids cover the sphere evenly: over many draws the mean position
// is near the centre.
func TestCentroidsAreUniform(t *testing.T) {
	var sum sphere.Vec
	const draws = 400
	for seed := range uint64(draws) {
		for _, c := range placeCentroids(model.Stream(seed, "centroids"), 10) {
			sum = sum.Add(c)
		}
	}
	if l := sum.Len() / (draws * 10); l > 0.05 {
		t.Errorf("mean centroid is %v from the centre, want under 0.05", l)
	}
}

func TestPlatesRepeatable(t *testing.T) {
	a, b, c := newWorld(t, 4000, earth()), newWorld(t, 4000, earth()), newWorld(t, 4000, earth())
	run(t, a, 11, Plates{})
	run(t, b, 11, Plates{})
	run(t, c, 12, Plates{})
	if !slices.Equal(a.Plate, b.Plate) {
		t.Error("the same seed gave different plates")
	}
	if slices.Equal(a.Plate, c.Plate) {
		t.Error("different seeds gave the same plates")
	}
}

// The same seed gives nearly the same plates at another resolution: a
// sample of the coarse world has the plate of the fine world's sample
// nearest to it, except near boundaries, whose finest bends are smaller
// than the coarse spacing.
func TestPlatesIndependentOfResolution(t *testing.T) {
	coarse, fine := newWorld(t, 3000, earth()), newWorld(t, 12000, earth())
	run(t, coarse, 21, Plates{})
	run(t, fine, 21, Plates{})
	locator := sphere.NewLocator(fine.Mesh)
	var differ int
	for i, p := range coarse.Mesh.Points {
		if coarse.Plate[i] != fine.Plate[locator.Nearest(p)] {
			differ++
		}
	}
	if frac := float64(differ) / 3000; frac > 0.08 {
		t.Errorf("%.1f%% of coarse samples differ from the fine world, want at most 8%%", 100*frac)
	}
}

// A piece cut off from its plate is given to the plate around it.
func TestStrayPiecesAreReassigned(t *testing.T) {
	w := newWorld(t, 2000, earth())
	mesh := w.Mesh
	n := len(mesh.Points)

	// Plate 0 is the northern hemisphere and plate 1 the southern,
	// except for two pieces of plate 0 inside the south: a patch around
	// a point, and a single sample.
	patch := sphere.Vec{X: 0.6, Z: -0.8}
	plate := make([]int32, n)
	var strays []int
	for i, p := range mesh.Points {
		switch {
		case p.Z >= 0:
			plate[i] = 0
		case sphere.Angle(p, patch) < 0.2:
			plate[i] = 0
			strays = append(strays, i)
		default:
			plate[i] = 1
		}
	}
	single := n - 40
	plate[single] = 0
	strays = append(strays, single)
	if len(strays) < 10 {
		t.Fatalf("only %d stray samples; the test needs a patch", len(strays))
	}
	before := slices.Clone(plate)

	connect(mesh, plate, []int32{0, int32(n - 1)}) // the seeds are the samples nearest the poles

	for _, i := range strays {
		if plate[i] != 1 {
			t.Errorf("stray sample %d is on plate %d, want 1", i, plate[i])
		}
		before[i] = 1
	}
	if !slices.Equal(plate, before) {
		t.Error("connect changed samples that were not strays")
	}

	// A stray piece between two plates goes to one of them, and what is
	// left is connected.
	w.Plates = 2
	for i, p := range mesh.Points {
		w.Plate[i] = 0
		if p.Z < 0 {
			w.Plate[i] = 1
		}
		if math.Abs(p.Z) < 0.1 && p.X > 0.9 {
			w.Plate[i] = 0
		}
		if p.Z > 0.1 && p.Z < 0.3 {
			w.Plate[i] = 1 // a band of plate 1 that cuts plate 0 in two
		}
	}
	connect(mesh, w.Plate, []int32{0, int32(n - 1)})
	w.NumPlates = 2
	checkPartition(t, w)
}

// A plate that no sample chose still gets a sample.
func TestEmptyPlateTakesASample(t *testing.T) {
	w := newWorld(t, 500, earth())
	w.Plates = 3
	north := sphere.Vec{Z: 1}
	// Plates 0 and 2 share a centroid, and ties go to the lower number,
	// so no sample chooses plate 2.
	if err := partition(t.Context(), w, []sphere.Vec{north, {Z: -1}, north}, warp{}); err != nil {
		t.Fatal(err)
	}
	size := checkPartition(t, w)
	if size[2] != 1 {
		t.Errorf("plate 2 has %d samples, want the 1 it took", size[2])
	}
	// It took the sample nearest its centroid that was not a seed:
	// sample 0 is plate 0's seed, so sample 1.
	if w.Plate[0] != 0 || w.Plate[1] != 2 {
		t.Errorf("samples 0 and 1 are on plates %d and %d, want 0 and 2", w.Plate[0], w.Plate[1])
	}
}

func TestPlatesCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w := newWorld(t, 20000, earth())
	if err := (Plates{}).Run(ctx, w, model.Stream(1, "plates")); err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
