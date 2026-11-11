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
	cfg.Plates = min(cfg.Plates, n/10)
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
	if _, err := Elevation(w, 16); !errors.Is(err, ErrNoAttribute) {
		t.Errorf("no attributes: got %v, want ErrNoAttribute", err)
	}

	w = world(t, 100, flat)
	*w.Attribute(yaee.AttrElevation) = yaee.Attribute{Name: yaee.AttrElevation, Unit: "km", Type: yaee.Float32, Components: 1, Float32s: make([]float32, 100)}
	if _, err := Elevation(w, 16); !errors.Is(err, ErrNoAttribute) {
		t.Errorf("float32 elevation: got %v, want ErrNoAttribute", err)
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

func TestPlateColour(t *testing.T) {
	tests := []struct {
		plate int32
		want  color.NRGBA
	}{
		{0, color.NRGBA{242, 133, 133, 255}}, // hue 0, saturation 0.45, value 0.95
		{1, color.NRGBA{97, 242, 139, 255}},  // hue 137.508, saturation 0.6
		{3, color.NRGBA{204, 193, 112, 255}}, // hue 52.524, value 0.8
		{-1, color.NRGBA{255, 0, 255, 255}},
	}
	for _, tt := range tests {
		if got := PlateColour(tt.plate); got != tt.want {
			t.Errorf("PlateColour(%d) = %v, want %v", tt.plate, got, tt.want)
		}
	}
	// The first 40 plates have 40 colours, none of them black, the
	// colour of a boundary.
	seen := map[color.NRGBA]int32{}
	for k := range int32(1000) {
		c := PlateColour(k)
		if c.A != 255 || int(c.R)+int(c.G)+int(c.B) < 150 {
			t.Fatalf("PlateColour(%d) = %v is too dark to tell from a boundary", k, c)
		}
		if other, dup := seen[c]; dup && k < 40 {
			t.Errorf("plates %d and %d share the colour %v", other, k, c)
		}
		seen[c] = k
	}
}

func TestPlates(t *testing.T) {
	const width = 128
	w := world(t, 4000, func(x, y, z float64) float64 { return 0 })
	// Three plates split by longitude, so every boundary is a meridian.
	plate := w.Attribute(yaee.AttrPlate).Int32s
	for i, p := range w.Positions {
		lon := math.Atan2(p[1], p[0]) * 180 / math.Pi
		switch {
		case lon < -60:
			plate[i] = 0
		case lon < 60:
			plate[i] = 1
		default:
			plate[i] = 2
		}
	}
	img, err := Plates(w, width)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != width || b.Dy() != width/2 {
		t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), width, width/2)
	}
	black := color.NRGBA{A: 255}
	for row := 8; row < width/2-8; row++ { // away from the poles, where the meridians meet
		var boundaries int
		for col := range width {
			c := img.NRGBAAt(col, row)
			if c == black {
				boundaries++
				continue
			}
			lon := -180 + (float64(col)+0.5)*360/width
			want := int32(2)
			switch {
			case lon < -60:
				want = 0
			case lon < 60:
				want = 1
			}
			// Within a sample spacing of a boundary the nearest sample
			// may be on either side.
			if near := min(math.Abs(lon+60), math.Abs(lon-60), 180-math.Abs(lon)); near > 8 && c != PlateColour(want) {
				t.Fatalf("pixel (%d, %d) at longitude %v is %v, want plate %d's %v", col, row, lon, c, want, PlateColour(want))
			}
		}
		// Three boundaries cross each row, one of them at the edge of
		// the image; each is a pixel or two wide.
		if boundaries < 3 || boundaries > 9 {
			t.Fatalf("row %d has %d boundary pixels, want 3 to 9", row, boundaries)
		}
	}

	// Errors.
	if _, err := Plates(w, 7); !errors.Is(err, ErrWidth) {
		t.Errorf("odd width: got %v, want ErrWidth", err)
	}
	w.Attribute(yaee.AttrPlate).Name = "renamed"
	if _, err := Plates(w, 16); !errors.Is(err, ErrNoAttribute) {
		t.Errorf("no plate attribute: got %v, want ErrNoAttribute", err)
	}
}

