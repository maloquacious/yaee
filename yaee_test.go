// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os/exec"
	"reflect"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/maloquacious/yaee"
)

// smallConfig returns a valid Config with about n samples.
func smallConfig(n int) yaee.Config {
	cfg := yaee.DefaultConfig()
	cfg.SampleSpacing = math.Sqrt(8 * math.Pi * cfg.Radius * cfg.Radius / (math.Sqrt(3) * float64(n)))
	cfg.Plates = min(cfg.Plates, n)
	return cfg
}

func mustGenerate(t testing.TB, cfg yaee.Config) *yaee.World {
	t.Helper()
	w, err := yaee.Generate(t.Context(), cfg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	return w
}

func TestDefaultConfig(t *testing.T) {
	cfg := yaee.DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DefaultConfig is invalid: %v", err)
	}
	want := yaee.Config{
		Seed: 0, Radius: 6370, SampleSpacing: 25, Duration: 250, TimeStep: 2,
		Plates: 40, PlateWarpAmplitude: 1500, PlateWarpFeatureSize: 2500,
		LandCoverage: 0.3, RidgeElevation: -1, AbyssalElevation: -6, TrenchElevation: -10, MaxContinentalElevation: 10,
		MaxPlateSpeed: 100, OceanicDampening: 0.04,
		SubductionDistance: 1800, SubductionUplift: 0.6, ContinentalErosion: 0.03, SedimentAccretion: 0.3,
		CollisionPenetration: 300, CollisionDistance: 4200, CollisionCoefficient: 1.3e-5,
		RiftingRate: 0.004,
	}
	if cfg != want {
		t.Fatalf("DefaultConfig() = %+v, want %+v", cfg, want)
	}
}

func TestZeroConfigInvalid(t *testing.T) {
	err := yaee.Config{}.Validate()
	if !errors.Is(err, yaee.ErrInvalidConfig) {
		t.Fatalf("zero Config: got %v, want ErrInvalidConfig", err)
	}
	// Zero is legitimate for Seed, Duration, PlateWarpAmplitude,
	// LandCoverage, MaxPlateSpeed, OceanicDampening, SubductionDistance,
	// SubductionUplift, ContinentalErosion, SedimentAccretion,
	// CollisionPenetration, CollisionDistance, CollisionCoefficient, and
	// RiftingRate, and
	// for nothing else. A zero TrenchElevation is invalid only because it is not
	// below the AbyssalElevation.
	for _, field := range []string{"Radius", "SampleSpacing", "TimeStep", "Plates", "PlateWarpFeatureSize",
		"RidgeElevation", "AbyssalElevation", "TrenchElevation", "MaxContinentalElevation"} {
		if !strings.Contains(err.Error(), ": "+field+": ") {
			t.Errorf("error does not name %s: %v", field, err)
		}
	}
	for _, field := range []string{"Seed", "Duration", "PlateWarpAmplitude", "LandCoverage", "MaxPlateSpeed", "OceanicDampening",
		"SubductionDistance", "SubductionUplift", "ContinentalErosion", "SedimentAccretion",
		"CollisionPenetration", "CollisionDistance", "CollisionCoefficient", "RiftingRate"} {
		if strings.Contains(err.Error(), ": "+field+": ") {
			t.Errorf("error names %s, whose zero is valid: %v", field, err)
		}
	}
}

