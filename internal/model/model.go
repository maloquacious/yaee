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

// World is the working data of a world. Per-sample data is one slice per
// attribute, indexed by sample number.
type World struct {
	Radius   float64 // km
	Duration float64 // My
	TimeStep float64 // My

	// Mesh holds the sample positions and their triangulation. Stages
	// must not change it.
	Mesh *sphere.Mesh

	// Elevation is the height of each sample above sea level, in km.
	Elevation []float64
}

// NewWorld returns a World over the mesh with every per-sample slice
// allocated and zero.
func NewWorld(mesh *sphere.Mesh) *World {
	return &World{
		Mesh:      mesh,
		Elevation: make([]float64, len(mesh.Points)),
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
