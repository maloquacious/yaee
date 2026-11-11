// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package stats measures a world: the numbers by which a generated
// world is judged and its defaults tuned. Everything is computed from a
// yaee.World, so from a datafile alone.
//
// The same world always gives the same Stats, and the same JSON bytes.
package stats

import (
	"fmt"
	"math"
	"reflect"
	"slices"
	"strings"

	"github.com/maloquacious/yaee"
	"github.com/maloquacious/yaee/internal/cerrs"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Errors returned by this package, wrapped with detail.
const (
	// ErrNoAttribute: the world lacks an attribute that is measured, or
	// the attribute does not have the type, component count, and length
	// that Generate gives it.
	ErrNoAttribute = cerrs.Error("stats: the world has no usable attribute")
	// ErrWorld: the world's samples and triangles are not a usable
	// triangulation of the sphere.
	ErrWorld = cerrs.Error("stats: the world cannot be measured")
	// ErrValue: an elevation or an oceanic age is NaN or infinite, or a
	// plate number is negative.
	ErrValue = cerrs.Error("stats: a value cannot be measured")
)

// BinWidth is the width of one bin of the elevation histogram, in km.
const BinWidth = 0.5

// Stats is what Measure reports. Each field's "unit" tag is the unit of
// its value, and its "doc" tag says what the value is; Keys lists them.
//
// The area of a sample is the area of the sphere it stands for: one
// third of the area of every triangle that has the sample as a corner.
// The areas of all samples sum to the area of the sphere. A fraction is
// a share of the sphere's area, not of the number of samples.
type Stats struct {
	LibraryVersion string  `json:"library_version" unit:"text" doc:"Version of the yaee library that generated the world."`
	Samples        int     `json:"samples" unit:"count" doc:"Number of samples."`
	Triangles      int     `json:"triangles" unit:"count" doc:"Number of triangles."`
	Radius         float64 `json:"radius_km" unit:"km" doc:"Radius of the sphere at sea level: the Config's radius_km."`
	Spacing        float64 `json:"spacing_km" unit:"km" doc:"Distance between neighbouring samples: sqrt(8 pi R^2 / (sqrt(3) N)) for N samples on a sphere of radius R."`
	Area           float64 `json:"area_km2" unit:"km2" doc:"Area of the sphere: 4 pi R^2."`

	ContinentalFraction    float64 `json:"continental_fraction" unit:"fraction of the sphere's area, 0 to 1" doc:"Area of the samples whose crust_type is 1 (continental)."`
	LandFraction           float64 `json:"land_fraction" unit:"fraction of the sphere's area, 0 to 1" doc:"Area of the samples whose elevation is above 0 km (sea level), whatever their crust."`
	OceanicLandFraction    float64 `json:"oceanic_land_fraction" unit:"fraction of the sphere's area, 0 to 1" doc:"Area of the samples of oceanic crust whose elevation is above 0 km."`
	ContinentalSeaFraction float64 `json:"continental_sea_fraction" unit:"fraction of the sphere's area, 0 to 1" doc:"Area of the samples of continental crust whose elevation is 0 km or below."`

	Elevation            Distribution `json:"elevation_km" doc:"Elevation of all samples."`
	ContinentalElevation Distribution `json:"continental_elevation_km" doc:"Elevation of the samples of continental crust. Every member is 0 if there are none."`
	OceanicElevation     Distribution `json:"oceanic_elevation_km" doc:"Elevation of the samples of oceanic crust. Every member is 0 if there are none."`
	Histogram            Histogram    `json:"elevation_histogram" doc:"Histogram of the elevation of all samples."`

	Plates     Plates  `json:"plates" doc:"The tectonic plates: the values of the plate attribute."`
	Terranes   Regions `json:"terranes" doc:"Terranes: sets of samples of continental crust on one plate, joined by neighbouring samples of continental crust on that plate."`
	Continents Regions `json:"continents" doc:"Continents: sets of samples of continental crust joined by neighbouring samples of continental crust, whatever their plates."`
	Landmasses Regions `json:"landmasses" doc:"Landmasses: sets of samples above 0 km joined by neighbouring samples above 0 km, whatever their crust and plates."`

	OceanicAge Ages    `json:"oceanic_age_my" doc:"The oceanic_age attribute of the samples of oceanic crust. Every member is 0 if there are none."`
	Orogeny    Orogeny `json:"orogeny" doc:"The orogeny_type attribute."`
}

// Distribution describes the elevations of a set of samples.
type Distribution struct {
	Min  float64 `json:"min" unit:"km" doc:"Least elevation."`
	Max  float64 `json:"max" unit:"km" doc:"Greatest elevation."`
	Mean float64 `json:"mean" unit:"km" doc:"Mean elevation, each sample weighted by its area."`
	P10  float64 `json:"p10" unit:"km" doc:"The least elevation z such that the samples at or below z hold at least 0.10 of the set's area."`
	P50  float64 `json:"p50" unit:"km" doc:"As p10, for 0.50 of the area: the median."`
	P90  float64 `json:"p90" unit:"km" doc:"As p10, for 0.90 of the area."`
}

// Histogram is the distribution of elevation over the sphere's area, in
// bins of equal width.
type Histogram struct {
	BinWidth float64   `json:"bin_width_km" unit:"km" doc:"Width of every bin: 0.5."`
	First    float64   `json:"first_bin_km" unit:"km" doc:"Lower edge of bin 0: the greatest multiple of bin_width_km that is not above the least elevation."`
	Bins     []float64 `json:"fractions" unit:"array of fractions of the sphere's area, each 0 to 1" doc:"Element i is the area of the samples whose elevation z has first_bin_km + i * bin_width_km <= z < first_bin_km + (i + 1) * bin_width_km. The last element holds the greatest elevation. The elements sum to 1."`
}

// Plates describes the tectonic plates.
type Plates struct {
	Count    int     `json:"count" unit:"count" doc:"Number of plates: 1 + the greatest plate number."`
	Largest  float64 `json:"largest_km2" unit:"km2" doc:"Area of the plate with the most area."`
	Smallest float64 `json:"smallest_km2" unit:"km2" doc:"Area of the plate with the least area. 0 if some plate number below the greatest has no sample."`
}

// Regions describes sets of connected samples.
type Regions struct {
	Count   int     `json:"count" unit:"count" doc:"Number of regions. A single sample with no neighbour of its kind is a region."`
	Largest float64 `json:"largest_km2" unit:"km2" doc:"Area of the region with the most area. 0 if there is no region."`
}

// Ages describes the ages of oceanic crust.
type Ages struct {
	Max  float64 `json:"max" unit:"My" doc:"Greatest age: the oldest oceanic crust."`
	Mean float64 `json:"mean" unit:"My" doc:"Mean age, each sample weighted by its area."`
}

// Orogeny describes where mountains were built.
type Orogeny struct {
	Andean    float64 `json:"andean_fraction" unit:"fraction of the sphere's area, 0 to 1" doc:"Area of the samples whose orogeny_type is 1 (raised by subduction)."`
	Himalayan float64 `json:"himalayan_fraction" unit:"fraction of the sphere's area, 0 to 1" doc:"Area of the samples whose orogeny_type is 2 (raised by continental collision)."`
}

// A Key documents one value of the JSON encoding of a Stats.
type Key struct {
	// Path is the member's name, with the names of the objects that
	// contain it before it, joined by ".": "plates.count".
	Path string
	// Unit is the unit of the value, or "object" for a member that
	// holds other members.
	Unit string
	// Doc says what the value is.
	Doc string
}

// Keys returns every member of the JSON encoding of a Stats, in the
// order they are written.
func Keys() []Key {
	var keys []Key
	var walk func(t reflect.Type, prefix string)
	walk = func(t reflect.Type, prefix string) {
		for f := range t.Fields() {
			path := prefix + f.Tag.Get("json")
			if f.Type.Kind() == reflect.Struct {
				keys = append(keys, Key{Path: path, Unit: "object", Doc: f.Tag.Get("doc")})
				walk(f.Type, path+".")
				continue
			}
			keys = append(keys, Key{Path: path, Unit: f.Tag.Get("unit"), Doc: f.Tag.Get("doc")})
		}
	}
	walk(reflect.TypeFor[Stats](), "")
	return keys
}

// Measure returns the statistics of a world.
//
// The world must have the attributes plate, crust_type, elevation,
// oceanic_age, and orogeny_type as Generate produces them; otherwise the
// error wraps ErrNoAttribute. It wraps ErrWorld if the samples and
// triangles are not a triangulation of the sphere, and ErrValue if an
// elevation or oceanic age is not finite or a plate number is negative.
//
// The cost is O(n log n) for n samples.
func Measure(w *yaee.World) (*Stats, error) {
	n := w.NumSamples()
	plateAttr, err := attribute(w, yaee.AttrPlate, yaee.Int32)
	if err != nil {
		return nil, err
	}
	crustAttr, err := attribute(w, yaee.AttrCrustType, yaee.Uint8)
	if err != nil {
		return nil, err
	}
	elevationAttr, err := attribute(w, yaee.AttrElevation, yaee.Float64)
	if err != nil {
		return nil, err
	}
	ageAttr, err := attribute(w, yaee.AttrOceanicAge, yaee.Float64)
	if err != nil {
		return nil, err
	}
	orogenyAttr, err := attribute(w, yaee.AttrOrogenyType, yaee.Uint8)
	if err != nil {
		return nil, err
	}
	plate, crust, z, age, orogeny := plateAttr.Int32s, crustAttr.Uint8s, elevationAttr.Float64s, ageAttr.Float64s, orogenyAttr.Uint8s

	numPlates := 0
	for i := range n {
		if math.IsNaN(z[i]) || math.IsInf(z[i], 0) {
			return nil, fmt.Errorf("%w: the elevation of sample %d is %g", ErrValue, i, z[i])
		}
		if math.IsNaN(age[i]) || math.IsInf(age[i], 0) {
			return nil, fmt.Errorf("%w: the oceanic age of sample %d is %g", ErrValue, i, age[i])
		}
		if plate[i] < 0 {
			return nil, fmt.Errorf("%w: the plate of sample %d is %d", ErrValue, i, plate[i])
		}
		numPlates = max(numPlates, int(plate[i])+1)
	}

	mesh, err := newMesh(w)
	if err != nil {
		return nil, err
	}
	// Areas as fractions of the sphere, summing to 1.
	areas := mesh.PointAreas()
	var total float64
	for _, a := range areas {
		total += a
	}
	for i := range areas {
		areas[i] /= total
	}

	r := w.Config.Radius
	sphereArea := 4 * math.Pi * r * r
	s := &Stats{
		LibraryVersion: w.Version,
		Samples:        n,
		Triangles:      w.NumTriangles(),
		Radius:         r,
		Spacing:        sphere.SampleSpacing(r, n),
		Area:           sphereArea,
	}

	continental := func(i int32) bool { return crust[i] == yaee.CrustContinental }
	land := func(i int32) bool { return z[i] > 0 }
	var ageSum, oceanicArea float64
	plateArea := make([]float64, numPlates)
	for i := range int32(n) {
		a := areas[i]
		plateArea[plate[i]] += a
		switch {
		case continental(i) && land(i):
			s.ContinentalFraction += a
			s.LandFraction += a
		case continental(i):
			s.ContinentalFraction += a
			s.ContinentalSeaFraction += a
		default:
			if land(i) {
				s.LandFraction += a
				s.OceanicLandFraction += a
			}
			oceanicArea += a
			ageSum += a * age[i]
			s.OceanicAge.Max = max(s.OceanicAge.Max, age[i])
		}
		switch orogeny[i] {
		case yaee.OrogenyAndean:
			s.Orogeny.Andean += a
		case yaee.OrogenyHimalayan:
			s.Orogeny.Himalayan += a
		}
	}
	if oceanicArea > 0 {
		s.OceanicAge.Mean = ageSum / oceanicArea
	}

	order := make([]int32, n)
	for i := range order {
		order[i] = int32(i)
	}
	slices.SortFunc(order, func(a, b int32) int {
		switch {
		case z[a] < z[b]:
			return -1
		case z[a] > z[b]:
			return 1
		}
		return int(a - b)
	})
	s.Elevation = distribution(order, z, areas, func(int32) bool { return true })
	s.ContinentalElevation = distribution(order, z, areas, continental)
	s.OceanicElevation = distribution(order, z, areas, func(i int32) bool { return !continental(i) })
	s.Histogram = histogram(z, areas)

	s.Plates.Count = numPlates
	if numPlates > 0 {
		s.Plates.Largest = slices.Max(plateArea) * sphereArea
		s.Plates.Smallest = slices.Min(plateArea) * sphereArea
	}
	s.Terranes = regions(mesh, areas, sphereArea, continental, func(i, j int32) bool { return plate[i] == plate[j] })
	s.Continents = regions(mesh, areas, sphereArea, continental, nil)
	s.Landmasses = regions(mesh, areas, sphereArea, land, nil)
	return s, nil
}

// attribute returns the named attribute if it has one component of the
// wanted type for every sample.
func attribute(w *yaee.World, name string, want yaee.ElementType) (*yaee.Attribute, error) {
	a := w.Attribute(name)
	if a == nil {
		return nil, fmt.Errorf("%w: no attribute named %q", ErrNoAttribute, name)
	}
	n := len(a.Uint8s) + len(a.Int32s) + len(a.Float32s) + len(a.Float64s)
	var got int
	switch want {
	case yaee.Uint8:
		got = len(a.Uint8s)
	case yaee.Int32:
		got = len(a.Int32s)
	case yaee.Float64:
		got = len(a.Float64s)
	}
	if a.Type != want || a.Components != 1 || got != w.NumSamples() {
		return nil, fmt.Errorf("%w: %q is %s with %d components and %d values for %d samples; want %s with 1 component",
			ErrNoAttribute, name, a.Type, a.Components, n, w.NumSamples(), want)
	}
	return a, nil
}

// newMesh builds the mesh of the world's own samples and triangles.
func newMesh(w *yaee.World) (*sphere.Mesh, error) {
	points := make([]sphere.Vec, len(w.Positions))
	for i, p := range w.Positions {
		points[i] = sphere.Vec{X: p[0], Y: p[1], Z: p[2]}
		if l2 := points[i].Dot(points[i]); !(math.Abs(l2-1) <= 1e-9) {
			return nil, fmt.Errorf("%w: sample %d is not a unit vector: squared length %g", ErrWorld, i, l2)
		}
	}
	tri := make([]int32, 0, 3*len(w.Triangles))
	for _, t := range w.Triangles {
		tri = append(tri, t[:]...)
	}
	mesh, err := sphere.NewMesh(points, tri)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrWorld, err)
	}
	return mesh, nil
}