func TestValidateNamesField(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(1)
	tests := []struct {
		field string
		set   func(*yaee.Config)
	}{
		{"Radius", func(c *yaee.Config) { c.Radius = 0 }},
		{"Radius", func(c *yaee.Config) { c.Radius = -6370 }},
		{"Radius", func(c *yaee.Config) { c.Radius = nan }},
		{"Radius", func(c *yaee.Config) { c.Radius = inf }},
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = 0 }},
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = -25 }},
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = nan }},
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = inf }},
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = 20000 }}, // 1 sample
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = 1 }},     // 589 million samples
		{"Plates", func(c *yaee.Config) { c.Plates = 0 }},
		{"Plates", func(c *yaee.Config) { c.Plates = -3 }},
		{"Plates", func(c *yaee.Config) { c.Plates = yaee.MaxPlates + 1 }},
		{"Plates", func(c *yaee.Config) { c.SampleSpacing = 5000; c.Plates = 25 }}, // 24 samples
		{"PlateWarpAmplitude", func(c *yaee.Config) { c.PlateWarpAmplitude = -1 }},
		{"PlateWarpAmplitude", func(c *yaee.Config) { c.PlateWarpAmplitude = nan }},
		{"PlateWarpAmplitude", func(c *yaee.Config) { c.PlateWarpAmplitude = inf }},
		{"PlateWarpFeatureSize", func(c *yaee.Config) { c.PlateWarpFeatureSize = 0 }},
		{"PlateWarpFeatureSize", func(c *yaee.Config) { c.PlateWarpFeatureSize = -2500 }},
		{"PlateWarpFeatureSize", func(c *yaee.Config) { c.PlateWarpFeatureSize = nan }},
		{"PlateWarpFeatureSize", func(c *yaee.Config) { c.PlateWarpFeatureSize = inf }},
		{"LandCoverage", func(c *yaee.Config) { c.LandCoverage = -0.01 }},
		{"LandCoverage", func(c *yaee.Config) { c.LandCoverage = 1.01 }},
		{"LandCoverage", func(c *yaee.Config) { c.LandCoverage = nan }},
		{"LandCoverage", func(c *yaee.Config) { c.LandCoverage = inf }},
		{"RidgeElevation", func(c *yaee.Config) { c.RidgeElevation = 0 }},
		{"RidgeElevation", func(c *yaee.Config) { c.RidgeElevation = 1 }},
		{"RidgeElevation", func(c *yaee.Config) { c.RidgeElevation = inf }},
		{"AbyssalElevation", func(c *yaee.Config) { c.AbyssalElevation = -1 }}, // equal to the ridge
		{"AbyssalElevation", func(c *yaee.Config) { c.AbyssalElevation = -0.5 }},
		{"AbyssalElevation", func(c *yaee.Config) { c.AbyssalElevation = nan }},
		{"TrenchElevation", func(c *yaee.Config) { c.TrenchElevation = -6 }}, // equal to the abyssal plain
		{"TrenchElevation", func(c *yaee.Config) { c.TrenchElevation = -5 }},
		{"TrenchElevation", func(c *yaee.Config) { c.TrenchElevation = nan }},
		{"TrenchElevation", func(c *yaee.Config) { c.TrenchElevation = -inf }},
		{"MaxContinentalElevation", func(c *yaee.Config) { c.MaxContinentalElevation = 0 }},
		{"MaxContinentalElevation", func(c *yaee.Config) { c.MaxContinentalElevation = -10 }},
		{"MaxContinentalElevation", func(c *yaee.Config) { c.MaxContinentalElevation = nan }},
		{"MaxContinentalElevation", func(c *yaee.Config) { c.MaxContinentalElevation = inf }},
		{"MaxPlateSpeed", func(c *yaee.Config) { c.MaxPlateSpeed = -1 }},
		{"MaxPlateSpeed", func(c *yaee.Config) { c.MaxPlateSpeed = nan }},
		{"MaxPlateSpeed", func(c *yaee.Config) { c.MaxPlateSpeed = inf }},
		{"OceanicDampening", func(c *yaee.Config) { c.OceanicDampening = -0.01 }},
		{"OceanicDampening", func(c *yaee.Config) { c.OceanicDampening = nan }},
		{"OceanicDampening", func(c *yaee.Config) { c.OceanicDampening = inf }},
		{"SubductionDistance", func(c *yaee.Config) { c.SubductionDistance = -1 }},
		{"SubductionDistance", func(c *yaee.Config) { c.SubductionDistance = nan }},
		{"SubductionDistance", func(c *yaee.Config) { c.SubductionDistance = inf }},
		{"SubductionUplift", func(c *yaee.Config) { c.SubductionUplift = -0.1 }},
		{"SubductionUplift", func(c *yaee.Config) { c.SubductionUplift = nan }},
		{"SubductionUplift", func(c *yaee.Config) { c.SubductionUplift = inf }},
		{"ContinentalErosion", func(c *yaee.Config) { c.ContinentalErosion = -0.01 }},
		{"ContinentalErosion", func(c *yaee.Config) { c.ContinentalErosion = nan }},
		{"ContinentalErosion", func(c *yaee.Config) { c.ContinentalErosion = inf }},
		{"SedimentAccretion", func(c *yaee.Config) { c.SedimentAccretion = -0.1 }},
		{"SedimentAccretion", func(c *yaee.Config) { c.SedimentAccretion = nan }},
		{"SedimentAccretion", func(c *yaee.Config) { c.SedimentAccretion = inf }},
		{"CollisionPenetration", func(c *yaee.Config) { c.CollisionPenetration = -1 }},
		{"CollisionPenetration", func(c *yaee.Config) { c.CollisionPenetration = nan }},
		{"CollisionDistance", func(c *yaee.Config) { c.CollisionDistance = -1 }},
		{"CollisionDistance", func(c *yaee.Config) { c.CollisionDistance = inf }},
		{"CollisionCoefficient", func(c *yaee.Config) { c.CollisionCoefficient = -1e-9 }},
		{"CollisionCoefficient", func(c *yaee.Config) { c.CollisionCoefficient = nan }},
		{"RiftingRate", func(c *yaee.Config) { c.RiftingRate = -0.001 }},
		{"RiftingRate", func(c *yaee.Config) { c.RiftingRate = inf }},
		{"Duration", func(c *yaee.Config) { c.Duration = 2*yaee.MaxSteps + 1 }},
		{"Duration", func(c *yaee.Config) { c.Duration = 1; c.TimeStep = 1e-7 }},
		{"SampleSpacing", func(c *yaee.Config) { c.SampleSpacing = 1e-300 }},
		{"Duration", func(c *yaee.Config) { c.Duration = -1 }},
		{"Duration", func(c *yaee.Config) { c.Duration = nan }},
		{"Duration", func(c *yaee.Config) { c.Duration = inf }},
		{"TimeStep", func(c *yaee.Config) { c.TimeStep = 0 }},
		{"TimeStep", func(c *yaee.Config) { c.TimeStep = -2 }},
		{"TimeStep", func(c *yaee.Config) { c.TimeStep = nan }},
		{"TimeStep", func(c *yaee.Config) { c.TimeStep = inf }},
	}
	for _, tt := range tests {
		cfg := yaee.DefaultConfig()
		tt.set(&cfg)
		err := cfg.Validate()
		if !errors.Is(err, yaee.ErrInvalidConfig) {
			t.Errorf("%+v: got %v, want ErrInvalidConfig", cfg, err)
			continue
		}
		if !strings.HasPrefix(err.Error(), "yaee: invalid config: "+tt.field+": ") || strings.Contains(err.Error(), "\n") {
			t.Errorf("%+v: error should name %s and nothing else: %v", cfg, tt.field, err)
		}
		if _, err := yaee.Generate(t.Context(), cfg); !errors.Is(err, yaee.ErrInvalidConfig) {
			t.Errorf("Generate(%+v): got %v, want ErrInvalidConfig", cfg, err)
		}
	}
}

