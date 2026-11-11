// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package sphere holds the geometry of points on a sphere: vectors,
// rotations, sample placement, triangulation, and point location.
//
// Positions are unit vectors from the centre of the sphere. Angles are in
// radians. A distance along the surface is an angle multiplied by the
// radius; nothing in this package except SampleCount and SampleSpacing
// takes a radius.
package sphere