// distribution describes the elevations of the samples that member
// accepts. order holds every sample, by rising elevation.
func distribution(order []int32, z, areas []float64, member func(int32) bool) Distribution {
	var d Distribution
	var area, sum float64
	first := true
	for _, i := range order {
		if !member(i) {
			continue
		}
		if first {
			d.Min, first = z[i], false
		}
		d.Max = z[i]
		area += areas[i]
		sum += areas[i] * z[i]
	}
	if first {
		return d
	}
	d.Mean = sum / area
	// The quantiles default to the maximum, so that rounding in the
	// running sum cannot leave one unset.
	d.P10, d.P50, d.P90 = d.Max, d.Max, d.Max
	quantiles := []struct {
		share float64
		at    *float64
	}{{0.10, &d.P10}, {0.50, &d.P50}, {0.90, &d.P90}}
	var below float64
	next := 0
	for _, i := range order {
		if !member(i) {
			continue
		}
		below += areas[i]
		for next < len(quantiles) && below >= quantiles[next].share*area {
			*quantiles[next].at = z[i]
			next++
		}
		if next == len(quantiles) {
			break
		}
	}
	return d
}

// histogram bins the elevations by area.
func histogram(z, areas []float64) Histogram {
	h := Histogram{BinWidth: BinWidth, Bins: []float64{}}
	if len(z) == 0 {
		return h
	}
	bin := func(v float64) int { return int(math.Floor(v / BinWidth)) }
	lo, hi := bin(slices.Min(z)), bin(slices.Max(z))
	h.First = float64(lo) * BinWidth
	h.Bins = make([]float64, hi-lo+1)
	for i, v := range z {
		h.Bins[bin(v)-lo] += areas[i]
	}
	return h
}