func TestCrustType(t *testing.T) {
	const width = 128
	w := world(t, 4000, func(x, y, z float64) float64 { return 0 })
	// Land north of the equator, ocean south of it, and one sample
	// with a value that is not a crust type.
	crust := w.Attribute(yaee.AttrCrustType).Uint8s
	for i, p := range w.Positions {
		crust[i] = yaee.CrustOceanic
		if p[2] > 0 {
			crust[i] = yaee.CrustContinental
		}
	}
	img, err := CrustType(w, width)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != width || b.Dy() != width/2 {
		t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), width, width/2)
	}
	land, ocean := color.NRGBA{190, 170, 110, 255}, color.NRGBA{40, 90, 160, 255}
	for row := range width / 2 {
		for col := range width {
			_, _, z := pixelCentre(col, row, width)
			want := ocean
			if z > 0 {
				want = land
			}
			// Within a sample spacing of the equator the nearest
			// sample may be on either side.
			if got := img.NRGBAAt(col, row); math.Abs(z) > 0.1 && got != want {
				t.Fatalf("pixel (%d, %d) at z = %v is %v, want %v", col, row, z, got, want)
			}
		}
	}

	for i := range crust {
		crust[i] = 7
	}
	img, err = CrustType(w, 16)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := img.NRGBAAt(3, 3), (color.NRGBA{255, 0, 255, 255}); got != want {
		t.Errorf("an unknown crust type is drawn %v, want %v", got, want)
	}

	// A world of generated crust draws both colours.
	img, err = CrustType(world(t, 4000, func(x, y, z float64) float64 { return 0 }), width)
	if err != nil {
		t.Fatal(err)
	}
	count := map[color.NRGBA]int{}
	for row := range width / 2 {
		for col := range width {
			count[img.NRGBAAt(col, row)]++
		}
	}
	if len(count) != 2 || count[land] == 0 || count[ocean] == 0 {
		t.Errorf("generated crust drew colours %v, want only land and ocean", count)
	}

	if _, err := CrustType(w, 15); !errors.Is(err, ErrWidth) {
		t.Errorf("odd width: got %v, want ErrWidth", err)
	}
	w.Attributes = w.Attributes[:1] // the plate attribute only
	if _, err := CrustType(w, 16); !errors.Is(err, ErrNoAttribute) {
		t.Errorf("no crust_type attribute: got %v, want ErrNoAttribute", err)
	}
}

func TestAgeColour(t *testing.T) {
	for _, c := range []struct {
		my   float64
		want color.NRGBA
	}{
		{0, color.NRGBA{215, 48, 39, 255}},
		{62.5, color.NRGBA{254, 224, 139, 255}},
		{125, color.NRGBA{120, 198, 121, 255}},
		{187.5, color.NRGBA{49, 130, 189, 255}},
		{250, color.NRGBA{48, 24, 96, 255}},
		{1e9, color.NRGBA{48, 24, 96, 255}},
		{math.Inf(1), color.NRGBA{48, 24, 96, 255}},
		{31.25, color.NRGBA{235, 136, 89, 255}}, // halfway from red to yellow
		{-1, color.NRGBA{255, 0, 255, 255}},
		{math.NaN(), color.NRGBA{255, 0, 255, 255}},
	} {
		if got := AgeColour(c.my); got != c.want {
			t.Errorf("AgeColour(%v) = %v, want %v", c.my, got, c.want)
		}
	}
}

func TestOceanicAge(t *testing.T) {
	const width = 128
	w := world(t, 4000, func(x, y, z float64) float64 { return 0 })
	// Land north of 30° north; south of it ocean whose age grows with
	// the distance south, from 0 to 250 My at the south pole.
	crust := w.Attribute(yaee.AttrCrustType).Uint8s
	age := w.Attribute(yaee.AttrOceanicAge).Float64s
	ageAt := func(z float64) float64 { return (0.5 - z) / 1.5 * 250 }
	for i, p := range w.Positions {
		crust[i], age[i] = yaee.CrustOceanic, ageAt(p[2])
		if p[2] > 0.5 {
			crust[i], age[i] = yaee.CrustContinental, 0
		}
	}
	img, err := OceanicAge(w, width)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != width || b.Dy() != width/2 {
		t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), width, width/2)
	}
	near := func(a, b color.NRGBA) bool {
		d := func(x, y uint8) int { return max(int(x), int(y)) - min(int(x), int(y)) }
		return d(a.R, b.R) <= 30 && d(a.G, b.G) <= 30 && d(a.B, b.B) <= 30
	}
	for row := range width / 2 {
		for col := range width {
			_, _, z := pixelCentre(col, row, width)
			got := img.NRGBAAt(col, row)
			switch {
			case math.Abs(z-0.5) < 0.1: // either side of the coast
			case z > 0.5:
				if got != (color.NRGBA{200, 200, 200, 255}) {
					t.Fatalf("pixel (%d, %d) on land is %v, want grey", col, row, got)
				}
			default:
				// The nearest sample is within a spacing of the pixel, which
				// is 10 My of age and up to 30 in a colour channel.
				if want := AgeColour(ageAt(z)); !near(got, want) {
					t.Fatalf("pixel (%d, %d) at z = %v is %v, want about %v", col, row, z, got, want)
				}
			}
		}
	}

	for i := range crust {
		crust[i] = 7
	}
	img, err = OceanicAge(w, 16)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := img.NRGBAAt(3, 3), (color.NRGBA{255, 0, 255, 255}); got != want {
		t.Errorf("an unknown crust type is drawn %v, want %v", got, want)
	}

	if _, err := OceanicAge(w, 15); !errors.Is(err, ErrWidth) {
		t.Errorf("odd width: got %v, want ErrWidth", err)
	}
	w.Attribute(yaee.AttrOceanicAge).Name = "renamed"
	if _, err := OceanicAge(w, 16); !errors.Is(err, ErrNoAttribute) {
		t.Errorf("no oceanic_age attribute: got %v, want ErrNoAttribute", err)
	}
	w.Attributes = w.Attributes[:1] // the plate attribute only
	if _, err := OceanicAge(w, 16); !errors.Is(err, ErrNoAttribute) {
		t.Errorf("no crust_type attribute: got %v, want ErrNoAttribute", err)
	}
}

