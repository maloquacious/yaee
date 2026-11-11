// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package model holds the working data of a world while it is generated,
// and runs the stages that fill it in.
//
// The root package builds a World from its Config, runs the stages over
// it with Run, and copies the result into the public world type. A stage
// reads and writes the World's per-sample slices; it never sees the
// public types.
//
// Units are kilometres and millions of years (My).
package model

import (
	"context"
	"hash/fnv"
	"math/rand/v2"
	"runtime"
	"sync"

	"github.com/maloquacious/yaee/internal/sphere"
)

// Params are the parameters of generation: the fields of the public
// Config that stages read. Each has the unit and range of the Config
// field it is copied from.
type Params struct {
	Radius   float64 // km
	Duration float64 // My
	TimeStep float64 // My

	Plates               int     // number of plates at the start
	PlateWarpAmplitude   float64 // km
	PlateWarpFeatureSize float64 // km

	LandCoverage            float64 // fraction of the sphere's area, 0 to 1
	RidgeElevation          float64 // km; z_r of the paper's Table 3
	AbyssalElevation        float64 // km; z_a
	TrenchElevation         float64 // km; z_t
	MaxContinentalElevation float64 // km; z_c

	MaxPlateSpeed float64 // km/My; v0 of the paper's Table 3

	OceanicDampening float64 // km/My; ε_o of the paper's Table 3

	SubductionDistance float64 // km; r_s
	SubductionUplift   float64 // km/My; u_0
	ContinentalErosion float64 // km/My; ε_c
	SedimentAccretion  float64 // km/My; ε_t

	CollisionPenetration float64 // km; the interpenetration distance of §6
	CollisionDistance    float64 // km; r_c
	CollisionCoefficient float64 // per km; Δ_c

	RiftingRate float64 // per My; λ_0 of §4.4
}

// Values of World.CrustType.
const (
	Oceanic     uint8 = 0
	Continental uint8 = 1
)

// Values of World.OrogenyType.
const (
	OrogenyNone      uint8 = 0 // no mountain building has affected the sample
	OrogenyAndean    uint8 = 1 // raised by subduction (§4.1)
	OrogenyHimalayan uint8 = 2 // raised by continental collision (§4.2)
)

// World is the working data of a world. Per-sample data is one slice per
// attribute, indexed by sample number.
type World struct {
	Params

	// Mesh holds the sample positions and their triangulation. Stages
	// must not change it.
	Mesh *sphere.Mesh

	// Locator finds the triangle of Mesh that contains a point.
	Locator *sphere.Locator

	// NumPlates is the number of plates. Plates are numbered from 0.
	NumPlates int

	// Plate is the number of the plate each sample belongs to, from 0
	// to NumPlates-1.
	Plate []int32

	// The movement of each plate (G in the paper's §3), indexed by plate
	// number. The three slices have length NumPlates once the motion
	// stage has run, and are nil before it.
	//
	// A sample's position in Mesh is where its crust was at the last
	// resampling. Where it is now is that position turned by its
	// plate's PlateRotation.

	// PlateAxis is the unit axis w each plate turns about. A positive
	// speed turns the plate counter-clockwise as seen from the tip of
	// the axis.
	PlateAxis []sphere.Vec

	// PlateSpeed is each plate's angular speed ω, in radians per My. It
	// is at least 0.
	PlateSpeed []float64

	// PlateRotation is the rotation each plate has made since the last
	// resampling.
	PlateRotation []sphere.Rotation

	// PlateOceanAge is, for each plate, the mean oceanic age in My of
	// its oceanic samples as they were at the last resampling. It
	// decides which of two oceanic plates subducts. It is nil until the
	// simulation first needs it.
	PlateOceanAge []float64

	// The crust at each sample: the parameters of the paper's Table 1.

	// CrustType is Oceanic or Continental (x_C).
	CrustType []uint8

	// Thickness is the thickness of the crust, in km (e).
	Thickness []float64

	// Elevation is the height of each sample above sea level, in km (z).
	Elevation []float64

	// OceanicAge is the time since oceanic crust formed at a ridge, in
	// My (a_o). It is 0 for continental crust.
	OceanicAge []float64

	// RidgeDirection is the direction of the ridge where oceanic crust
	// formed (r): a unit vector tangent to the sphere at the sample, or
	// the zero vector where no direction is known. It is the zero
	// vector for continental crust.
	RidgeDirection []sphere.Vec

	// OrogenyAge is the time since the sample's latest orogeny began,
	// in My (a_c). It is 0 where OrogenyType is OrogenyNone.
	OrogenyAge []float64

	// OrogenyType is OrogenyNone, OrogenyAndean, or OrogenyHimalayan (o).
	OrogenyType []uint8

	// FoldDirection is the direction of folding of the crust (f): a
	// vector tangent to the sphere at the sample, or the zero vector
	// where the crust has not been folded.
	FoldDirection []sphere.Vec

	// Working data of subduction (§4.1), beyond the paper's Table 1.

	// Subducted marks the samples whose crust has gone beneath another
	// plate since the last resampling. Such a sample still belongs to
	// its plate and moves with it, but its plate no longer holds
	// anything where the sample is. Resampling clears every mark.
	Subducted []bool

	// Uplift is the rate at which subduction raised each sample in the
	// latest time step, in km/My (ũ). It is 0 where nothing was raised.
	Uplift []float64

	// Trench is how far each sample has been lowered into an oceanic
	// trench, in km. It is at least 0, and 0 outside a trench. The
	// sample's elevation without the trench is Elevation + Trench.
	Trench []float64
}

