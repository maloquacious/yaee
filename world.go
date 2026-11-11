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

// Names of the attributes that Generate produces, in the order they
// appear in World.Attributes.
const (
	// AttrPlate is the number of the tectonic plate the sample belongs
	// to. Type Int32, 1 component, no unit. Range: 0 to
	// Config.Plates−1.
	//
	// In the initial world (Config.Duration 0) every number in that
	// range is used by at least one sample, and the samples of one plate
	// form a single connected region: any two of them are joined by a
	// path of neighbouring samples of the same plate. Once plates have
	// moved, neither holds: a plate may be in several pieces, and a
	// plate that others have covered may have no samples.
	AttrPlate = "plate"

	// AttrCrustType is the type of the crust (x_C in the paper's
	// Table 1). Type Uint8, 1 component, no unit. Values: CrustOceanic
	// (0) or CrustContinental (1).
	AttrCrustType = "crust_type"

	// AttrThickness is the thickness of the crust (e). Type Float64,
	// 1 component, unit "km". Range: greater than 0. It starts at 7 for
	// oceanic crust and 35 for continental crust.
	AttrThickness = "thickness"

	// AttrElevation is the height of the surface above sea level (z).
	// Type Float64, 1 component, unit "km". Negative below sea level.
	// Range: Config.TrenchElevation to Config.MaxContinentalElevation.
	//
	// Oceanic crust sinks toward Config.TrenchElevation as it ages and
	// is at Config.TrenchElevation in a trench at a subduction front.
	// Crust behind a subduction front is raised, and land is eroded
	// toward 0. Oceanic crust may be above 0 where it has been raised.
	AttrElevation = "elevation"

	// AttrOceanicAge is the time since oceanic crust formed at a ridge
	// (a_o). Type Float64, 1 component, unit "My". Range: at least 0.
	// It is 0 for continental crust.
	AttrOceanicAge = "oceanic_age"

	// AttrRidgeDirection is the direction of the ridge at which oceanic
	// crust formed (r). Type Float64, 3 components (x, y, z on the axes
	// of World.Positions), no unit. It is a unit vector perpendicular
	// to the sample's position, or (0, 0, 0) where no direction is
	// known. It is (0, 0, 0) for continental crust.
	AttrRidgeDirection = "ridge_direction"

	// AttrOrogenyAge is the time since the mountain building that last
	// affected the sample began (a_c). Type Float64, 1 component, unit
	// "My". Range: 0 to Config.Duration. It is 0 where the orogeny type
	// is OrogenyNone.
	AttrOrogenyAge = "orogeny_age"

	// AttrOrogenyType is the kind of mountain building that last
	// affected the sample (o). Type Uint8, 1 component, no unit.
	// Values: OrogenyNone (0), OrogenyAndean (1), or OrogenyHimalayan
	// (2).
	//
	// A sample becomes OrogenyAndean, with orogeny age 0, when
	// subduction raises it above sea level, or raises it while it is
	// above sea level and has no orogeny type. As of this version no
	// sample is OrogenyHimalayan.
	AttrOrogenyType = "orogeny_type"

	// AttrFoldDirection is the direction in which the crust is folded
	// (f). Type Float64, 3 components (x, y, z on the axes of
	// World.Positions), no unit. It is a vector perpendicular to the
	// sample's position, not necessarily of length 1, or (0, 0, 0)
	// where the crust is not folded.
	//
	// Behind a subduction front it points the way the subducting plate
	// moves relative to the sample's plate, and its length is the
	// distance the two plates have converged while the sample was
	// behind a front, in units of 100 km.
	AttrFoldDirection = "fold_direction"

	// AttrUplift is the rate at which subduction raised the sample in
	// the last time step of the simulation (ũ). Type Float64, 1
	// component, unit "km/My". Range: 0 to 2 × Config.SubductionUplift.
	// It is 0 where nothing was raised, and for every sample when
	// Config.Duration is 0.
	AttrUplift = "uplift"
)

// Values of the AttrCrustType attribute.
const (
	CrustOceanic     uint8 = 0
	CrustContinental uint8 = 1
)

// Values of the AttrOrogenyType attribute.
const (
	OrogenyNone      uint8 = 0 // no mountain building has affected the sample
	OrogenyAndean    uint8 = 1 // raised where an oceanic plate subducts beneath it
	OrogenyHimalayan uint8 = 2 // raised by the collision of two continents
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
