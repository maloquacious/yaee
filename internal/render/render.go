// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package render draws images of a world, for a person to judge its
// look. The images are derived from a yaee.World and are never an input
// to anything.
//
// The same world and width always give the same image, and WritePNG
// gives the same bytes for the same image.
package render

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/maloquacious/yaee"
	"github.com/maloquacious/yaee/internal/cerrs"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Errors returned by this package, wrapped with detail.
const (
	// ErrWidth: the image width is not an even number from MinWidth to
	// MaxWidth.
	ErrWidth = cerrs.Error("render: invalid image width")
	// ErrNoAttribute: the world lacks the attribute the image is drawn
	// from, or the attribute does not have the type, component count,
	// and length that Generate gives it.
	ErrNoAttribute = cerrs.Error("render: the world has no usable attribute")
	// ErrWorld: the world's samples and triangles are not a usable
	// triangulation of the sphere.
	ErrWorld = cerrs.Error("render: the world cannot be drawn")
)

// Limits on the width of an image, in pixels.
const (
	MinWidth = 2
	MaxWidth = 32768
)

// Elevation draws the world's elevation as an equirectangular image:
// width pixels wide and width/2 high. width must be even, from MinWidth
// to MaxWidth.
//
// Pixel (col, row), counted from the top left, has its centre at
//
//	longitude = −180° + (col + 0.5) · 360° / width
//	latitude  =   90° − (row + 0.5) · 180° / height
//
// so north is up, longitude 0 is the vertical line through the middle,
// and east is to the right. The pixel takes the colour, given by
// ElevationColour, of the elevation of the sample nearest to its centre.
// The image is not smoothed or shaded.
func Elevation(w *yaee.World, width int) (*image.NRGBA, error) {
	a := w.Attribute(yaee.AttrElevation)
	if a == nil || a.Type != yaee.Float64 || a.Components != 1 || len(a.Float64s) != w.NumSamples() {
		return nil, attributeError(w, yaee.AttrElevation, "float64")
	}
	grid, err := nearestSamples(w, width)
	if err != nil {
		return nil, err
	}
	return paint(grid, width, func(sample int32) color.NRGBA { return ElevationColour(a.Float64s[sample]) }), nil
}

// Plates draws the world's plates as an equirectangular image with the
// size and layout that Elevation documents. A pixel takes the colour,
// given by PlateColour, of the plate of the sample nearest to its
// centre, except that a pixel is black (0, 0, 0) where the pixel to its
// right or the pixel below it belongs to a different plate. The pixel to
// the right of the last column is the first column of the same row. The
// black pixels trace the plate boundaries.
func Plates(w *yaee.World, width int) (*image.NRGBA, error) {
	a := w.Attribute(yaee.AttrPlate)
	if a == nil || a.Type != yaee.Int32 || a.Components != 1 || len(a.Int32s) != w.NumSamples() {
		return nil, attributeError(w, yaee.AttrPlate, "int32")
	}
	grid, err := nearestSamples(w, width)
	if err != nil {
		return nil, err
	}
	plate := func(col, row int) int32 { return a.Int32s[grid[row*width+col]] }
	height := width / 2
	img := image.NewNRGBA(image.Rect(0, 0, width, height))
	for row := range height {
		for col := range width {
			p := plate(col, row)
			c := PlateColour(p)
			if plate((col+1)%width, row) != p || (row+1 < height && plate(col, row+1) != p) {
				c = color.NRGBA{A: 255}
			}
			img.SetNRGBA(col, row, c)
		}
	}
	return img, nil
}

// CrustType draws the world's crust type as an equirectangular image
// with the size and layout that Elevation documents. A pixel takes the
// colour of the crust type of the sample nearest to its centre: oceanic
// (40, 90, 160), a blue; continental (190, 170, 110), a tan; any other
// value (255, 0, 255), magenta.
func CrustType(w *yaee.World, width int) (*image.NRGBA, error) {
	a := w.Attribute(yaee.AttrCrustType)
	if a == nil || a.Type != yaee.Uint8 || a.Components != 1 || len(a.Uint8s) != w.NumSamples() {
		return nil, attributeError(w, yaee.AttrCrustType, "uint8")
	}
	grid, err := nearestSamples(w, width)
	if err != nil {
		return nil, err
	}
	return paint(grid, width, func(sample int32) color.NRGBA {
		switch a.Uint8s[sample] {
		case yaee.CrustOceanic:
			return color.NRGBA{R: 40, G: 90, B: 160, A: 255}
		case yaee.CrustContinental:
			return color.NRGBA{R: 190, G: 170, B: 110, A: 255}
		}
		return color.NRGBA{R: 255, G: 0, B: 255, A: 255}
	}), nil
}

// attributeError describes a missing or unusable attribute.
func attributeError(w *yaee.World, name, want string) error {
	a := w.Attribute(name)
	if a == nil {
		return fmt.Errorf("%w: no attribute named %q", ErrNoAttribute, name)
	}
	n := len(a.Uint8s) + len(a.Int32s) + len(a.Float32s) + len(a.Float64s)
	return fmt.Errorf("%w: %q is %s with %d components and %d values for %d samples; want %s with 1 component",
		ErrNoAttribute, name, a.Type, a.Components, n, w.NumSamples(), want)
}

