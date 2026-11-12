// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package rocks

import (
	"context"
	"math"
	"reflect"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// testWorld returns a world of n samples on an Earth-sized sphere with
// the given rock parameters, in km. Crust south of latitude −17° is
// oceanic. The continental crust north of it is in three sectors of
// longitude, one for each orogeny type, so every rock type has a region
// of its own with edges against the others. North of latitude 37° the
// Andean orogeny is older than RockOrogenyMaxAge, and north of 53° the
// Himalayan crust is being raised faster than RockArcUplift.
func testWorld(t testing.TB, n int, amplitude, featureSize, reach float64) *model.World {
	t.Helper()
	mesh, err := sphere.Triangulate(sphere.FibonacciPoints(n))
	if err != nil {
		t.Fatal(err)
	}
	w := model.NewWorld(mesh, model.Params{
		Radius:               6370,
		RockNoiseAmplitude:   amplitude,
		RockNoiseFeatureSize: featureSize,
		RockReach:            reach,
		RockOrogenyMaxAge:    150,
		RockArcUplift:        0.01,
	})
	for i, p := range mesh.Points {
		if p.Z < -0.3 {
			w.CrustType[i] = model.Oceanic
			w.OceanicAge[i] = 50
			continue
		}
		w.CrustType[i] = model.Continental
		lon := math.Atan2(p.Y, p.X) // −π to π
		switch {
		case lon < -math.Pi/3:
			w.OrogenyType[i] = model.OrogenyNone
		case lon < math.Pi/3:
			w.OrogenyType[i], w.OrogenyAge[i] = model.OrogenyAndean, 10
			if p.Z > 0.6 {
				w.OrogenyAge[i] = 200
			}
		default:
			w.OrogenyType[i], w.OrogenyAge[i] = model.OrogenyHimalayan, 20
			if p.Z > 0.8 {
				w.Uplift[i] = 0.05
			} else if p.Z > 0.6 {
				w.Uplift[i] = 0.01 // not above RockArcUplift
			}
		}
	}
	return w
}

func run(t testing.TB, w *model.World, seed uint64) {
	t.Helper()
	if err := model.Run(context.Background(), w, seed, []model.Stage{RockType{}}); err != nil {
		t.Fatal(err)
	}
}

// wantBase is the table of the stage's documentation, for a world of
// testWorld.
func wantBase(w *model.World, i int) uint8 {
	switch {
	case w.CrustType[i] == model.Oceanic:
		return model.RockIgneous
	case w.Uplift[i] > 0.01:
		return model.RockIgneous
	case w.OrogenyAge[i] > 150:
		return model.RockSedimentary
	}
	return map[uint8]uint8{
		model.OrogenyNone:      model.RockSedimentary,
		model.OrogenyAndean:    model.RockIgneous,
		model.OrogenyHimalayan: model.RockMetamorphic,
	}[w.OrogenyType[i]]
}

func TestNoNoiseFollowsTable(t *testing.T) {
	w := testWorld(t, 3000, 0, 1000, 500)
	run(t, w, 1)
	seen := map[[2]uint8]bool{}
	for i, r := range w.RockType {
		if want := wantBase(w, i); r != want {
			t.Fatalf("sample %d (crust %d, orogeny %d, orogeny age %v, uplift %v) has rock type %d, want %d",
				i, w.CrustType[i], w.OrogenyType[i], w.OrogenyAge[i], w.Uplift[i], r, want)
		}
		seen[[2]uint8{w.OrogenyType[i], r}] = true
	}
	// Every rule applied: old Andean crust is sedimentary, and raised
	// Himalayan crust igneous.
	for _, k := range [][2]uint8{
		{model.OrogenyAndean, model.RockIgneous}, {model.OrogenyAndean, model.RockSedimentary},
		{model.OrogenyHimalayan, model.RockMetamorphic}, {model.OrogenyHimalayan, model.RockIgneous},
		{model.OrogenyNone, model.RockSedimentary},
	} {
		if !seen[k] {
			t.Errorf("no sample of orogeny type %d has rock type %d", k[0], k[1])
		}
	}
}

func TestEverySampleClassified(t *testing.T) {
	for _, amplitude := range []float64{0, 300, 3000} {
		w := testWorld(t, 3000, amplitude, 1000, 500)
		run(t, w, 2)
		for i, r := range w.RockType {
			if r < model.RockIgneous || r > model.RockSedimentary {
				t.Fatalf("amplitude %v: sample %d has rock type %d", amplitude, i, r)
			}
		}
	}
}

func TestOceanIsIgneous(t *testing.T) {
	w := testWorld(t, 3000, 3000, 1000, 500)
	run(t, w, 3)
	for i, r := range w.RockType {
		if w.CrustType[i] == model.Oceanic && r != model.RockIgneous {
			t.Fatalf("oceanic sample %d has rock type %d", i, r)
		}
	}
}

func TestChangesNothingElse(t *testing.T) {
	w := testWorld(t, 3000, 300, 1000, 500)
	clone := func(w *model.World) model.World {
		c := *w
		c.CrustType = append([]uint8(nil), w.CrustType...)
		c.OrogenyType = append([]uint8(nil), w.OrogenyType...)
		c.OrogenyAge = append([]float64(nil), w.OrogenyAge...)
		c.OceanicAge = append([]float64(nil), w.OceanicAge...)
		c.Elevation = append([]float64(nil), w.Elevation...)
		c.RockType = nil
		return c
	}
	before := clone(w)
	run(t, w, 4)
	after := clone(w)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("the stage changed something other than RockType")
	}
}

