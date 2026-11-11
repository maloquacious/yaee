// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package render

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"math"
	"runtime"
	"testing"

	"github.com/maloquacious/yaee"
)

// world returns a world of about n samples whose elevation is f of the
// sample's position.
func world(t testing.TB, n int, f func(x, y, z float64) float64) *yaee.World {
	t.Helper()
	cfg := yaee.DefaultConfig()
	cfg.SampleSpacing = math.Sqrt(8 * math.Pi * cfg.Radius * cfg.Radius / (math.Sqrt(3) * float64(n)))
	w, err := yaee.Generate(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	elev := w.Attribute(yaee.AttrElevation).Float64s
	for i, p := range w.Positions {
		elev[i] = f(p[0], p[1], p[2])
	}
	return w
}

// pixelCentre returns the unit vector at the centre of a pixel.
func pixelCentre(col, row, width int) (x, y, z float64) {
	lon := (-180 + (float64(col)+0.5)*360/float64(width)) * math.Pi / 180
	lat := (90 - (float64(row)+0.5)*180/float64(width/2)) * math.Pi / 180
	return math.Cos(lat) * math.Cos(lon), math.Cos(lat) * math.Sin(lon), math.Sin(lat)
}

func TestElevationColour(t *testing.T) {
	tests := []struct {
		km   float64
		want color.NRGBA
	}{
		{0, color.NRGBA{200, 220, 150, 255}},
		{math.Copysign(0, -1), color.NRGBA{200, 220, 150, 255}},
		{3, color.NRGBA{145, 135, 85, 255}},
		{6, color.NRGBA{90, 50, 20, 255}},
		{9, color.NRGBA{90, 50, 20, 255}},
		{math.Inf(1), color.NRGBA{90, 50, 20, 255}},
		{-1e-9, color.NRGBA{150, 200, 235, 255}},
		{-4, color.NRGBA{79, 112, 158, 255}},
		{-8, color.NRGBA{8, 24, 80, 255}},
		{-11, color.NRGBA{8, 24, 80, 255}},
		{math.Inf(-1), color.NRGBA{8, 24, 80, 255}},
		{math.NaN(), color.NRGBA{255, 0, 255, 255}},
	}
	for _, tt := range tests {
		if got := ElevationColour(tt.km); got != tt.want {
			t.Errorf("ElevationColour(%v) = %v, want %v", tt.km, got, tt.want)
		}
	}
	// Both ramps darken away from sea level.
	for km := 0.0; km < 6; km += 0.25 {
		if a, b := ElevationColour(km), ElevationColour(km+0.25); b.R > a.R || b.G > a.G || b.B > a.B {
			t.Errorf("land does not darken from %v to %v km", km, km+0.25)
		}
		d := -km * 8 / 6
		if a, b := ElevationColour(d-1e-9), ElevationColour(d-1e-9-1.0/3); b.R > a.R || b.G > a.G || b.B > a.B {
			t.Errorf("ocean does not darken below %v km", d)
		}
	}
}

func TestImageSize(t *testing.T) {
	w := world(t, 200, func(x, y, z float64) float64 { return z })
	for _, width := range []int{2, 4, 64, 250} {
		img, err := Elevation(w, width)
		if err != nil {
			t.Fatalf("width %d: %v", width, err)
		}
		if b := img.Bounds(); b.Min.X != 0 || b.Min.Y != 0 || b.Dx() != width || b.Dy() != width/2 {
			t.Errorf("width %d: bounds %v, want %dx%d", width, b, width, width/2)
		}
	}
	for _, width := range []int{-2, 0, 1, 3, 255, MaxWidth + 2} {
		if _, err := Elevation(w, width); !errors.Is(err, ErrWidth) {
			t.Errorf("width %d: got %v, want ErrWidth", width, err)
		}
	}
}

// Every pixel has exactly the colour of the sample nearest its centre,
// found here by comparing with every sample.
func TestPixelsTakeNearestSample(t *testing.T) {
	w := world(t, 300, func(x, y, z float64) float64 { return 9*x - 2 })
	elev := w.Attribute(yaee.AttrElevation).Float64s
	const width = 64
	img, err := Elevation(w, width)
	if err != nil {
		t.Fatal(err)
	}
	for row := range width / 2 {
		for col := range width {
			x, y, z := pixelCentre(col, row, width)
			best, bestDot := 0, math.Inf(-1)
			for i, p := range w.Positions {
				if d := p[0]*x + p[1]*y + p[2]*z; d > bestDot {
					best, bestDot = i, d
				}
			}
			if got, want := img.NRGBAAt(col, row), ElevationColour(elev[best]); got != want {
				t.Fatalf("pixel (%d, %d): colour %v, want %v, that of sample %d", col, row, got, want, best)
			}
		}
	}
}

// For an elevation that is a known function of position, a pixel has the
// colour of the function's value at the pixel's latitude and longitude,
// to within the change in the function over half a sample spacing.
func TestPixelsFollowKnownFunction(t *testing.T) {
	f := func(x, y, z float64) float64 { return 5*z + 3*x*y }
	w := world(t, 40000, f)
	const width = 360
	img, err := Elevation(w, width)
	if err != nil {
		t.Fatal(err)
	}
	near := func(a, b uint8) bool { return a-b <= 3 || b-a <= 3 }
	for row := 0; row < width/2; row += 7 {
		for col := 0; col < width; col += 11 {
			x, y, z := pixelCentre(col, row, width)
			v := f(x, y, z)
			if math.Abs(v) < 0.15 {
				continue // too close to the coast, where the colour jumps
			}
			got, want := img.NRGBAAt(col, row), ElevationColour(v)
			if !near(got.R, want.R) || !near(got.G, want.G) || !near(got.B, want.B) {
				t.Errorf("pixel (%d, %d), elevation %.3f km: colour %v, want about %v", col, row, v, got, want)
			}
		}
	}

	// Orientation: north is up, longitude 0 is in the middle, and east
	// is to the right. 5z + 3xy is highest at the north pole, and at
	// latitude 0 it is zero on the meridians 0 and ±90°.
	top, bottom := img.NRGBAAt(width/2, 0), img.NRGBAAt(width/2, width/2-1)
	if top.B >= top.G || bottom.B <= bottom.G {
		t.Errorf("top is %v and bottom %v; want land at the top and ocean at the bottom", top, bottom)
	}
	g := func(x, y, z float64) float64 { return 4 * y } // highest at 90° east
	img, err = Elevation(world(t, 2000, g), width)
	if err != nil {
		t.Fatal(err)
	}
	east, west := img.NRGBAAt(3*width/4, width/4), img.NRGBAAt(width/4, width/4)
	if east.B >= east.G || west.B <= west.G {
		t.Errorf("at the equator, 90°E is %v and 90°W is %v; want land in the east and ocean in the west", east, west)
	}
}

func TestSameWorldSameBytes(t *testing.T) {
	w := world(t, 5000, func(x, y, z float64) float64 { return 4*(x*x-y*y) + 6*y*z - 1 })
	encode := func() []byte {
		img, err := Elevation(w, 256)
		if err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		if err := WritePNG(&buf, img); err != nil {
			t.Fatal(err)
		}
		return buf.Bytes()
	}
	many := encode()
	if !bytes.Equal(many, encode()) {
		t.Fatal("rendering the same world twice gave different bytes")
	}
	prev := runtime.GOMAXPROCS(1)
	one := encode()
	runtime.GOMAXPROCS(prev)
	if !bytes.Equal(many, one) {
		t.Fatal("the image differs between 1 processor and many")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(many))
	if err != nil || cfg.Width != 256 || cfg.Height != 128 {
		t.Fatalf("the PNG decodes as %dx%d (%v), want 256x128", cfg.Width, cfg.Height, err)
	}
}

func TestUnusableWorlds(t *testing.T) {
	flat := func(x, y, z float64) float64 { return 1 }

	w := world(t, 100, flat)
	w.Attributes = nil
	if _, err := Elevation(w, 16); !errors.Is(err, ErrNoElevation) {
		t.Errorf("no attributes: got %v, want ErrNoElevation", err)
	}

	w = world(t, 100, flat)
	w.Attributes[0] = yaee.Attribute{Name: yaee.AttrElevation, Unit: "km", Type: yaee.Float32, Components: 1, Float32s: make([]float32, 100)}
	if _, err := Elevation(w, 16); !errors.Is(err, ErrNoElevation) {
		t.Errorf("float32 elevation: got %v, want ErrNoElevation", err)
	}

	w = world(t, 100, flat)
	w.Triangles = w.Triangles[1:]
	if _, err := Elevation(w, 16); !errors.Is(err, ErrWorld) {
		t.Errorf("a triangle missing: got %v, want ErrWorld", err)
	}

	w = world(t, 100, flat)
	w.Positions[7] = [3]float64{2, 0, 0}
	if _, err := Elevation(w, 16); !errors.Is(err, ErrWorld) {
		t.Errorf("a position off the sphere: got %v, want ErrWorld", err)
	}
}
