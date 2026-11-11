// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package stats

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/maloquacious/yaee"
	"github.com/maloquacious/yaee/internal/sphere"
)

const testRadius = 1000.0

// constructed returns a world of n samples whose values are set by
// rule, so that every statistic is known:
//
//   - Continental crust is two caps: the north one where z > 0.6, and
//     the south one where z < −0.8.
//   - Plate 0 holds x ≥ 0 and plate 1 holds x < 0, except that the
//     south cap is plate 2. The north cap is therefore two terranes of
//     one continent.
//   - Elevation is 2 km in the north cap where z > 0.9, and 0.25 km in
//     the rest of it; −0.3 km in the south cap; −4.2 km on oceanic
//     crust, except 1.1 km on the oceanic crust where y > 0.95.
//   - Oceanic age is 100·(1 − |z|) My on oceanic crust.
//   - Orogeny is Himalayan where z > 0.9 and Andean where y > 0.95.
func constructed(t *testing.T, n int) (*yaee.World, *sphere.Mesh) {
	t.Helper()
	mesh, err := sphere.Triangulate(sphere.FibonacciPoints(n))
	if err != nil {
		t.Fatal(err)
	}
	w := &yaee.World{
		Config:    yaee.DefaultConfig(),
		Version:   "1.2.3-test",
		Positions: make([][3]float64, n),
		Triangles: make([][3]int32, mesh.NumTriangles()),
	}
	w.Config.Radius = testRadius
	for t := range w.Triangles {
		w.Triangles[t] = [3]int32(mesh.Tri[3*t : 3*t+3])
	}
	plate, crust, orogeny := make([]int32, n), make([]uint8, n), make([]uint8, n)
	z, age := make([]float64, n), make([]float64, n)
	for i, p := range mesh.Points {
		w.Positions[i] = [3]float64{p.X, p.Y, p.Z}
		if p.X < 0 {
			plate[i] = 1
		}
		switch {
		case p.Z > 0.9:
			crust[i], z[i], orogeny[i] = yaee.CrustContinental, 2, yaee.OrogenyHimalayan
		case p.Z > 0.6:
			crust[i], z[i] = yaee.CrustContinental, 0.25
		case p.Z < -0.8:
			crust[i], z[i], plate[i] = yaee.CrustContinental, -0.3, 2
		case p.Y > 0.95:
			z[i], orogeny[i], age[i] = 1.1, yaee.OrogenyAndean, 100*(1-math.Abs(p.Z))
		default:
			z[i], age[i] = -4.2, 100*(1-math.Abs(p.Z))
		}
	}
	w.Attributes = []yaee.Attribute{
		{Name: yaee.AttrPlate, Type: yaee.Int32, Components: 1, Int32s: plate},
		{Name: yaee.AttrCrustType, Type: yaee.Uint8, Components: 1, Uint8s: crust},
		{Name: yaee.AttrElevation, Unit: "km", Type: yaee.Float64, Components: 1, Float64s: z},
		{Name: yaee.AttrOceanicAge, Unit: "My", Type: yaee.Float64, Components: 1, Float64s: age},
		{Name: yaee.AttrOrogenyType, Type: yaee.Uint8, Components: 1, Uint8s: orogeny},
	}
	return w, mesh
}

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if !(math.Abs(got-want) <= tol) {
		t.Errorf("%s = %.12g, want %.12g within %g", name, got, want, tol)
	}
}