func TestValidateAcceptsBoundaries(t *testing.T) {
	for _, set := range []func(*yaee.Config){
		func(c *yaee.Config) { c.Seed = math.MaxUint64 },
		func(c *yaee.Config) { c.Duration = 0 },
		func(c *yaee.Config) { *c = smallConfig(yaee.MinSamples) },
		func(c *yaee.Config) { c.Radius = 1; c.SampleSpacing = 0.5 },
		func(c *yaee.Config) { c.Plates = 1 },
		func(c *yaee.Config) { c.Plates = yaee.MaxPlates },
		func(c *yaee.Config) { c.SampleSpacing = 5000; c.Plates = 24 }, // a plate per sample
		func(c *yaee.Config) { c.PlateWarpAmplitude = 0 },
		func(c *yaee.Config) { c.LandCoverage = 0 },
		func(c *yaee.Config) { c.LandCoverage = 1 },
		func(c *yaee.Config) { c.MaxPlateSpeed = 0 },
		func(c *yaee.Config) { c.OceanicDampening = 0 },
		func(c *yaee.Config) { c.OceanicDampening = 1000 },
		func(c *yaee.Config) { c.SubductionDistance = 0 },
		func(c *yaee.Config) { c.SubductionDistance = 1e6 },
		func(c *yaee.Config) { c.SubductionUplift = 0 },
		func(c *yaee.Config) { c.ContinentalErosion = 0 },
		func(c *yaee.Config) { c.ContinentalErosion = 1000 },
		func(c *yaee.Config) { c.SedimentAccretion = 0 },
		func(c *yaee.Config) { c.CollisionPenetration = 0 },
		func(c *yaee.Config) { c.CollisionDistance = 0 },
		func(c *yaee.Config) { c.CollisionCoefficient = 0 },
		func(c *yaee.Config) { c.RiftingRate = 0 },
		func(c *yaee.Config) { c.Duration = 2 * yaee.MaxSteps },
		func(c *yaee.Config) {
			c.RidgeElevation, c.AbyssalElevation, c.TrenchElevation, c.MaxContinentalElevation = -1e-9, -2e-9, -3e-9, 1e-9
		},
	} {
		cfg := yaee.DefaultConfig()
		set(&cfg)
		if err := cfg.Validate(); err != nil {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	cfg := yaee.Config{
		Seed: math.MaxUint64, Radius: 6371.0088, SampleSpacing: 0.1 + 0.2, Duration: 1e-9, TimeStep: math.Pi,
		Plates: 7, PlateWarpAmplitude: 1e-300, PlateWarpFeatureSize: 1.0 / 3,
		LandCoverage: 1.0 / 7, RidgeElevation: -0.1, AbyssalElevation: -math.E, TrenchElevation: -1e17, MaxContinentalElevation: 8.848,
		MaxPlateSpeed: 77.7, OceanicDampening: 0.1 / 3,
		SubductionDistance: 1234.5, SubductionUplift: 2.0 / 3, ContinentalErosion: 1e-5, SedimentAccretion: 0.7,
		CollisionPenetration: 123.25, CollisionDistance: 3999.5, CollisionCoefficient: 2.5e-6,
		RiftingRate: 1.0 / 300,
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	var got yaee.Config
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Fatalf("round trip changed the Config:\n got %+v\nwant %+v", got, cfg)
	}

	data, err = json.Marshal(yaee.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"seed":0,"radius_km":6370,"sample_spacing_km":25,"duration_my":250,"time_step_my":2,` +
		`"plates":40,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,` +
		`"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,` +
		`"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,` +
		`"oceanic_dampening_km_per_my":0.04,"subduction_distance_km":1800,"subduction_uplift_km_per_my":0.6,` +
		`"continental_erosion_km_per_my":0.03,"sediment_accretion_km_per_my":0.3,` +
		`"collision_penetration_km":300,"collision_distance_km":4200,"collision_coefficient_per_km":0.000013,"rifting_rate_per_my":0.004}`
	if string(data) != want {
		t.Fatalf("DefaultConfig as JSON:\n got %s\nwant %s", data, want)
	}
}

func TestGenerateShape(t *testing.T) {
	cfg := smallConfig(500)
	cfg.Duration = 0 // the initial world; TestGenerateSimulated moves the plates
	w := mustGenerate(t, cfg)
	if w.Config != cfg {
		t.Errorf("Config = %+v, want %+v", w.Config, cfg)
	}
	if w.Version != yaee.Version().String() {
		t.Errorf("Version = %q, want %q", w.Version, yaee.Version().String())
	}
	if w.NumSamples() != 500 || w.NumTriangles() != 2*500-4 {
		t.Fatalf("%d samples and %d triangles, want 500 and 996", w.NumSamples(), w.NumTriangles())
	}
	for i, p := range w.Positions {
		if l := math.Sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2]); math.Abs(l-1) > 1e-12 {
			t.Fatalf("position %d has length %v", i, l)
		}
		if i > 0 && p[2] >= w.Positions[i-1][2] {
			t.Fatalf("z does not decrease at sample %d", i)
		}
	}
	// Every attribute, in order: name, unit, type, components.
	want := []struct {
		name, unit string
		typ        yaee.ElementType
		components int
	}{
		{yaee.AttrPlate, "", yaee.Int32, 1},
		{yaee.AttrCrustType, "", yaee.Uint8, 1},
		{yaee.AttrThickness, "km", yaee.Float64, 1},
		{yaee.AttrElevation, "km", yaee.Float64, 1},
		{yaee.AttrOceanicAge, "My", yaee.Float64, 1},
		{yaee.AttrRidgeDirection, "", yaee.Float64, 3},
		{yaee.AttrOrogenyAge, "My", yaee.Float64, 1},
		{yaee.AttrOrogenyType, "", yaee.Uint8, 1},
		{yaee.AttrFoldDirection, "", yaee.Float64, 3},
		{yaee.AttrUplift, "km/My", yaee.Float64, 1},
	}
	if len(w.Attributes) != len(want) {
		t.Fatalf("%d attributes, want %d", len(w.Attributes), len(want))
	}
	for k, wa := range want {
		a := &w.Attributes[k]
		if a.Name != wa.name || a.Unit != wa.unit || a.Type != wa.typ || a.Components != wa.components {
			t.Fatalf("attribute %d is %q %q %v x%d, want %q %q %v x%d",
				k, a.Name, a.Unit, a.Type, a.Components, wa.name, wa.unit, wa.typ, wa.components)
		}
		if w.Attribute(wa.name) != a {
			t.Fatalf("Attribute(%q) is not attribute %d", wa.name, k)
		}
		lens := map[yaee.ElementType]int{
			yaee.Uint8: len(a.Uint8s), yaee.Int32: len(a.Int32s), yaee.Float32: len(a.Float32s), yaee.Float64: len(a.Float64s),
		}
		for typ, n := range lens {
			if typ == a.Type && n != 500*a.Components {
				t.Fatalf("%s has %d values, want %d", a.Name, n, 500*a.Components)
			}
			if typ != a.Type && n != 0 {
				t.Fatalf("%s has data in a slice its type does not select", a.Name)
			}
		}
	}

	plate := w.Attribute(yaee.AttrPlate).Int32s
	used := make([]bool, cfg.Plates)
	for i, k := range plate {
		if k < 0 || int(k) >= cfg.Plates {
			t.Fatalf("sample %d has plate %d, outside 0 to %d", i, k, cfg.Plates-1)
		}
		used[k] = true
	}
	for k, u := range used {
		if !u {
			t.Fatalf("plate %d has no samples", k)
		}
	}

	// The initial crust: both types are present, land is above sea
	// level and ocean below, and every value is in its documented range.
	crust := w.Attribute(yaee.AttrCrustType).Uint8s
	thickness := w.Attribute(yaee.AttrThickness).Float64s
	elevation := w.Attribute(yaee.AttrElevation).Float64s
	oceanicAge := w.Attribute(yaee.AttrOceanicAge).Float64s
	orogenyAge := w.Attribute(yaee.AttrOrogenyAge).Float64s
	orogeny := w.Attribute(yaee.AttrOrogenyType).Uint8s
	var count [2]int
	for i := range 500 {
		z := elevation[i]
		switch crust[i] {
		case yaee.CrustOceanic:
			if thickness[i] != 7 || z < cfg.AbyssalElevation || z > cfg.RidgeElevation || !(oceanicAge[i] > 0) {
				t.Fatalf("oceanic sample %d: thickness %v, elevation %v, age %v", i, thickness[i], z, oceanicAge[i])
			}
		case yaee.CrustContinental:
			if thickness[i] != 35 || z < 0.01*cfg.MaxContinentalElevation || z > 0.05*cfg.MaxContinentalElevation || oceanicAge[i] != 0 {
				t.Fatalf("continental sample %d: thickness %v, elevation %v, age %v", i, thickness[i], z, oceanicAge[i])
			}
		default:
			t.Fatalf("sample %d has crust type %d", i, crust[i])
		}
		count[crust[i]]++
		if orogeny[i] != yaee.OrogenyNone || orogenyAge[i] != 0 {
			t.Fatalf("sample %d starts with orogeny type %d, age %v", i, orogeny[i], orogenyAge[i])
		}
	}
	// Samples have nearly equal areas, so 0.3 of the area is about 150
	// of 500 samples.
	if count[yaee.CrustContinental] < 140 || count[yaee.CrustContinental] > 160 {
		t.Fatalf("%d of 500 samples are continental; want about 150", count[yaee.CrustContinental])
	}
	for _, name := range []string{yaee.AttrRidgeDirection, yaee.AttrFoldDirection} {
		for i, v := range w.Attribute(name).Float64s {
			if v != 0 {
				t.Fatalf("%s[%d] = %v, want 0", name, i, v)
			}
		}
	}
	if w.Attribute("no such attribute") != nil {
		t.Fatal("Attribute returned a value for an unknown name")
	}
}

// A world whose plates have moved keeps every attribute in its
// documented range, and differs from the initial world.
func TestGenerateSimulated(t *testing.T) {
	cfg := smallConfig(3000)
	w := mustGenerate(t, cfg)
	initial := cfg
	initial.Duration = 0
	start := mustGenerate(t, initial)
	if reflect.DeepEqual(w.Attributes, start.Attributes) {
		t.Fatal("250 My of simulation left the world as it started")
	}
	if !reflect.DeepEqual(w.Positions, start.Positions) || !reflect.DeepEqual(w.Triangles, start.Triangles) {
		t.Fatal("the simulation changed the samples or the triangles")
	}
	still := cfg
	still.MaxPlateSpeed = 0
	// Plates that do not move leave the crust where it was; the ocean
	// still ages and sinks, and the land erodes.
	rest := mustGenerate(t, still)
	for _, name := range []string{yaee.AttrPlate, yaee.AttrCrustType, yaee.AttrThickness, yaee.AttrRidgeDirection} {
		if !reflect.DeepEqual(rest.Attribute(name), start.Attribute(name)) {
			t.Fatalf("with MaxPlateSpeed 0, %s is not as in the initial world", name)
		}
	}
	for i, kind := range rest.Attribute(yaee.AttrCrustType).Uint8s {
		age, age0 := rest.Attribute(yaee.AttrOceanicAge).Float64s[i], start.Attribute(yaee.AttrOceanicAge).Float64s[i]
		z, z0 := rest.Attribute(yaee.AttrElevation).Float64s[i], start.Attribute(yaee.AttrElevation).Float64s[i]
		eroded := z0
		if z0 > 0 {
			// 125 steps of 2 My.
			eroded = z0 * math.Pow(1-cfg.ContinentalErosion*cfg.TimeStep/cfg.MaxContinentalElevation, 125)
		}
		if kind == yaee.CrustContinental && (age != 0 || math.Abs(z-eroded) > 1e-12) {
			t.Fatalf("with MaxPlateSpeed 0, continental sample %d went from %v km to %v km, age %v", i, z0, z, age)
		}
		if kind == yaee.CrustOceanic && (math.Abs(age-age0-cfg.Duration) > 1e-6 || !(z < z0) || z < cfg.TrenchElevation) {
			t.Fatalf("with MaxPlateSpeed 0, oceanic sample %d went from %v km, %v My to %v km, %v My", i, z0, age0, z, age)
		}
	}
	undamped := still
	undamped.OceanicDampening, undamped.ContinentalErosion = 0, 0
	if flat := mustGenerate(t, undamped); !reflect.DeepEqual(flat.Attribute(yaee.AttrElevation), start.Attribute(yaee.AttrElevation)) {
		t.Fatal("with MaxPlateSpeed 0, OceanicDampening 0, and ContinentalErosion 0 the elevation is not that of the initial world")
	}
	// A shorter last step: 5 My in steps of 2 is not 4 or 6 My.
	short := cfg
	short.Duration = 5
	four, six := short, short
	four.Duration, six.Duration = 4, 6
	a, b, c := mustGenerate(t, short), mustGenerate(t, four), mustGenerate(t, six)
	if reflect.DeepEqual(a.Attributes, b.Attributes) || reflect.DeepEqual(a.Attributes, c.Attributes) {
		t.Fatal("a Duration of 5 My gave the world of 4 or 6 My")
	}

	n := w.NumSamples()
	plate := w.Attribute(yaee.AttrPlate).Int32s
	crust := w.Attribute(yaee.AttrCrustType).Uint8s
	thickness := w.Attribute(yaee.AttrThickness).Float64s
	elevation := w.Attribute(yaee.AttrElevation).Float64s
	oceanicAge := w.Attribute(yaee.AttrOceanicAge).Float64s
	ridge := w.Attribute(yaee.AttrRidgeDirection).Float64s
	orogenyAge := w.Attribute(yaee.AttrOrogenyAge).Float64s
	orogeny := w.Attribute(yaee.AttrOrogenyType).Uint8s
	fold := w.Attribute(yaee.AttrFoldDirection).Float64s
	// Every plate number up to the greatest is in use.
	used := make([]bool, slices.Max(plate)+1)
	for _, k := range plate {
		if k >= 0 {
			used[k] = true
		}
	}
	if slices.Contains(used, false) {
		t.Fatalf("a plate number below the greatest, %d, is used by no sample", len(used)-1)
	}
	var land int
	for i := range n {
		if plate[i] < 0 {
			t.Fatalf("sample %d has plate %d", i, plate[i])
		}
		if crust[i] != yaee.CrustOceanic && crust[i] != yaee.CrustContinental {
			t.Fatalf("sample %d has crust type %d", i, crust[i])
		}
		if orogeny[i] > yaee.OrogenyHimalayan {
			t.Fatalf("sample %d has orogeny type %d", i, orogeny[i])
		}
		land += int(crust[i])
		values := []float64{thickness[i], elevation[i], oceanicAge[i], orogenyAge[i]}
		values = append(values, ridge[3*i:3*i+3]...)
		values = append(values, fold[3*i:3*i+3]...)
		for _, v := range values {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("sample %d has a value that is not finite: %v", i, values)
			}
		}
		if z := elevation[i]; z < cfg.TrenchElevation || z > cfg.MaxContinentalElevation {
			t.Fatalf("sample %d has elevation %v", i, z)
		}
		if !(thickness[i] > 0) || oceanicAge[i] < 0 || orogenyAge[i] < 0 {
			t.Fatalf("sample %d: thickness %v, oceanic age %v, orogeny age %v", i, thickness[i], oceanicAge[i], orogenyAge[i])
		}
		if crust[i] == yaee.CrustContinental && (oceanicAge[i] != 0 || ridge[3*i] != 0 || ridge[3*i+1] != 0 || ridge[3*i+2] != 0) {
			t.Fatalf("continental sample %d has an oceanic age or a ridge direction", i)
		}
		if orogeny[i] == yaee.OrogenyNone && orogenyAge[i] != 0 {
			t.Fatalf("sample %d has an orogeny age without an orogeny", i)
		}
	}
	if land < n/10 || land > n/2 {
		t.Fatalf("%d of %d samples are continental after 250 My; the world started with 0.3", land, n)
	}
}