// regions counts the connected sets of the samples that member accepts,
// and finds the area of the largest. Two neighbouring members are
// joined if same accepts the pair; nil accepts every pair. same must be
// an equivalence, as "on the same plate" is.
func regions(mesh *sphere.Mesh, areas []float64, sphereArea float64, member func(int32) bool, same func(i, j int32) bool) Regions {
	n := len(mesh.Points)
	seen := make([]bool, n)
	var r Regions
	var queue []int32
	for i := range int32(n) {
		if seen[i] || !member(i) {
			continue
		}
		r.Count++
		seen[i] = true
		queue = append(queue[:0], i)
		var area float64
		for head := 0; head < len(queue); head++ {
			at := queue[head]
			area += areas[at]
			for _, j := range mesh.Neighbours(int(at)) {
				if !seen[j] && member(j) && (same == nil || same(at, j)) {
					seen[j] = true
					queue = append(queue, j)
				}
			}
		}
		r.Largest = max(r.Largest, area*sphereArea)
	}
	return r
}

// Help returns the description of every key of the JSON encoding, one
// key per paragraph, each line indented by two spaces.
func Help() string {
	var b strings.Builder
	for _, k := range Keys() {
		fmt.Fprintf(&b, "  %s\n      Unit: %s\n      %s\n", k.Path, k.Unit, k.Doc)
	}
	return b.String()
}