// Every number is checked twice: against a sum made here sample by
// sample, to rounding, and against the area that geometry gives the
// construction, to the error of 4000 samples.
func TestMeasureConstructedWorld(t *testing.T) {
	const n = 4000
	w, mesh := constructed(t, n)
	s, err := Measure(w)
	if err != nil {
		t.Fatal(err)
	}

	if s.LibraryVersion != "1.2.3-test" || s.Samples != n || s.Triangles != 2*n-4 {
		t.Errorf("version %q, %d samples, %d triangles", s.LibraryVersion, s.Samples, s.Triangles)
	}
	sphereArea := 4 * math.Pi * testRadius * testRadius
	near(t, "radius_km", s.Radius, testRadius, 0)
	near(t, "area_km2", s.Area, sphereArea, 1e-6)
	near(t, "spacing_km", s.Spacing, sphere.SampleSpacing(testRadius, n), 0)
	if got := sphere.SampleCount(testRadius, s.Spacing); got != n {
		t.Errorf("spacing_km %g gives %d samples, want %d", s.Spacing, got, n)
	}

	// Sums made here. share[i] is sample i's fraction of the sphere.
	share := mesh.PointAreas()
	for i := range share {
		share[i] /= 4 * math.Pi
	}
	plate := w.Attribute(yaee.AttrPlate).Int32s
	z := w.Attribute(yaee.AttrElevation).Float64s
	age := w.Attribute(yaee.AttrOceanicAge).Float64s
	sum := func(keep func(p sphere.Vec, i int) bool) (area float64) {
		for i, p := range mesh.Points {
			if keep(p, i) {
				area += share[i]
			}
		}
		return area
	}
	north := func(p sphere.Vec, _ int) bool { return p.Z > 0.6 }
	south := func(p sphere.Vec, _ int) bool { return p.Z < -0.8 }
	peak := func(p sphere.Vec, _ int) bool { return p.Z > 0.9 }
	island := func(p sphere.Vec, _ int) bool { return p.Z <= 0.6 && p.Z >= -0.8 && p.Y > 0.95 }
	const exact = 1e-12
	// A cap with z above h is (1 − h)/2 of the sphere; the samples near
	// its edge make the sum uncertain by about the width of one sample.
	const coarse = 0.01

	near(t, "continental_fraction", s.ContinentalFraction, sum(north)+sum(south), exact)
	near(t, "continental_fraction", s.ContinentalFraction, 0.2+0.1, coarse)
	near(t, "land_fraction", s.LandFraction, sum(north)+sum(island), exact)
	near(t, "land_fraction", s.LandFraction, 0.2+0.025, coarse)
	near(t, "oceanic_land_fraction", s.OceanicLandFraction, sum(island), exact)
	near(t, "oceanic_land_fraction", s.OceanicLandFraction, 0.025, coarse)
	near(t, "continental_sea_fraction", s.ContinentalSeaFraction, sum(south), exact)
	near(t, "continental_sea_fraction", s.ContinentalSeaFraction, 0.1, coarse)
	near(t, "orogeny.himalayan_fraction", s.Orogeny.Himalayan, sum(peak), exact)
	near(t, "orogeny.himalayan_fraction", s.Orogeny.Himalayan, 0.05, coarse)
	near(t, "orogeny.andean_fraction", s.Orogeny.Andean, sum(island), exact)

	// Elevation: 0.05 at 2, 0.15 at 0.25, 0.1 at −0.3, 0.025 at 1.1,
	// and 0.675 at −4.2.
	mean := func(keep func(p sphere.Vec, i int) bool) float64 {
		var area, total float64
		for i, p := range mesh.Points {
			if keep(p, i) {
				area += share[i]
				total += share[i] * z[i]
			}
		}
		return total / area
	}
	all := func(sphere.Vec, int) bool { return true }
	cont := func(p sphere.Vec, i int) bool { return north(p, i) || south(p, i) }
	ocean := func(p sphere.Vec, i int) bool { return !cont(p, i) }
	e := s.Elevation
	if e.Min != -4.2 || e.Max != 2 || e.P10 != -4.2 || e.P50 != -4.2 || e.P90 != 0.25 {
		t.Errorf("elevation_km = %+v, want min −4.2, max 2, p10 −4.2, p50 −4.2, p90 0.25", e)
	}
	near(t, "elevation_km.mean", e.Mean, mean(all), exact)
	near(t, "elevation_km.mean", e.Mean, 0.05*2+0.15*0.25-0.1*0.3+0.025*1.1-0.675*4.2, 0.05)
	// Of the continental area, a third is at −0.3, a half at 0.25, and
	// a sixth at 2.
	c := s.ContinentalElevation
	if c.Min != -0.3 || c.Max != 2 || c.P10 != -0.3 || c.P50 != 0.25 || c.P90 != 2 {
		t.Errorf("continental_elevation_km = %+v, want min −0.3, max 2, p10 −0.3, p50 0.25, p90 2", c)
	}
	near(t, "continental_elevation_km.mean", c.Mean, mean(cont), exact)
	o := s.OceanicElevation
	if o.Min != -4.2 || o.Max != 1.1 || o.P10 != -4.2 || o.P50 != -4.2 || o.P90 != -4.2 {
		t.Errorf("oceanic_elevation_km = %+v, want min −4.2, max 1.1, and −4.2 at every quantile", o)
	}
	near(t, "oceanic_elevation_km.mean", o.Mean, mean(ocean), exact)

	// The histogram: bin 0 starts at −4.5 and holds −4.2; −0.3 is in
	// bin 8, 0.25 in bin 9, 1.1 in bin 11, and 2 in bin 13, the last.
	h := s.Histogram
	if h.BinWidth != 0.5 || h.First != -4.5 || len(h.Bins) != 14 {
		t.Fatalf("histogram: width %g, first %g, %d bins; want 0.5, −4.5, 14", h.BinWidth, h.First, len(h.Bins))
	}
	wantBins := map[int]float64{0: 1 - sum(cont) - sum(island), 8: sum(south), 9: sum(north) - sum(peak), 11: sum(island), 13: sum(peak)}
	var binSum float64
	for i, got := range h.Bins {
		near(t, "histogram bin", got, wantBins[i], exact)
		binSum += got
	}
	near(t, "the sum of the histogram", binSum, 1, exact)

	// Plates: 0 and 1 are the two halves less the south cap; 2 is the
	// south cap, a tenth of the sphere.
	plateShare := make([]float64, 3)
	for i := range mesh.Points {
		plateShare[plate[i]] += share[i]
	}
	if s.Plates.Count != 3 {
		t.Errorf("plates.count = %d, want 3", s.Plates.Count)
	}
	near(t, "plates.largest_km2", s.Plates.Largest, slices.Max(plateShare)*sphereArea, 1e-6)
	near(t, "plates.largest_km2", s.Plates.Largest, 0.45*sphereArea, coarse*sphereArea)
	near(t, "plates.smallest_km2", s.Plates.Smallest, plateShare[2]*sphereArea, 1e-6)
	near(t, "plates.smallest_km2", s.Plates.Smallest, 0.1*sphereArea, coarse*sphereArea)

	// The north cap is two terranes of one continent; the south cap is
	// one of each, and the smaller continent. The larger half of the
	// north cap is the largest terrane, unless the south cap is.
	if s.Terranes.Count != 3 || s.Continents.Count != 2 || s.Landmasses.Count != 2 {
		t.Errorf("%d terranes, %d continents, %d landmasses; want 3, 2, 2", s.Terranes.Count, s.Continents.Count, s.Landmasses.Count)
	}
	east := sum(func(p sphere.Vec, i int) bool { return north(p, i) && p.X >= 0 })
	near(t, "terranes.largest_km2", s.Terranes.Largest, max(east, sum(north)-east, sum(south))*sphereArea, 1e-6)
	near(t, "terranes.largest_km2", s.Terranes.Largest, 0.1*sphereArea, coarse*sphereArea)
	near(t, "continents.largest_km2", s.Continents.Largest, sum(north)*sphereArea, 1e-6)
	// Land is the north cap and the island where y > 0.95, which lies
	// near the equator and does not touch the cap.
	near(t, "landmasses.largest_km2", s.Landmasses.Largest, sum(north)*sphereArea, 1e-6)

	// Oceanic age.
	var ageMax, ageSum, oceanArea float64
	for i, p := range mesh.Points {
		if ocean(p, i) {
			ageMax = max(ageMax, age[i])
			ageSum += share[i] * age[i]
			oceanArea += share[i]
		}
	}
	near(t, "oceanic_age_my.max", s.OceanicAge.Max, ageMax, 0)
	near(t, "oceanic_age_my.max", s.OceanicAge.Max, 100, 0.1)
	near(t, "oceanic_age_my.mean", s.OceanicAge.Mean, ageSum/oceanArea, exact*100)
	// The mean of 1 − |z| over −0.8 ≤ z ≤ 0.6 is (0.48 + 0.42)/1.4.
	near(t, "oceanic_age_my.mean", s.OceanicAge.Mean, 100*0.9/1.4, 1)
}