func TestOrogenyColour(t *testing.T) {
	magenta := color.NRGBA{255, 0, 255, 255}
	for _, c := range []struct {
		crust, kind uint8
		my          float64
		want        color.NRGBA
	}{
		{yaee.CrustOceanic, yaee.OrogenyNone, 0, color.NRGBA{200, 215, 230, 255}},
		{yaee.CrustContinental, yaee.OrogenyNone, 0, color.NRGBA{225, 225, 225, 255}},
		{yaee.CrustContinental, yaee.OrogenyNone, math.NaN(), color.NRGBA{225, 225, 225, 255}},
		{7, yaee.OrogenyNone, 0, magenta},
		{yaee.CrustContinental, yaee.OrogenyAndean, 0, color.NRGBA{165, 15, 21, 255}},
		{yaee.CrustOceanic, yaee.OrogenyAndean, 0, color.NRGBA{165, 15, 21, 255}},
		{yaee.CrustContinental, yaee.OrogenyAndean, 125, color.NRGBA{209, 101, 91, 255}},
		{yaee.CrustContinental, yaee.OrogenyAndean, 250, color.NRGBA{252, 187, 161, 255}},
		{yaee.CrustContinental, yaee.OrogenyAndean, math.Inf(1), color.NRGBA{252, 187, 161, 255}},
		{yaee.CrustContinental, yaee.OrogenyHimalayan, 0, color.NRGBA{63, 0, 125, 255}},
		{yaee.CrustContinental, yaee.OrogenyHimalayan, 125, color.NRGBA{126, 95, 173, 255}},
		{yaee.CrustContinental, yaee.OrogenyHimalayan, 1e9, color.NRGBA{188, 189, 220, 255}},
		{yaee.CrustContinental, yaee.OrogenyAndean, -1, magenta},
		{yaee.CrustContinental, yaee.OrogenyHimalayan, math.NaN(), magenta},
		{yaee.CrustContinental, 3, 10, magenta},
	} {
		if got := OrogenyColour(c.crust, c.kind, c.my); got != c.want {
			t.Errorf("OrogenyColour(%d, %d, %v) = %v, want %v", c.crust, c.kind, c.my, got, c.want)
		}
	}
}

func TestOrogeny(t *testing.T) {
	const width = 128
	w := world(t, 4000, func(x, y, z float64) float64 { return 0 })
	// Ocean south of the equator; north of it land, with an Andean
	// orogeny of 40 My north of 30° north and a Himalayan one of 90 My
	// north of 60° north.
	crust := w.Attribute(yaee.AttrCrustType).Uint8s
	kind := w.Attribute(yaee.AttrOrogenyType).Uint8s
	age := w.Attribute(yaee.AttrOrogenyAge).Float64s
	at := func(z float64) (uint8, uint8, float64) {
		switch {
		case z > 0.866:
			return yaee.CrustContinental, yaee.OrogenyHimalayan, 90
		case z > 0.5:
			return yaee.CrustContinental, yaee.OrogenyAndean, 40
		case z > 0:
			return yaee.CrustContinental, yaee.OrogenyNone, 0
		}
		return yaee.CrustOceanic, yaee.OrogenyNone, 0
	}
	for i, p := range w.Positions {
		crust[i], kind[i], age[i] = at(p[2])
	}
	img, err := Orogeny(w, width)
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != width || b.Dy() != width/2 {
		t.Fatalf("image is %dx%d, want %dx%d", b.Dx(), b.Dy(), width, width/2)
	}
	seen := map[color.NRGBA]int{}
	for row := range width / 2 {
		for col := range width {
			_, _, z := pixelCentre(col, row, width)
			if math.Abs(z) < 0.1 || math.Abs(z-0.5) < 0.1 || math.Abs(z-0.866) < 0.1 {
				continue // either side of a border
			}
			got, want := img.NRGBAAt(col, row), OrogenyColour(at(z))
			if got != want {
				t.Fatalf("pixel (%d, %d) at z = %v is %v, want %v", col, row, z, got, want)
			}
			seen[got]++
		}
	}
	if len(seen) != 4 {
		t.Fatalf("%d colours in the image, want 4: %v", len(seen), seen)
	}

	if _, err := Orogeny(w, 15); !errors.Is(err, ErrWidth) {
		t.Errorf("odd width: got %v, want ErrWidth", err)
	}
	for _, name := range []string{yaee.AttrOrogenyAge, yaee.AttrOrogenyType, yaee.AttrCrustType} {
		w.Attribute(name).Name = "renamed"
		if _, err := Orogeny(w, 16); !errors.Is(err, ErrNoAttribute) {
			t.Errorf("no %s attribute: got %v, want ErrNoAttribute", name, err)
		}
	}
}
