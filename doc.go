// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package yaee generates worlds by simulating plate tectonics on a
// sphere.
//
// A world is a set of samples spread evenly over the sphere, the
// triangulation that connects them, and named data for each sample. The
// default configuration is Earth-like: radius 6370 km and 25 km between
// samples, which is 942,059 samples.
//
// To generate a world, start from DefaultConfig, change fields, and call
// Generate:
//
//	cfg := yaee.DefaultConfig()
//	cfg.Seed = 42
//	world, err := yaee.Generate(ctx, cfg)
//
// Equal Config values give equal worlds on the same CPU architecture.
//
// Write encodes a World as a datafile and Read decodes one. The datafile
// format is specified in docs/datafile.md, completely enough to write a
// reader in another language; FormatVersion is the version this library
// reads and writes.
//
//	err := yaee.Write(file, world)
//	world, err := yaee.Read(file)
//
// A client that wants the world on a grid of its own, such as the cells
// of a map, builds a Locator and asks it, for each of its points, which
// triangle of samples contains the point and which sample is nearest:
//
//	loc, err := yaee.NewLocator(world)
//	triangle, weights := loc.Locate(x, y, z)
//	sample := loc.Nearest(x, y, z)
//
// Lengths are in kilometres (km) and times in millions of years (My).
//
// The package does not print, read the environment or command-line
// flags, or exit. Every failure is returned as an error.
//
// As of this version the initial plates and crust are generated, and
// the plates then move as rigid bodies for Config.Duration, carrying
// their crust. Where plates part, new oceanic crust forms at the ridge
// between them, and all oceanic crust ages and sinks. Where plates
// meet, one subducts beneath the other: a trench forms along the front,
// the overriding plate is raised, and land erodes. Where two continents
// meet they collide: the smaller leaves its plate and joins the other,
// and the land on both sides of the suture is raised. From time to
// time a plate breaks into two to four plates that move apart, which
// keeps the land from gathering into one continent.
//
// The tectonic model is that of
//
//	Yann Cortial, Adrien Peytavie, Eric Galin, Eric Guérin.
//	Procedural Tectonic Planets.
//	Computer Graphics Forum, 2019, 38 (2). doi:10.1111/cgf.13614. hal-02136820.
package yaee