// The quantiles follow their definition on values that are all
// different.
func TestQuantiles(t *testing.T) {
	const n = 1500
	w, mesh := constructed(t, n)
	z := w.Attribute(yaee.AttrElevation).Float64s
	for i, p := range mesh.Points {
		z[i] = 3*p.X - 2*p.Y*p.Z
	}
	s, err := Measure(w)
	if err != nil {
		t.Fatal(err)
	}
	share := mesh.PointAreas()
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(a, b int) bool { return z[order[a]] < z[order[b]] })
	var total float64
	for _, a := range share {
		total += a
	}
	quantile := func(q float64) float64 {
		var below float64
		for _, i := range order {
			below += share[i]
			if below >= q*total {
				return z[i]
			}
		}
		return z[order[n-1]]
	}
	e := s.Elevation
	if e.P10 != quantile(0.1) || e.P50 != quantile(0.5) || e.P90 != quantile(0.9) {
		t.Errorf("quantiles %g, %g, %g; want %g, %g, %g", e.P10, e.P50, e.P90, quantile(0.1), quantile(0.5), quantile(0.9))
	}
	if e.Min != z[order[0]] || e.Max != z[order[n-1]] {
		t.Errorf("min %g, max %g; want %g, %g", e.Min, e.Max, z[order[0]], z[order[n-1]])
	}
	// z is odd under the reflection that Fibonacci samples nearly have,
	// so the median is near 0 and p10 is near −p90.
	near(t, "p50", e.P50, 0, 0.1)
	near(t, "p10 + p90", e.P10+e.P90, 0, 0.1)
}

