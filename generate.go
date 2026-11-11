// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import (
	"context"
	"fmt"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
	"github.com/maloquacious/yaee/internal/tectonics"
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
// At the default scale Generate takes about 6 seconds on ten processors
// and uses about 570 MB of memory. It may run goroutines, all of which
// have ended when it returns.
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

	mw := model.NewWorld(mesh, model.Params{
		Radius:               cfg.Radius,
		Duration:             cfg.Duration,
		TimeStep:             cfg.TimeStep,
		Plates:               cfg.Plates,
		PlateWarpAmplitude:   cfg.PlateWarpAmplitude,
		PlateWarpFeatureSize: cfg.PlateWarpFeatureSize,

		LandCoverage:            cfg.LandCoverage,
		RidgeElevation:          cfg.RidgeElevation,
		AbyssalElevation:        cfg.AbyssalElevation,
		TrenchElevation:         cfg.TrenchElevation,
		MaxContinentalElevation: cfg.MaxContinentalElevation,

		MaxPlateSpeed: cfg.MaxPlateSpeed,

		OceanicDampening: cfg.OceanicDampening,
	})
	if err := model.Run(ctx, mw, cfg.Seed, stages()); err != nil {
		return nil, err
	}
	return export(cfg, mw), nil
}

// stages returns the stages of generation, in the order they run.
func stages() []model.Stage {
	return []model.Stage{tectonics.Plates{}, tectonics.Crust{}, tectonics.Motion{}, tectonics.Simulation{}}
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
		{Name: AttrPlate, Unit: "", Type: Int32, Components: 1, Int32s: mw.Plate},
		{Name: AttrCrustType, Unit: "", Type: Uint8, Components: 1, Uint8s: mw.CrustType},
		{Name: AttrThickness, Unit: "km", Type: Float64, Components: 1, Float64s: mw.Thickness},
		{Name: AttrElevation, Unit: "km", Type: Float64, Components: 1, Float64s: mw.Elevation},
		{Name: AttrOceanicAge, Unit: "My", Type: Float64, Components: 1, Float64s: mw.OceanicAge},
		{Name: AttrRidgeDirection, Unit: "", Type: Float64, Components: 3, Float64s: flatten(mw.RidgeDirection)},
		{Name: AttrOrogenyAge, Unit: "My", Type: Float64, Components: 1, Float64s: mw.OrogenyAge},
		{Name: AttrOrogenyType, Unit: "", Type: Uint8, Components: 1, Uint8s: mw.OrogenyType},
		{Name: AttrFoldDirection, Unit: "", Type: Float64, Components: 3, Float64s: flatten(mw.FoldDirection)},
	}
	return w
}

// flatten returns the vectors' components: x, y, z of each in turn.
func flatten(vecs []sphere.Vec) []float64 {
	out := make([]float64, 0, 3*len(vecs))
	for _, v := range vecs {
		out = append(out, v.X, v.Y, v.Z)
	}
	return out
}
