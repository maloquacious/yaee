// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import "fmt"

// World is a generated world: a set of samples spread evenly over a
// sphere, the triangulation that connects them, and data for each
// sample.
//
// Samples are numbered from 0. Sample i has position Positions[i] and,
// in each attribute, the Components values starting at index
// i*Components.
//
// Generate and Read return a World whose slices are consistent with one
// another. Treat it as read-only: a World changed by hand may be
// rejected by Write.
type World struct {
	// Config is the configuration that generated the world.
	Config Config

	// Version is the version of this library that generated the world,
	// as a semantic version string such as "0.6.0-alpha".
	Version string

	// Positions holds each sample's position as a unit vector (x, y, z)
	// from the centre of the sphere, without unit. Its length is the
	// number of samples.
	//
	// The +z axis passes through the north pole. Latitude is asin(z),
	// and longitude is atan2(y, x), east positive. The sample's point at
	// sea level, in km, is its position multiplied by Config.Radius.
	//
	// Sample 0 is the nearest to the north pole and the last sample the
	// nearest to the south pole; z decreases with the sample number.
	Positions [][3]float64

	// Triangles is the Delaunay triangulation of the samples on the
	// sphere. Each element is the sample numbers of one triangle's
	// corners, counter-clockwise as seen from outside the sphere. There
	// are 2N − 4 triangles for N samples, and they cover the sphere
	// without gaps or overlaps.
	//
	// Two samples are neighbours when some triangle has both as corners.
	// Every sample has at least three neighbours.
	Triangles [][3]int32

	// Attributes holds the per-sample data, in a fixed order. Names are
	// unique. Use Attribute to find one by name.
	Attributes []Attribute
}

// Names of the attributes that Generate produces.
const (
	// AttrElevation is the height of the surface above sea level.
	// Type Float64, 1 component, unit "km". Negative below sea level.
	//
	// As of this version it is a placeholder, not a simulated value:
	// for a sample at (x, y, z) it is 4(x² − y²) + 6yz − 1.
	AttrElevation = "elevation"
)

// NumSamples returns the number of samples.
func (w *World) NumSamples() int { return len(w.Positions) }

// NumTriangles returns the number of triangles.
func (w *World) NumTriangles() int { return len(w.Triangles) }

// Attribute returns the attribute with the given name, or nil if the
// world has none. The result points into w.Attributes.
func (w *World) Attribute(name string) *Attribute {
	for i := range w.Attributes {
		if w.Attributes[i].Name == name {
			return &w.Attributes[i]
		}
	}
	return nil
}

// Attribute is one named kind of per-sample data.
//
// Exactly one of Uint8s, Int32s, Float32s, and Float64s is in use: the
// one that Type selects. Its length is the number of samples multiplied
// by Components, and the values for sample i are the Components elements
// starting at index i*Components. The other three slices are nil.
type Attribute struct {
	// Name identifies the attribute. It is 1 to 255 bytes of UTF-8.
	Name string

	// Unit is the unit of every element, such as "km" or "My". It is
	// empty for a value without a unit, and at most 255 bytes of UTF-8.
	Unit string

	// Type is the type of each element.
	Type ElementType

	// Components is the number of elements per sample, 1 to 255:
	// 1 for a scalar, 3 for a vector (x, y, z).
	Components int

	Uint8s   []uint8   // the data when Type is Uint8
	Int32s   []int32   // the data when Type is Int32
	Float32s []float32 // the data when Type is Float32
	Float64s []float64 // the data when Type is Float64
}

// ElementType is the type of an attribute's elements. Its numeric value
// is the type code stored in the datafile.
type ElementType uint8

// The element types.
const (
	Uint8   ElementType = 1 // unsigned 8-bit integer
	Int32   ElementType = 2 // signed 32-bit integer
	Float32 ElementType = 3 // IEEE 754 binary32
	Float64 ElementType = 4 // IEEE 754 binary64
)

// Size returns the number of bytes one element occupies in the datafile:
// 1, 4, 4, or 8. It returns 0 for a value that is not one of the element
// types.
func (t ElementType) Size() int {
	switch t {
	case Uint8:
		return 1
	case Int32, Float32:
		return 4
	case Float64:
		return 8
	}
	return 0
}

// String returns "uint8", "int32", "float32", or "float64", or
// "ElementType(n)" for any other value n.
func (t ElementType) String() string {
	switch t {
	case Uint8:
		return "uint8"
	case Int32:
		return "int32"
	case Float32:
		return "float32"
	case Float64:
		return "float64"
	}
	return fmt.Sprintf("ElementType(%d)", uint8(t))
}