// A world with no continental crust, and one with nothing else.
func TestMeasureOneCrustType(t *testing.T) {
	w, _ := constructed(t, 500)
	crust := w.Attribute(yaee.AttrCrustType).Uint8s
	clear(crust)
	s, err := Measure(w)
	if err != nil {
		t.Fatal(err)
	}
	if s.ContinentalFraction != 0 || s.ContinentalElevation != (Distribution{}) || s.Terranes != (Regions{}) || s.Continents != (Regions{}) {
		t.Errorf("an oceanic world has continental statistics: %+v", s)
	}
	for i := range crust {
		crust[i] = yaee.CrustContinental
	}
	s, err = Measure(w)
	if err != nil {
		t.Fatal(err)
	}
	near(t, "continental_fraction", s.ContinentalFraction, 1, 1e-12)
	near(t, "continents.largest_km2", s.Continents.Largest, s.Area, 1e-6)
	if s.OceanicElevation != (Distribution{}) || s.OceanicAge != (Ages{}) || s.Continents.Count != 1 {
		t.Errorf("a continental world has oceanic statistics or more than one continent: %+v", s)
	}
}

// A plate number that no sample has counts as a plate of no area.
func TestMeasureEmptyPlate(t *testing.T) {
	w, _ := constructed(t, 500)
	plate := w.Attribute(yaee.AttrPlate).Int32s
	for i := range plate {
		if plate[i] == 2 {
			plate[i] = 5
		}
	}
	s, err := Measure(w)
	if err != nil {
		t.Fatal(err)
	}
	if s.Plates.Count != 6 || s.Plates.Smallest != 0 {
		t.Errorf("plates %+v, want 6 with the smallest 0", s.Plates)
	}
}

