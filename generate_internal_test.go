// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import (
	"context"
	"reflect"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/rocks"
)

// TestRockTypeAddsOnly checks that the rock type stage leaves every
// other attribute as the stages before it wrote it, for a Config that
// exercises every tectonic phenomenon.
func TestRockTypeAddsOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Seed, cfg.SampleSpacing, cfg.Duration = 3, 200, 60

	all := stages()
	if _, ok := all[len(all)-1].(rocks.RockType); !ok {
		t.Fatalf("the last stage is %T, want rocks.RockType", all[len(all)-1])
	}
	worlds := make([]*World, 2)
	for k, st := range [][]model.Stage{all[:len(all)-1], all} {
		mw, err := newModelWorld(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := model.Run(context.Background(), mw, cfg.Seed, st); err != nil {
			t.Fatal(err)
		}
		worlds[k] = export(cfg, mw)
	}
	without, with := worlds[0], worlds[1]
	for _, a := range without.Attributes {
		if a.Name == AttrRockType {
			continue
		}
		if !reflect.DeepEqual(a, *with.Attribute(a.Name)) {
			t.Errorf("attribute %s differs with the rock type stage", a.Name)
		}
	}
	rock := with.Attribute(AttrRockType)
	for i, r := range rock.Uint8s {
		if r == RockUnclassified || r > RockSedimentary {
			t.Fatalf("sample %d has rock type %d", i, r)
		}
	}
}