// NewWorld returns a World over the mesh with every per-sample slice
// allocated and zero, and a Locator for the mesh. It has no plates:
// NumPlates is 0.
func NewWorld(mesh *sphere.Mesh, params Params) *World {
	n := len(mesh.Points)
	return &World{
		Params:         params,
		Mesh:           mesh,
		Locator:        sphere.NewLocator(mesh),
		Plate:          make([]int32, n),
		CrustType:      make([]uint8, n),
		Thickness:      make([]float64, n),
		Elevation:      make([]float64, n),
		OceanicAge:     make([]float64, n),
		RidgeDirection: make([]sphere.Vec, n),
		OrogenyAge:     make([]float64, n),
		OrogenyType:    make([]uint8, n),
		FoldDirection:  make([]sphere.Vec, n),
		Subducted:      make([]bool, n),
		Uplift:         make([]float64, n),
		Trench:         make([]float64, n),
	}
}

// A Stage is one step of generation. Stages run in order, each adding to
// what the earlier ones left in the World.
type Stage interface {
	// Name identifies the stage. It selects the stage's random stream,
	// so renaming a stage changes the worlds it generates.
	Name() string

	// Run applies the stage to w. All randomness must be drawn from rng.
	// Run returns ctx.Err() promptly once ctx is cancelled.
	Run(ctx context.Context, w *World, rng *rand.Rand) error
}

// Run applies the stages to w in order. It stops at the first error, and
// returns ctx.Err() if ctx is cancelled before a stage starts.
func Run(ctx context.Context, w *World, seed uint64, stages []Stage) error {
	for _, s := range stages {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.Run(ctx, w, Stream(seed, s.Name())); err != nil {
			return err
		}
	}
	return nil
}

// Stream returns the random stream for the named stage: a PCG generator
// seeded with the world's seed and the 64-bit FNV-1a hash of the name.
// A stage's stream depends on nothing but the seed and its own name, so
// adding, removing, or reordering other stages does not change the
// numbers it draws.
func Stream(seed uint64, name string) *rand.Rand {
	h := fnv.New64a()
	h.Write([]byte(name))
	return rand.New(rand.NewPCG(seed, h.Sum64()))
}

// parallelChunk is the number of indexes handed to a goroutine at a time.
const parallelChunk = 4096

// ParallelRange calls fn(lo, hi) for consecutive half-open ranges that
// together cover [0, n), from several goroutines at once. It returns
// ctx.Err() if ctx was cancelled before every range was started; fn may
// then have run on only part of [0, n).
//
// The result is repeatable only if fn obeys the project's rule: each
// output value depends on read-only input and on its own index alone,
// and fn(lo, hi) writes nothing outside indexes lo to hi-1. fn must not
// draw random numbers from a source shared between calls.
func ParallelRange(ctx context.Context, n int, fn func(lo, hi int)) error {
	var (
		mu   sync.Mutex
		next int
		wg   sync.WaitGroup
	)
	for range min(runtime.GOMAXPROCS(0), (n+parallelChunk-1)/parallelChunk) {
		wg.Go(func() {
			for ctx.Err() == nil {
				mu.Lock()
				lo := next
				next = min(n, lo+parallelChunk)
				hi := next
				mu.Unlock()
				if lo >= hi {
					return
				}
				fn(lo, hi)
			}
		})
	}
	wg.Wait()
	return ctx.Err()
}
