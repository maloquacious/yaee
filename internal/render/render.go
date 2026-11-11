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
	// ErrNoElevation: the world has no elevation attribute of type
	// float64 with one component.
	ErrNoElevation = cerrs.Error("render: the world has no usable elevation attribute")
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
	if width < MinWidth || width > MaxWidth || width%2 != 0 {
		return nil, fmt.Errorf("%w: got %d; it must be even, from %d to %d", ErrWidth, width, MinWidth, MaxWidth)
	}
	a := w.Attribute(yaee.AttrElevation)
	if a == nil {
		return nil, fmt.Errorf("%w: no attribute named %q", ErrNoElevation, yaee.AttrElevation)
	}
	if a.Type != yaee.Float64 || a.Components != 1 || len(a.Float64s) != w.NumSamples() {
		return nil, fmt.Errorf("%w: %q is %s with %d components and %d values for %d samples",
			ErrNoElevation, yaee.AttrElevation, a.Type, a.Components, len(a.Float64s), w.NumSamples())
	}
	locator, err := newLocator(w)
	if err != nil {
		return nil, err
	}

	height := width / 2
	img := image.NewNRGBA(image.Rect(0, 0, width, height))

	// The direction of each column's longitude is the same on every row.
	cosLon, sinLon := make([]float64, width), make([]float64, width)
	for col := range width {
		sinLon[col], cosLon[col] = math.Sincos(-math.Pi + (float64(col)+0.5)*2*math.Pi/float64(width))
	}

	// Each row is drawn by one goroutine, from read-only data, so the
	// image does not depend on how the rows are shared out.
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
				pix := img.Pix[row*img.Stride:]
				for col := range width {
					p := sphere.Vec{X: cosLat * cosLon[col], Y: cosLat * sinLon[col], Z: sinLat}
					c := ElevationColour(a.Float64s[locator.Nearest(p)])
					pix[4*col], pix[4*col+1], pix[4*col+2], pix[4*col+3] = c.R, c.G, c.B, c.A
				}
			}
		})
	}
	wg.Wait()
	return img, nil
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

// WritePNG encodes the image as a PNG. The same image always gives the
// same bytes from the same build of the command.
func WritePNG(w io.Writer, img image.Image) error {
	return png.Encode(w, img)
}
