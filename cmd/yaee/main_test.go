// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/maloquacious/yaee"
)

// yaeeRun runs the command and returns its exit status and output.
func yaeeRun(t *testing.T, args ...string) (status int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	status = run(t.Context(), args, &out, &errOut)
	return status, out.String(), errOut.String()
}

// mustRun runs a command that must succeed silently.
func mustRun(t *testing.T, args ...string) {
	t.Helper()
	if status, stdout, stderr := yaeeRun(t, args...); status != 0 || stdout != "" || stderr != "" {
		t.Fatalf("yaee %s: status %d, stdout %q, stderr %q", strings.Join(args, " "), status, stdout, stderr)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestGenerateThenRender(t *testing.T) {
	dir := t.TempDir()
	world, image := filepath.Join(dir, "world.yaee"), filepath.Join(dir, "world.png")

	mustRun(t, "generate", "-seed", "7", "-spacing", "400", "-out", world)
	w, err := yaee.Read(bytes.NewReader(readFile(t, world)))
	if err != nil {
		t.Fatalf("the datafile does not read back: %v", err)
	}
	want := yaee.DefaultConfig()
	want.Seed, want.SampleSpacing = 7, 400
	if w.Config != want {
		t.Fatalf("the datafile records Config %+v, want %+v", w.Config, want)
	}

	// render needs nothing but the datafile.
	mustRun(t, "render", "--in="+world, "--out="+image, "--width=200")
	cfg, err := png.DecodeConfig(bytes.NewReader(readFile(t, image)))
	if err != nil || cfg.Width != 200 || cfg.Height != 100 {
		t.Fatalf("the image is %dx%d (%v), want 200x100", cfg.Width, cfg.Height, err)
	}

	// The same inputs give the same bytes.
	world2, image2 := filepath.Join(dir, "world2.yaee"), filepath.Join(dir, "world2.png")
	mustRun(t, "generate", "-seed", "7", "-spacing", "400", "-out", world2)
	mustRun(t, "render", "-in", world2, "-out", image2, "-width", "200")
	if !bytes.Equal(readFile(t, world), readFile(t, world2)) {
		t.Error("two runs of generate wrote different datafiles")
	}
	if !bytes.Equal(readFile(t, image), readFile(t, image2)) {
		t.Error("two runs of render wrote different images")
	}

	// Every kind of image, each different from the others.
	seen := map[string]string{}
	for name := range kinds {
		mustRun(t, "render", "-in", world, "-out", image, "-kind", name, "-width", "200")
		data := string(readFile(t, image))
		if cfg, err := png.DecodeConfig(strings.NewReader(data)); err != nil || cfg.Width != 200 || cfg.Height != 100 {
			t.Fatalf("the %s image is %dx%d (%v), want 200x100", name, cfg.Width, cfg.Height, err)
		}
		if other, dup := seen[data]; dup {
			t.Errorf("-kind %s and -kind %s wrote the same image", name, other)
		}
		seen[data] = name
	}
	mustRun(t, "render", "-in", world, "-out", image2, "-kind", "elevation", "-width", "200")
	mustRun(t, "render", "-in", world, "-out", image, "-width", "200")
	if !bytes.Equal(readFile(t, image), readFile(t, image2)) {
		t.Error("the default kind is not elevation")
	}

	// The plate flags reach the Config.
	mustRun(t, "generate", "-spacing", "800", "-plates", "5", "-plate-warp-amplitude", "0", "-plate-warp-feature-size", "900", "-out", world2)
	w2, err := yaee.Read(bytes.NewReader(readFile(t, world2)))
	if err != nil {
		t.Fatal(err)
	}
	want = yaee.DefaultConfig()
	want.SampleSpacing, want.Plates, want.PlateWarpAmplitude, want.PlateWarpFeatureSize = 800, 5, 0, 900
	if w2.Config != want {
		t.Fatalf("the datafile records Config %+v, want %+v", w2.Config, want)
	}

	// The crust flags reach the Config.
	mustRun(t, "generate", "-spacing", "800", "-land-coverage", "0.6", "-ridge-elevation", "-0.5", "-abyssal-elevation", "-4",
		"-trench-elevation", "-8", "-max-continental-elevation", "9", "-max-plate-speed", "60", "-oceanic-dampening", "0.02", "-out", world2)
	w2, err = yaee.Read(bytes.NewReader(readFile(t, world2)))
	if err != nil {
		t.Fatal(err)
	}
	want = yaee.DefaultConfig()
	want.SampleSpacing, want.LandCoverage = 800, 0.6
	want.RidgeElevation, want.AbyssalElevation, want.TrenchElevation, want.MaxContinentalElevation = -0.5, -4, -8, 9
	want.MaxPlateSpeed, want.OceanicDampening = 60, 0.02
	if w2.Config != want {
		t.Fatalf("the datafile records Config %+v, want %+v", w2.Config, want)
	}

	// The default width.
	mustRun(t, "render", "-in", world, "-out", image)
	if cfg, err := png.DecodeConfig(bytes.NewReader(readFile(t, image))); err != nil || cfg.Width != 2048 || cfg.Height != 1024 {
		t.Fatalf("the default image is %dx%d (%v), want 2048x1024", cfg.Width, cfg.Height, err)
	}
}

func TestVerboseGoesToStderr(t *testing.T) {
	dir := t.TempDir()
	world := filepath.Join(dir, "w.yaee")
	status, stdout, stderr := yaeeRun(t, "generate", "-spacing", "1000", "-out", world, "-v")
	if status != 0 || stdout != "" || !strings.Contains(stderr, "589 samples, 1174 triangles") {
		t.Fatalf("status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	status, stdout, stderr = yaeeRun(t, "render", "-in", world, "-out", filepath.Join(dir, "w.png"), "-width", "64", "-v")
	if status != 0 || stdout != "" || !strings.Contains(stderr, "64x32 pixels") {
		t.Fatalf("status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

func TestConfigFileAndWriteConfig(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in.json"), filepath.Join(dir, "out.json")

	// The effective Config, with no file and no world generated.
	status, stdout, stderr := yaeeRun(t, "generate", "-write-config", "-")
	if status != 0 || stderr != "" {
		t.Fatalf("status %d, stderr %q", status, stderr)
	}
	var cfg yaee.Config
	if err := json.Unmarshal([]byte(stdout), &cfg); err != nil || cfg != yaee.DefaultConfig() {
		t.Fatalf("-write-config - printed %q (%v), want the default Config", stdout, err)
	}

	// File members override defaults; flags override the file.
	if err := os.WriteFile(in, []byte(`{"seed": 18446744073709551615, "sample_spacing_km": 800, "duration_my": 10}`), 0o644); err != nil {
		t.Fatal(err)
	}
	mustRun(t, "generate", "-config", in, "-duration", "0", "-write-config", out)
	if err := json.Unmarshal(readFile(t, out), &cfg); err != nil {
		t.Fatal(err)
	}
	want := yaee.DefaultConfig()
	want.Seed, want.SampleSpacing, want.Duration = 18446744073709551615, 800, 0
	if cfg != want {
		t.Fatalf("effective Config %+v, want %+v", cfg, want)
	}

	// A written Config can be loaded again, and gives the same world as
	// the flags do.
	a, b := filepath.Join(dir, "a.yaee"), filepath.Join(dir, "b.yaee")
	mustRun(t, "generate", "-config", out, "-out", a)
	mustRun(t, "generate", "-seed", "18446744073709551615", "-spacing", "800", "-duration", "0", "-out", b)
	if !bytes.Equal(readFile(t, a), readFile(t, b)) {
		t.Fatal("the Config file and the equivalent flags gave different datafiles")
	}
}

func TestVersion(t *testing.T) {
	status, stdout, stderr := yaeeRun(t, "version")
	if status != 0 || stdout != yaee.Version().String()+"\n" || stderr != "" {
		t.Fatalf("status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
}

// Every failure exits non-zero, prints nothing on standard output, and
// names the problem on standard error.
func TestFailures(t *testing.T) {
	dir := t.TempDir()
	world := filepath.Join(dir, "world.yaee")
	mustRun(t, "generate", "-spacing", "1000", "-out", world)
	missing := filepath.Join(dir, "no-such-dir", "file")
	notDatafile := filepath.Join(dir, "text.yaee")
	truncated := filepath.Join(dir, "short.yaee")
	badJSON := filepath.Join(dir, "bad.json")
	unknownMember := filepath.Join(dir, "unknown.json")
	twoObjects := filepath.Join(dir, "two.json")
	for path, content := range map[string]string{
		notDatafile:   "this is not a datafile",
		truncated:     string(readFile(t, world)[:300]),
		badJSON:       `{"seed": `,
		unknownMember: `{"seed": 1, "moons": 2}`,
		twoObjects:    `{"seed": 1} {"seed": 2}`,
	} {
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name   string
		args   []string
		status int
		stderr []string // each must appear
	}{
		{"no command", nil, 2, []string{"no command given", "Usage:"}},
		{"unknown command", []string{"frobnicate"}, 2, []string{`unknown command "frobnicate"`}},
		{"help for unknown command", []string{"help", "frobnicate"}, 2, []string{`unknown command "frobnicate"`}},
		{"unknown flag", []string{"generate", "-moons", "2"}, 2, []string{"flag provided but not defined: -moons"}},
		{"unknown kind", []string{"render", "-in", world, "-out", filepath.Join(dir, "x.png"), "-kind", "rivers"}, 2,
			[]string{`unknown -kind "rivers"`, "age, crust, elevation, plates"}},
		{"malformed flag value", []string{"generate", "-radius", "big", "-out", world}, 2, []string{`invalid value "big" for flag -radius`}},
		{"negative seed", []string{"generate", "-seed", "-1", "-out", world}, 2, []string{"for flag -seed"}},
		{"unexpected argument", []string{"generate", "-out", world, "extra"}, 2, []string{`unexpected argument "extra"`}},
		{"generate without outputs", []string{"generate"}, 2, []string{"at least one of -out and -write-config is required"}},
		{"render without -in", []string{"render", "-out", filepath.Join(dir, "x.png")}, 2, []string{"-in and -out are both required"}},
		{"render without -out", []string{"render", "-in", world}, 2, []string{"-in and -out are both required"}},
		{"version with argument", []string{"version", "now"}, 2, []string{`unexpected argument "now"`}},

		{"invalid Config", []string{"generate", "-radius", "-5", "-time-step", "0", "-out", filepath.Join(dir, "bad.yaee")}, 1,
			[]string{"invalid config: Radius:", "invalid config: TimeStep:"}},
		{"bad land coverage", []string{"generate", "-land-coverage", "1.5", "-out", filepath.Join(dir, "bad.yaee")}, 1, []string{"invalid config: LandCoverage:"}},
		{"too many plates", []string{"generate", "-spacing", "5000", "-plates", "30", "-out", filepath.Join(dir, "bad.yaee")}, 1, []string{"invalid config: Plates:"}},
		{"too many samples", []string{"generate", "-spacing", "1", "-out", filepath.Join(dir, "bad.yaee")}, 1, []string{"invalid config: SampleSpacing:"}},
		{"unreadable Config", []string{"generate", "-config", missing, "-out", world}, 1, []string{missing}},
		{"malformed Config", []string{"generate", "-config", badJSON, "-out", world}, 1, []string{badJSON}},
		{"unknown Config member", []string{"generate", "-config", unknownMember, "-out", world}, 1, []string{unknownMember, `unknown field "moons"`}},
		{"data after Config", []string{"generate", "-config", twoObjects, "-out", world}, 1, []string{twoObjects, "unexpected data"}},
		{"unwritable datafile", []string{"generate", "-spacing", "1000", "-out", missing}, 1, []string{missing}},
		{"unwritable Config", []string{"generate", "-write-config", missing}, 1, []string{missing}},
		{"unreadable datafile", []string{"render", "-in", missing, "-out", filepath.Join(dir, "x.png")}, 1, []string{missing}},
		{"not a datafile", []string{"render", "-in", notDatafile, "-out", filepath.Join(dir, "x.png")}, 1, []string{notDatafile, "not a datafile"}},
		{"truncated datafile", []string{"render", "-in", truncated, "-out", filepath.Join(dir, "x.png")}, 1, []string{truncated, "truncated datafile"}},
		{"unwritable image", []string{"render", "-in", world, "-out", missing, "-width", "64"}, 1, []string{missing}},
		{"odd width", []string{"render", "-in", world, "-out", filepath.Join(dir, "x.png"), "-width", "65"}, 1, []string{"invalid image width", "65"}},
	}
	for _, tt := range tests {
		status, stdout, stderr := yaeeRun(t, tt.args...)
		if status != tt.status {
			t.Errorf("%s: status %d, want %d (stderr %q)", tt.name, status, tt.status, stderr)
		}
		if stdout != "" {
			t.Errorf("%s: wrote %q to standard output", tt.name, stdout)
		}
		for _, want := range tt.stderr {
			if !strings.Contains(stderr, want) {
				t.Errorf("%s: standard error %q does not contain %q", tt.name, stderr, want)
			}
		}
	}

	// A failed command leaves no file behind.
	for _, name := range []string{"bad.yaee", "x.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			t.Errorf("a failed command left %s behind", name)
		}
	}
}

func TestInterrupted(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var out, errOut bytes.Buffer
	path := filepath.Join(t.TempDir(), "w.yaee")
	if status := run(ctx, []string{"generate", "-spacing", "1000", "-out", path}, &out, &errOut); status != 1 {
		t.Fatalf("status %d, want 1 (stderr %q)", status, errOut.String())
	}
	if !strings.Contains(errOut.String(), "context canceled") {
		t.Fatalf("standard error %q does not report the cancellation", errOut.String())
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("an interrupted generate wrote a datafile")
	}
}

// The top-level help names every command and exit status, and each
// command's help lists every one of its flags with its default.
func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"-help"}, {"--help"}, {"help", "help"}} {
		status, stdout, stderr := yaeeRun(t, args...)
		if status != 0 || stderr != "" || stdout != mainHelp {
			t.Errorf("yaee %v: status %d, stderr %q, or the wrong text", args, status, stderr)
		}
	}
	for _, c := range commands() {
		if !strings.Contains(mainHelp, "\n  "+c.name+" ") {
			t.Errorf("the top-level help does not list %q", c.name)
		}
	}
	for _, want := range []string{"Exit status:", "\n  0  ", "\n  1  ", "\n  2  "} {
		if !strings.Contains(mainHelp, want) {
			t.Errorf("the top-level help lacks %q", want)
		}
	}

	for _, c := range commands() {
		status, viaHelp, stderr := yaeeRun(t, "help", c.name)
		if status != 0 || stderr != "" {
			t.Fatalf("yaee help %s: status %d, stderr %q", c.name, status, stderr)
		}
		for _, flagForm := range []string{"-h", "-help", "--help"} {
			status, stdout, stderr := yaeeRun(t, c.name, flagForm)
			if status != 0 || stderr != "" || stdout != viaHelp {
				t.Errorf("yaee %s %s: status %d, stderr %q, or text unlike yaee help %s", c.name, flagForm, status, stderr, c.name)
			}
		}
		c.flags.VisitAll(func(f *flag.Flag) {
			def := f.DefValue
			if def == "" {
				def = "none"
			}
			i := strings.Index(viaHelp, "\n  -"+f.Name+" ")
			if i < 0 {
				t.Errorf("the help of %s does not list -%s", c.name, f.Name)
				return
			}
			entry, _, _ := strings.Cut(viaHelp[i+1:], "\n  -")
			if !strings.Contains(entry+"\n", "Default: "+def+"\n") {
				t.Errorf("the help of %s does not give the default of -%s as %s", c.name, f.Name, def)
			}
		})
	}

	// The defaults shown for generate are the default Config.
	_, help, _ := yaeeRun(t, "help", "generate")
	data, _ := json.MarshalIndent(yaee.DefaultConfig(), "  ", "  ")
	if !strings.Contains(help, "\n  "+string(data)+"\n") {
		t.Errorf("the help of generate does not show the default Config %s", data)
	}
}
