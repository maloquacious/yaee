// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package yaee simulates plate tectonics on a sphere.
//
// Most of the model follows the process described in Procedural Tectonic Planets
//
//	Yann Cortial, Adrien Peytavie, Eric Galin, Eric Guérin.
//	Computer Graphics Forum, 2019, 38 (2), ⟨10.1111/cgf.13614⟩. ⟨hal-02136820⟩
package yaee

import (
	"github.com/maloquacious/semver"
)

var (
	version = semver.Version{
		Major:      0,
		Minor:      4,
		Patch:      0,
		PreRelease: "alpha",
	}
)

// Version returns the version of the package.
func Version() semver.Version {
	return version
}
