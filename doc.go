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
// the overriding plate is raised, and land erodes. Continents do not
// collide yet: where two meet, the crust of one is consumed. Plates do
// not split yet.
//
// The tectonic model is that of
//
//	Yann Cortial, Adrien Peytavie, Eric Galin, Eric Guérin.
//	Procedural Tectonic Planets.
//	Computer Graphics Forum, 2019, 38 (2). doi:10.1111/cgf.13614. hal-02136820.
package yaee