// nearestSamples returns, for each pixel of an equirectangular image of
// the given width, row by row, the sample nearest to the pixel's centre.
func nearestSamples(w *yaee.World, width int) ([]int32, error) {
	if width < MinWidth || width > MaxWidth || width%2 != 0 {
		return nil, fmt.Errorf("%w: got %d; it must be even, from %d to %d", ErrWidth, width, MinWidth, MaxWidth)
	}
	locator, err := newLocator(w)
	if err != nil {
		return nil, err
	}
	height := width / 2
	grid := make([]int32, width*height)

	// The direction of each column's longitude is the same on every row.
	cosLon, sinLon := make([]float64, width), make([]float64, width)
	for col := range width {
		sinLon[col], cosLon[col] = math.Sincos(-math.Pi + (float64(col)+0.5)*2*math.Pi/float64(width))
	}

	// Each row is filled by one goroutine, from read-only data, so the
	// result does not depend on how the rows are shared out.
	var (
		nextRow atomic.Int64
		wg      sync.WaitGroup
	)
	for range min(runtime.GOMAXPROCS(0), height) {
		wg.Go(func() {
			for {
				row := int(nextRow.Add(1)) - 1
				if row >= height {
					return
				}
				sinLat, cosLat := math.Sincos(math.Pi/2 - (float64(row)+0.5)*math.Pi/float64(height))
				for col := range width {
					p := sphere.Vec{X: cosLat * cosLon[col], Y: cosLat * sinLon[col], Z: sinLat}
					grid[row*width+col] = locator.Nearest(p)
				}
			}
		})
	}
	wg.Wait()
	return grid, nil
}

// paint returns the image whose pixels are the colours of the samples
// in grid.
func paint(grid []int32, width int, colour func(sample int32) color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, width, len(grid)/width))
	for i, sample := range grid {
		c := colour(sample)
		img.Pix[4*i], img.Pix[4*i+1], img.Pix[4*i+2], img.Pix[4*i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

// newLocator builds the index that finds the sample nearest to a point,
// from the world's own samples and triangles.
func newLocator(w *yaee.World) (*sphere.Locator, error) {
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
	return sphere.NewLocator(mesh), nil
}

// The elevations at which the colour ramps end, in km.
const (
	DeepestOcean = -8.0
	HighestLand  = 6.0
)

// The colours at the ends of the two ramps, as red, green, blue.
var (
	shallowOcean = [3]float64{150, 200, 235}
	deepOcean    = [3]float64{8, 24, 80}
	lowLand      = [3]float64{200, 220, 150}
	highLand     = [3]float64{90, 50, 20}
)

// ElevationColour returns the colour of an elevation in km, with sea
// level at 0. Ocean is blue and land is green to brown; both darken
// away from sea level, so the coast is where light blue meets light
// green.
//
//   - Below 0: from (150, 200, 235) just below sea level, linearly to
//     (8, 24, 80) at DeepestOcean and below.
//   - 0 and above: from (200, 220, 150) at sea level, linearly to
//     (90, 50, 20) at HighestLand and above.
//   - NaN: (255, 0, 255), so that missing data stands out.
//
// Each channel is rounded to the nearest integer. Alpha is always 255.
func ElevationColour(km float64) color.NRGBA {
	var from, to [3]float64
	var t float64
	switch {
	case math.IsNaN(km):
		return color.NRGBA{R: 255, G: 0, B: 255, A: 255}
	case km < 0:
		from, to, t = shallowOcean, deepOcean, min(km/DeepestOcean, 1)
	default:
		from, to, t = lowLand, highLand, min(km/HighestLand, 1)
	}
	mix := func(i int) uint8 { return uint8(math.Round(from[i] + (to[i]-from[i])*t)) }
	return color.NRGBA{R: mix(0), G: mix(1), B: mix(2), A: 255}
}

// PlateColour returns the colour of a plate number. Plate k has hue
// k · 137.508° (the golden angle, so that plates close in number differ
// widely in hue), saturation 0.45, 0.6, or 0.75 as k mod 3 is 0, 1, or
// 2, and value 0.95, 0.8, or 0.65 as (k / 3) mod 3 is 0, 1, or 2, in the
// HSV colour model. Each channel is rounded to the nearest integer.
// Alpha is always 255. A negative number is magenta, (255, 0, 255).
//
// Colours repeat only approximately, and two plates far apart in number
// may look alike; the black boundaries drawn by Plates still separate
// them.
func PlateColour(plate int32) color.NRGBA {
	if plate < 0 {
		return color.NRGBA{R: 255, G: 0, B: 255, A: 255}
	}
	hue := math.Mod(float64(plate)*137.508, 360)
	sat := 0.45 + 0.15*float64(plate%3)
	val := 0.95 - 0.15*float64(plate/3%3)

	// HSV to RGB.
	c := val * sat
	h := hue / 60
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g, b = c, x, 0
	case 1:
		r, g, b = x, c, 0
	case 2:
		r, g, b = 0, c, x
	case 3:
		r, g, b = 0, x, c
	case 4:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	m := val - c
	to8 := func(v float64) uint8 { return uint8(math.Round((v + m) * 255)) }
	return color.NRGBA{R: to8(r), G: to8(g), B: to8(b), A: 255}
}

// WritePNG encodes the image as a PNG. The same image always gives the
// same bytes from the same build of the command.
func WritePNG(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}
