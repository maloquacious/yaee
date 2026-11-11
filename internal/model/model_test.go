// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package model

import (
	"context"
	"errors"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/sphere"
)

type recorder struct {
	name  string
	err   error
	draws *[]uint64
	order *[]string
}

func (r recorder) Name() string { return r.name }

func (r recorder) Run(_ context.Context, _ *World, rng *rand.Rand) error {
	if r.draws != nil {
		*r.draws = append(*r.draws, rng.Uint64(), rng.Uint64())
	}
	if r.order != nil {
		*r.order = append(*r.order, r.name)
	}
	return r.err
}

func TestRunOrderAndErrors(t *testing.T) {
	var order []string
	boom := errors.New("boom")
	stages := []Stage{
		recorder{name: "a", order: &order},
		recorder{name: "b", order: &order, err: boom},
		recorder{name: "c", order: &order},
	}
	if err := Run(t.Context(), &World{}, 1, stages); !errors.Is(err, boom) {
		t.Fatalf("Run returned %v, want boom", err)
	}
	if !slices.Equal(order, []string{"a", "b"}) {
		t.Fatalf("stages ran as %v, want [a b]", order)
	}
}

func TestRunCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var order []string
	err := Run(ctx, &World{}, 1, []Stage{recorder{name: "a", order: &order}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v, want context.Canceled", err)
	}
	if len(order) != 0 {
		t.Fatalf("a stage ran after cancellation")
	}
}

// A stage's stream depends only on the seed and the stage's name, not on
// which other stages run before it.
func TestStreamIndependentOfOtherStages(t *testing.T) {
	var alone, after []uint64
	if err := Run(t.Context(), &World{}, 7, []Stage{recorder{name: "x", draws: &alone}}); err != nil {
		t.Fatal(err)
	}
	var other []uint64
	stages := []Stage{recorder{name: "w", draws: &other}, recorder{name: "x", draws: &after}}
	if err := Run(t.Context(), &World{}, 7, stages); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(alone, after) {
		t.Fatalf("stage x drew %v alone and %v after another stage", alone, after)
	}
	if slices.Equal(other, after) {
		t.Fatalf("stages w and x drew the same numbers")
	}
	var reseeded []uint64
	if err := Run(t.Context(), &World{}, 8, []Stage{recorder{name: "x", draws: &reseeded}}); err != nil {
		t.Fatal(err)
	}
	if slices.Equal(alone, reseeded) {
		t.Fatalf("seeds 7 and 8 gave the same numbers")
	}
}

func TestParallelRangeCoversEachIndexOnce(t *testing.T) {
	for _, n := range []int{0, 1, parallelChunk - 1, parallelChunk, parallelChunk + 1, 10*parallelChunk + 17} {
		seen := make([]int32, n)
		err := ParallelRange(t.Context(), n, func(lo, hi int) {
			for i := lo; i < hi; i++ {
				seen[i]++
			}
		})
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}
		for i, c := range seen {
			if c != 1 {
				t.Fatalf("n=%d: index %d visited %d times", n, i, c)
			}
		}
	}
}

func TestParallelRangeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := ParallelRange(ctx, 100*parallelChunk, func(lo, hi int) {})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want context.Canceled", err)
	}
}

func TestNewWorld(t *testing.T) {
	mesh, err := sphere.Triangulate(sphere.FibonacciPoints(20))
	if err != nil {
		t.Fatal(err)
	}
	w := NewWorld(mesh, Params{Radius: 6370, Plates: 3})
	for name, n := range map[string]int{
		"Plate": len(w.Plate), "CrustType": len(w.CrustType), "Thickness": len(w.Thickness),
		"Elevation": len(w.Elevation), "OceanicAge": len(w.OceanicAge), "RidgeDirection": len(w.RidgeDirection),
		"OrogenyAge": len(w.OrogenyAge), "OrogenyType": len(w.OrogenyType), "FoldDirection": len(w.FoldDirection),
	} {
		if n != 20 {
			t.Fatalf("len(%s) = %d, want 20", name, n)
		}
	}
	if w.Radius != 6370 || w.Plates != 3 || w.NumPlates != 0 || w.Mesh != mesh {
		t.Fatalf("NewWorld did not keep its arguments: %+v", w)
	}
}
