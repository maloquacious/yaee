// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"context"
	"math"
	"math/rand/v2"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Motion is the stage that gives every plate its initial movement (§3:
// "a rigid geodetic movement, denoted as G", "defined as a rotation with
// an axis w passing through the center of the planet" and an angular
// speed ω). It sets World.PlateAxis, World.PlateSpeed, and
// World.PlateRotation. It must run after Plates.
//
// The paper lets the user prescribe the movements and gives no rule for
// drawing them, so this one is ours. For each plate, in order of plate
// number:
//
//  1. The axis is a point drawn uniformly from the sphere.
//  2. A speed v is drawn uniformly from 0 to MaxPlateSpeed (v0), in
//     km/My.
//  3. The angular speed is ω = v / (R · m), where m is the greatest value
//     of |w × p| over the plate's samples p: the sine of the angle from
//     the axis of the sample that moves fastest. That sample therefore
//     moves at exactly v, and no sample of any plate moves faster than
//     v0. A plate with m = 0 gets ω = 0.
//
// The speed of the fastest point is limited, not ω alone, because a
// point's speed is ω R sin(angle from the axis): two plates with the same
// ω move at very different speeds if one lies at its axis and the other
// a quarter turn from it.
//
// The rotation since the last resampling starts as the identity.
type Motion struct{}

// Name returns "motion".
func (Motion) Name() string { return "motion" }

// Run applies the stage.
func (Motion) Run(ctx context.Context, w *model.World, rng *rand.Rand) error {
	w.PlateAxis = make([]sphere.Vec, w.NumPlates)
	w.PlateSpeed = make([]float64, w.NumPlates)
	w.PlateRotation = make([]sphere.Rotation, w.NumPlates)
	linear := make([]float64, w.NumPlates) // v of step 2
	for k := range w.NumPlates {
		z := 1 - 2*rng.Float64()
		r := math.Sqrt(max(0, 1-z*z))
		sin, cos := math.Sincos(2 * math.Pi * rng.Float64())
		w.PlateAxis[k] = sphere.Vec{X: r * cos, Y: r * sin, Z: z}
		linear[k] = w.MaxPlateSpeed * rng.Float64()
		w.PlateRotation[k] = sphere.Identity()
	}
	for k, m := range fastestSines(w) {
		if m > 0 {
			w.PlateSpeed[k] = linear[k] / (w.Radius * m)
		}
	}
	return ctx.Err()
}

// fastestSines returns, for each plate, the greatest |w × p| over its
// samples p, with w the plate's axis: the sine of the angle from the
// axis of the plate's fastest sample. It is 0 for a plate without
// samples.
func fastestSines(w *model.World) []float64 {
	m2 := make([]float64, w.NumPlates)
	for i, p := range w.Mesh.Points {
		k := w.Plate[i]
		c := w.PlateAxis[k].Cross(p)
		m2[k] = max(m2[k], c.Dot(c))
	}
	for k := range m2 {
		m2[k] = math.Sqrt(m2[k])
	}
	return m2
}

// limitSpeeds lowers the angular speed of every plate whose fastest
// sample would move faster than MaxPlateSpeed, so that it moves at
// exactly that speed. Resampling changes which samples a plate holds,
// and keeps its angular speed; a plate that gains samples further from
// its axis would otherwise pass the limit.
func limitSpeeds(w *model.World) {
	for k, m := range fastestSines(w) {
		if w.PlateSpeed[k]*w.Radius*m > w.MaxPlateSpeed {
			w.PlateSpeed[k] = w.MaxPlateSpeed / (w.Radius * m)
		}
	}
}

// maxSpeed returns the speed in km/My of the fastest sample of any
// plate.
func maxSpeed(w *model.World) float64 {
	var v float64
	for k, m := range fastestSines(w) {
		v = max(v, w.PlateSpeed[k]*w.Radius*m)
	}
	return v
}

// Advance moves every plate through dt My: plate k turns a further
// PlateSpeed[k] · dt radians about PlateAxis[k]. A plate with speed 0 is
// left exactly as it was.
func Advance(w *model.World, dt float64) {
	for k := range w.PlateRotation {
		w.PlateRotation[k] = w.PlateRotation[k].Then(sphere.NewRotation(w.PlateAxis[k], w.PlateSpeed[k]*dt))
	}
}

// Position returns where sample i is now: its position in the mesh
// turned by the rotation its plate has made since the last resampling.
func Position(w *model.World, i int) sphere.Vec {
	return w.PlateRotation[w.Plate[i]].Apply(w.Mesh.Points[i])
}

// Velocity returns the velocity, in km/My, of plate k's surface at the
// unit position p: s(p) = ω w × p (§3), multiplied by the radius. It is
// tangent to the sphere at p, and its length is ω R sin(the angle
// between p and the axis). p is where the point is now, not where it was
// at the last resampling.
func Velocity(w *model.World, k int, p sphere.Vec) sphere.Vec {
	return w.PlateAxis[k].Cross(p).Scale(w.PlateSpeed[k] * w.Radius)
}

// Query reports what plate k holds at the unit position q.
//
// q is turned back by the plate's rotation since the last resampling and
// located in the mesh. tri is the triangle found. Its corner nearest to
// the turned-back point, meaning the one with the greatest barycentric
// weight (the lowest-numbered sample among equals), is the point's
// owner, and ok reports whether the owner is a sample of plate k that
// has not been subducted (World.Subducted). A plate holds nothing where
// its crust has gone beneath another plate.
//
// A triangle whose corners are in different plates therefore belongs to
// no single plate: it is divided between them, each plate holding the
// part nearest its own corners. The plates' parts meet without gap or
// overlap, so before any plate has moved every point of the sphere is
// held by exactly one plate. The alternative, giving a plate only the
// triangles that have all three corners in it, would leave a strip one
// triangle wide between every pair of plates held by neither.
//
// When ok is true, weights are the weights with which to interpolate the
// plate's data at q from the corners of tri, corner Tri[3·tri+c] having
// weights[c]: the barycentric weights of the turned-back point with the
// weight of every corner outside plate k set to 0 and the rest scaled to
// sum to 1, because a corner in another plate carries that plate's
// crust. If the turned-back point is exactly a sample's position, that
// corner's weight is exactly 1. When ok is false, weights is zero.
func Query(w *model.World, k int, q sphere.Vec) (tri int32, weights [3]float64, ok bool) {
	return query(w, int32(k), w.PlateRotation[k].Inverse().Apply(q))
}

// query is Query for a point already turned back by the plate's
// rotation.
func query(w *model.World, k int32, back sphere.Vec) (tri int32, weights [3]float64, ok bool) {
	mesh := w.Mesh
	tri, weights = w.Locator.Locate(back)
	corners := mesh.Tri[3*tri : 3*tri+3]
	owner := nearestCorner(corners, weights)
	if w.Plate[corners[owner]] != k || w.Subducted[corners[owner]] {
		return tri, [3]float64{}, false
	}
	// Rounding leaves a point at a corner with weights a hair away from
	// 1, 0, 0. Make them exact, so that a plate that has not moved gets
	// back exactly the data it holds.
	if mesh.Points[corners[owner]] == back {
		weights = [3]float64{}
		weights[owner] = 1
		return tri, weights, true
	}
	var sum float64
	for c, s := range corners {
		if w.Plate[s] != k {
			weights[c] = 0
		}
		sum += weights[c]
	}
	for c := range weights {
		weights[c] /= sum
	}
	return tri, weights, true
}

// nearestCorner returns which of a triangle's three corners has the
// greatest weight: 0, 1, or 2. Among equal weights it is the corner with
// the lowest sample number.
func nearestCorner(corners []int32, weights [3]float64) int {
	c := 0
	for j := 1; j < 3; j++ {
		if weights[j] > weights[c] || weights[j] == weights[c] && corners[j] < corners[c] {
			c = j
		}
	}
	return c
}

// ownerAt returns the sample that owns the point back of the mesh: the
// nearest corner of the triangle that contains it. The point belongs to
// that sample's plate as the plates lay at the last resampling, whether
// or not the sample has been subducted since.
func ownerAt(w *model.World, back sphere.Vec) int32 {
	tri, weights := w.Locator.Locate(back)
	corners := w.Mesh.Tri[3*tri : 3*tri+3]
	return corners[nearestCorner(corners, weights)]
}