func TestMeasureErrors(t *testing.T) {
	tests := []struct {
		name   string
		damage func(w *yaee.World)
		want   error
		text   string
	}{
		{"no elevation", func(w *yaee.World) { w.Attribute(yaee.AttrElevation).Name = "height" }, ErrNoAttribute, `"elevation"`},
		{"wrong type", func(w *yaee.World) {
			a := w.Attribute(yaee.AttrCrustType)
			a.Type, a.Int32s, a.Uint8s = yaee.Int32, make([]int32, len(a.Uint8s)), nil
		}, ErrNoAttribute, `"crust_type" is int32`},
		{"short attribute", func(w *yaee.World) {
			a := w.Attribute(yaee.AttrPlate)
			a.Int32s = a.Int32s[:10]
		}, ErrNoAttribute, `"plate"`},
		{"NaN elevation", func(w *yaee.World) { w.Attribute(yaee.AttrElevation).Float64s[7] = math.NaN() }, ErrValue, "sample 7"},
		{"infinite age", func(w *yaee.World) { w.Attribute(yaee.AttrOceanicAge).Float64s[8] = math.Inf(1) }, ErrValue, "sample 8"},
		{"negative plate", func(w *yaee.World) { w.Attribute(yaee.AttrPlate).Int32s[9] = -1 }, ErrValue, "sample 9"},
		{"not unit vectors", func(w *yaee.World) { w.Positions[3] = [3]float64{2, 0, 0} }, ErrWorld, "sample 3"},
		{"missing triangle", func(w *yaee.World) { w.Triangles = w.Triangles[1:] }, ErrWorld, ""},
	}
	for _, tt := range tests {
		w, _ := constructed(t, 300)
		tt.damage(w)
		s, err := Measure(w)
		if s != nil || !errors.Is(err, tt.want) || !strings.Contains(err.Error(), tt.text) {
			t.Errorf("%s: got %v, want an error wrapping %q and containing %q", tt.name, err, tt.want, tt.text)
		}
	}
}

// Keys names every member of the JSON encoding, in order, each with a
// unit and a description, and Help prints them all.
func TestKeysMatchJSON(t *testing.T) {
	w, _ := constructed(t, 300)
	s, err := Measure(w)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Measure(w)
	data2, _ := json.Marshal(again)
	if !bytes.Equal(data, data2) {
		t.Error("two measurements of one world gave different JSON")
	}

	// The members, in the order the encoding has them.
	var paths []string
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func(prefix string)
	walk = func(prefix string) {
		for dec.More() {
			tok, err := dec.Token()
			if err != nil {
				t.Fatal(err)
			}
			path := prefix + tok.(string)
			paths = append(paths, path)
			tok, err = dec.Token()
			if err != nil {
				t.Fatal(err)
			}
			switch tok {
			case json.Delim('{'):
				walk(path + ".")
				dec.Token() // the closing brace
			case json.Delim('['):
				for dec.More() {
					dec.Token()
				}
				dec.Token() // the closing bracket
			}
		}
	}
	if tok, _ := dec.Token(); tok != json.Delim('{') {
		t.Fatalf("the encoding starts with %v, not an object", tok)
	}
	walk("")

	keys := Keys()
	var names []string
	help := Help()
	for _, k := range keys {
		names = append(names, k.Path)
		if k.Unit == "" || k.Doc == "" {
			t.Errorf("key %q has unit %q and description %q; both are required", k.Path, k.Unit, k.Doc)
		}
		if !strings.Contains("\n"+help, "\n  "+k.Path+"\n      Unit: "+k.Unit+"\n      "+k.Doc+"\n") {
			t.Errorf("Help does not describe %q", k.Path)
		}
	}
	if !slices.Equal(names, paths) {
		t.Errorf("Keys lists\n%q\nbut the encoding has\n%q", names, paths)
	}
}
