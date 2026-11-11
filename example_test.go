// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee_test

import (
	"context"
	"fmt"

	"github.com/maloquacious/yaee"
)

// Generate a small world and read its data.
func Example() {
	cfg := yaee.DefaultConfig() // Earth-like, about 25 km per sample
	cfg.Seed = 42
	cfg.SampleSpacing = 1000 // km; coarse, so that the example is quick

	world, err := yaee.Generate(context.Background(), cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("samples:", world.NumSamples())
	fmt.Println("triangles:", world.NumTriangles())

	elevation := world.Attribute(yaee.AttrElevation)
	fmt.Println("elevation:", elevation.Type, "in", elevation.Unit)

	p := world.Positions[100]
	fmt.Printf("sample 100 is at (%.3f, %.3f, %.3f), %.3f km above sea level\n",
		p[0], p[1], p[2], elevation.Float64s[100])

	// Output:
	// samples: 589
	// triangles: 1174
	// elevation: float64 in km
	// sample 100 is at (0.248, -0.710, 0.659), -5.581 km above sea level
}

// A Config that is out of range is rejected with an error naming the
// field.
func ExampleConfig_Validate() {
	cfg := yaee.DefaultConfig()
	cfg.Radius = -1
	cfg.TimeStep = 0
	fmt.Println(cfg.Validate())

	// Output:
	// yaee: invalid config: Radius: must be finite and greater than 0, got -1
	// yaee: invalid config: TimeStep: must be finite and greater than 0, got 0
}
