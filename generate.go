// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import (
	"context"
	"fmt"
	"math/rand/v2"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// Generate builds the world that cfg describes.
//
// It returns an error wrapping ErrInvalidConfig, and no world, if
// cfg.Validate fails. It returns ctx.Err(), and no world, if ctx is
// cancelled before it finishes; cancellation is noticed within about a
// second at the default scale.
//
// Equal Config values give equal worlds on the same CPU architecture,
// whatever the number of processors. Worlds generated on different
// architectures may differ.
//
// At the default scale Generate takes under a second and uses about
// 200 MB of memory. It may run goroutines, all of which have ended when it
// returns.
func Generate(ctx context.Context, cfg Config) (*World, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// The samples and their triangulation are computed once and never
	// change (§6).
	mesh, err := sphere.Triangulate(sphere.FibonacciPoints(cfg.sampleCount()))
	if err != nil {
		return nil, fmt.Errorf("yaee: triangulating %d samples: %w", cfg.sampleCount(), err)
	}

	mw := model.NewWorld(mesh)
	mw.Radius = cfg.Radius
	mw.Duration = cfg.Duration
	mw.TimeStep = cfg.TimeStep
	if err := model.Run(ctx, mw, cfg.Seed, stages()); err != nil {
		return nil, err
	}
	return export(cfg, mw), nil
}

// stages returns the stages of generation, in the order they run.
func stages() []model.Stage {
	return []model.Stage{placeholder{}}
}

// export copies the working data into a World.
func export(cfg Config, mw *model.World) *World {
	w := &World{
		Config:    cfg,
		Version:   version.String(),
		Positions: make([][3]float64, len(mw.Mesh.Points)),
		Triangles: make([][3]int32, mw.Mesh.NumTriangles()),
	}
	for i, p := range mw.Mesh.Points {
		w.Positions[i] = [3]float64{p.X, p.Y, p.Z}
	}
	for t := range w.Triangles {
		w.Triangles[t] = [3]int32(mw.Mesh.Tri[3*t : 3*t+3])
	}
	w.Attributes = []Attribute{
		{Name: AttrElevation, Unit: "km", Type: Float64, Components: 1, Float64s: mw.Elevation},
	}
	return w
}

// placeholder is the only stage until tectonics exists. It sets the
// elevation to a smooth function of position, so that images and
// datafiles of the skeleton are not uniform. It is not part of the
// paper's model, and it draws no random numbers.
type placeholder struct{}

func (placeholder) Name() string { return "placeholder" }

func (placeholder) Run(ctx context.Context, w *model.World, _ *rand.Rand) error {
	return model.ParallelRange(ctx, len(w.Elevation), func(lo, hi int) {
		for i := lo; i < hi; i++ {
			w.Elevation[i] = placeholderElevation(w.Mesh.Points[i])
		}
	})
}

// placeholderElevation is the placeholder stage's elevation in km at a
// unit position: 4(x² − y²) + 6yz − 1. It is written with explicit
// conversions so that no multiplication is fused with an addition, which
// would make the result depend on the compiler.
func placeholderElevation(p sphere.Vec) float64 {
	return float64(4*(float64(p.X*p.X)-float64(p.Y*p.Y))) + float64(6*p.Y*p.Z) - 1
}