func TestGenerateRepeatable(t *testing.T) {
	cfg := smallConfig(3000)
	a, b := mustGenerate(t, cfg), mustGenerate(t, cfg)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("two calls with equal Configs returned different worlds")
	}
}

// The number of processors must not change the world.
func TestGenerateIndependentOfProcessors(t *testing.T) {
	cfg := smallConfig(20000)
	many := mustGenerate(t, cfg)
	prev := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(prev)
	one := mustGenerate(t, cfg)
	if !reflect.DeepEqual(one, many) {
		t.Fatalf("the world differs between 1 processor and %d", prev)
	}
}

// A different seed gives a different world, and nothing but the seed
// needs to change for that.
func TestSeedChangesTheWorld(t *testing.T) {
	cfg := smallConfig(1000)
	a := mustGenerate(t, cfg)
	cfg.Seed = 12345
	b := mustGenerate(t, cfg)
	if !reflect.DeepEqual(a.Positions, b.Positions) || !reflect.DeepEqual(a.Triangles, b.Triangles) {
		t.Fatal("the seed changed the samples or the triangles, which depend only on Radius and SampleSpacing")
	}
	pa, pb := a.Attribute(yaee.AttrPlate).Int32s, b.Attribute(yaee.AttrPlate).Int32s
	var differ int
	for i := range pa {
		if pa[i] != pb[i] {
			differ++
		}
	}
	if differ < len(pa)/2 {
		t.Fatalf("seeds 0 and 12345 give the same plate to all but %d of %d samples", differ, len(pa))
	}
}

func TestGenerateCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w, err := yaee.Generate(ctx, smallConfig(1000))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
	if w != nil {
		t.Fatal("Generate returned a world along with an error")
	}
}

func TestElementType(t *testing.T) {
	tests := []struct {
		typ  yaee.ElementType
		code uint8
		size int
		name string
	}{
		{yaee.Uint8, 1, 1, "uint8"},
		{yaee.Int32, 2, 4, "int32"},
		{yaee.Float32, 3, 4, "float32"},
		{yaee.Float64, 4, 8, "float64"},
		{yaee.ElementType(0), 0, 0, "ElementType(0)"},
		{yaee.ElementType(9), 9, 0, "ElementType(9)"},
	}
	for _, tt := range tests {
		if uint8(tt.typ) != tt.code || tt.typ.Size() != tt.size || tt.typ.String() != tt.name {
			t.Errorf("%d: code %d, size %d, name %q; want %d, %d, %q",
				tt.code, uint8(tt.typ), tt.typ.Size(), tt.typ.String(), tt.code, tt.size, tt.name)
		}
	}
}

// Clients inherit every dependency of the root package, so it must not
// depend, even indirectly, on image encoding, flag parsing, or the
// packages that exist only for the command.
func TestRootPackageDependencies(t *testing.T) {
	out, err := exec.CommandContext(t.Context(), "go", "list", "-deps", ".").Output()
	if err != nil {
		t.Skipf("cannot list dependencies: %v", err)
	}
	deps := strings.Fields(string(out))
	if len(deps) < 10 {
		t.Fatalf("implausible dependency list: %q", deps)
	}
	for _, dep := range deps {
		switch {
		case dep == "flag", dep == "image", strings.HasPrefix(dep, "image/"),
			dep == "compress/zlib", dep == "compress/flate",
			strings.HasSuffix(dep, "/internal/render"),
			strings.HasSuffix(dep, "/internal/dotenv"),
			strings.Contains(dep, "godotenv"),
			strings.Contains(dep, "/cmd/"):
			t.Errorf("the root package depends on %s", dep)
		}
	}
}