func TestSeeds(t *testing.T) {
	// 2A < D, so a sample farther than D from every other rock type
	// keeps its base rock type whatever the seed.
	const n, amplitude, reach = 12000, 400, 1000
	a := testWorld(t, n, amplitude, 1500, reach)
	run(t, a, 5)
	again := testWorld(t, n, amplitude, 1500, reach)
	run(t, again, 5)
	if !reflect.DeepEqual(a.RockType, again.RockType) {
		t.Fatal("the same seed gave different rock types")
	}

	b := testWorld(t, n, amplitude, 1500, reach)
	run(t, b, 6)

	// The distance from each sample to the nearest continental sample
	// of another base rock type.
	base := testWorld(t, n, 0, 1500, reach)
	run(t, base, 0)
	far := make([]bool, n)
	for i := range far {
		far[i] = base.CrustType[i] == model.Continental
	}
	for _, c := range classes {
		d := distances(base, c)
		for i := range far {
			if base.RockType[i] != c && d[i] < reach {
				far[i] = false
			}
		}
	}

	farCount, changed, edgeChanged := 0, 0, 0
	for i := range far {
		if a.RockType[i] != b.RockType[i] {
			changed++
		}
		if far[i] {
			farCount++
			if a.RockType[i] != base.RockType[i] || b.RockType[i] != base.RockType[i] {
				t.Fatalf("sample %d, farther than %v km from every other rock type, changed from %d to %d (seed 5) and %d (seed 6)",
					i, float64(reach), base.RockType[i], a.RockType[i], b.RockType[i])
			}
		} else if a.RockType[i] != base.RockType[i] {
			edgeChanged++
		}
	}
	if farCount < n/10 {
		t.Fatalf("only %d samples are far from every edge; the test shows nothing", farCount)
	}
	if changed == 0 {
		t.Error("seeds 5 and 6 gave the same rock types")
	}
	if edgeChanged == 0 {
		t.Error("the noise moved no edge")
	}
}

func TestPatches(t *testing.T) {
	// With 2A well above D, rock types appear inside regions of
	// another, farther than D from it.
	const n, reach = 12000, 300
	w := testWorld(t, n, 1500, 1500, reach)
	run(t, w, 7)
	base := testWorld(t, n, 0, 1500, reach)
	run(t, base, 0)
	d := map[uint8][]float64{}
	for _, c := range classes {
		d[c] = distances(base, c)
	}
	patches := 0
	for i, r := range w.RockType {
		if w.CrustType[i] == model.Continental && d[r][i] >= reach {
			patches++
		}
	}
	if patches == 0 {
		t.Error("no sample took a rock type that is farther than RockReach away")
	}
}

func TestCancel(t *testing.T) {
	w := testWorld(t, 3000, 300, 1000, 500)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := (RockType{}).Run(ctx, w, model.Stream(1, "rock-type")); err != context.Canceled {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}
