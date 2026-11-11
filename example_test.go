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

	crust := world.Attribute(yaee.AttrCrustType)
	for _, i := range []int{100, 400} {
		kind := "oceanic"
		if crust.Uint8s[i] == yaee.CrustContinental {
			kind = "continental"
		}
		p := world.Positions[i]
		fmt.Printf("sample %d is at (%.3f, %.3f, %.3f): %s crust, %.3f km above sea level\n",
			i, p[0], p[1], p[2], kind, elevation.Float64s[i])
	}

	// Output:
	// samples: 589
	// triangles: 1174
	// elevation: float64 in km
	// sample 100 is at (0.248, -0.710, 0.659): continental crust, 0.243 km above sea level
	// sample 400 is at (0.212, 0.909, -0.360): oceanic crust, -6.702 km above sea level
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
