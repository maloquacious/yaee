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
	"strings"
	"testing"

	"github.com/maloquacious/yaee"
)

// smallConfig returns a valid Config with about n samples.
func smallConfig(n int) yaee.Config {
	cfg := yaee.DefaultConfig()
	cfg.SampleSpacing = math.Sqrt(8 * math.Pi * cfg.Radius * cfg.Radius / (math.Sqrt(3) * float64(n)))
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
	want := yaee.Config{Seed: 0, Radius: 6370, SampleSpacing: 25, Duration: 250, TimeStep: 2}
	if cfg != want {
		t.Fatalf("DefaultConfig() = %+v, want %+v", cfg, want)
	}
}

func TestZeroConfigInvalid(t *testing.T) {
	err := yaee.Config{}.Validate()
	if !errors.Is(err, yaee.ErrInvalidConfig) {
		t.Fatalf("zero Config: got %v, want ErrInvalidConfig", err)
	}
	// Zero is legitimate for Seed and Duration, and for nothing else.
	for _, field := range []string{"Radius", "SampleSpacing", "TimeStep"} {
		if !strings.Contains(err.Error(), ": "+field+": ") {
			t.Errorf("error does not name %s: %v", field, err)
		}
	}
	for _, field := range []string{"Seed", "Duration"} {
		if strings.Contains(err.Error(), field) {
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
	} {
		cfg := yaee.DefaultConfig()
		set(&cfg)
		if err := cfg.Validate(); err != nil {
			t.Errorf("%+v: %v", cfg, err)
		}
	}
}

func TestConfigJSONRoundTrip(t *testing.T) {
	cfg := yaee.Config{Seed: math.MaxUint64, Radius: 6371.0088, SampleSpacing: 0.1 + 0.2, Duration: 1e-9, TimeStep: math.Pi}
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
	const want = `{"seed":0,"radius_km":6370,"sample_spacing_km":25,"duration_my":250,"time_step_my":2}`
	if string(data) != want {
		t.Fatalf("DefaultConfig as JSON:\n got %s\nwant %s", data, want)
	}
}

func TestGenerateShape(t *testing.T) {
	cfg := smallConfig(500)
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
	if len(w.Attributes) != 1 {
		t.Fatalf("%d attributes, want 1", len(w.Attributes))
	}
	a := w.Attribute(yaee.AttrElevation)
	if a == nil {
		t.Fatal("no elevation attribute")
	}
	if a.Unit != "km" || a.Type != yaee.Float64 || a.Components != 1 || len(a.Float64s) != 500 {
		t.Fatalf("elevation is %q %v x%d with %d values", a.Unit, a.Type, a.Components, len(a.Float64s))
	}
	if a.Uint8s != nil || a.Int32s != nil || a.Float32s != nil {
		t.Fatal("elevation has data in a slice its type does not select")
	}
	lo, hi := math.Inf(1), math.Inf(-1)
	for i, e := range a.Float64s {
		p := w.Positions[i]
		want := 4*(p[0]*p[0]-p[1]*p[1]) + 6*p[1]*p[2] - 1
		if math.Abs(e-want) > 1e-12 {
			t.Fatalf("elevation[%d] = %v, want %v", i, e, want)
		}
		lo, hi = min(lo, e), max(hi, e)
	}
	// The placeholder must put some samples below sea level and some
	// above, or images of it show nothing.
	if lo > -3 || hi < 2 {
		t.Fatalf("elevation spans %v to %v km; want both ocean and land", lo, hi)
	}
	if w.Attribute("no such attribute") != nil {
		t.Fatal("Attribute returned a value for an unknown name")
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

// Nothing in the generator is random yet, so the seed changes only the
// recorded Config. When a stage starts to draw random numbers this test
// fails: replace it then with one that requires the worlds to differ.
func TestSeedChangesNothingYet(t *testing.T) {
	cfg := smallConfig(1000)
	a := mustGenerate(t, cfg)
	cfg.Seed = 12345
	b := mustGenerate(t, cfg)
	if a.Config == b.Config {
		t.Fatal("the worlds record the same Config")
	}
	b.Config = a.Config
	if !reflect.DeepEqual(a, b) {
		t.Fatal("changing only the seed changed the world: update this test and the doc comment on Config.Seed")
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
