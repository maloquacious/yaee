// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"testing"

	"github.com/maloquacious/yaee"
)

// initialWorld returns the initial world of about n samples.
func initialWorld(t testing.TB, n int) *yaee.World {
	t.Helper()
	cfg := smallConfig(n)
	cfg.Duration = 0
	return mustGenerate(t, cfg)
}

// randomDirection returns a direction spread evenly over the sphere,
// with a length from 0.001 to 1000.
func randomDirection(rng *rand.Rand) (x, y, z float64) {
	z = 2*rng.Float64() - 1
	lon := 2 * math.Pi * rng.Float64()
	r := math.Sqrt(1 - z*z)
	scale := math.Pow(10, 6*rng.Float64()-3)
	return scale * r * math.Cos(lon), scale * r * math.Sin(lon), scale * z
}

func TestLocatorLocate(t *testing.T) {
	w := initialWorld(t, 2000)
	loc, err := yaee.NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for range 5000 {
		x, y, z := randomDirection(rng)
		tri, wt := loc.Locate(x, y, z)
		if tri < 0 || tri >= w.NumTriangles() {
			t.Fatalf("Locate(%v, %v, %v) = triangle %d of %d", x, y, z, tri, w.NumTriangles())
		}
		// The weights rebuild a point on the ray of the direction.
		var q [3]float64
		var sum float64
		for k, s := range w.Triangles[tri] {
			if wt[k] < 0 || wt[k] > 1 {
				t.Fatalf("Locate(%v, %v, %v): weight %v", x, y, z, wt[k])
			}
			sum += wt[k]
			for a := range 3 {
				q[a] += wt[k] * w.Positions[s][a]
			}
		}
		if math.Abs(sum-1) > 1e-12 {
			t.Fatalf("Locate(%v, %v, %v): weights sum to %v", x, y, z, sum)
		}
		n, m := math.Sqrt(x*x+y*y+z*z), math.Sqrt(q[0]*q[0]+q[1]*q[1]+q[2]*q[2])
		if d := math.Hypot(math.Hypot(q[0]/m-x/n, q[1]/m-y/n), q[2]/m-z/n); d > 1e-9 {
			t.Fatalf("Locate(%v, %v, %v): the weights give a point %v away from the direction", x, y, z, d)
		}
	}
	// At a sample's position the sample has all the weight.
	for s, p := range w.Positions {
		tri, wt := loc.Locate(p[0], p[1], p[2])
		found := false
		for k, c := range w.Triangles[tri] {
			if int(c) == s {
				found = math.Abs(wt[k]-1) < 1e-9
			}
		}
		if !found {
			t.Fatalf("at sample %d Locate gives triangle %v with weights %v", s, w.Triangles[tri], wt)
		}
	}
}

func TestLocatorNearest(t *testing.T) {
	w := initialWorld(t, 2000)
	loc, err := yaee.NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(3, 4))
	for range 5000 {
		x, y, z := randomDirection(rng)
		want, best := -1, math.Inf(-1)
		for s, p := range w.Positions {
			if d := p[0]*x + p[1]*y + p[2]*z; d > best {
				want, best = s, d
			}
		}
		if got := loc.Nearest(x, y, z); got != want {
			t.Fatalf("Nearest(%v, %v, %v) = %d, want %d", x, y, z, got, want)
		}
	}
	for s, p := range w.Positions {
		if got := loc.Nearest(p[0], p[1], p[2]); got != s {
			t.Fatalf("Nearest at sample %d = %d", s, got)
		}
	}
}

func TestLocatorInvalidDirection(t *testing.T) {
	loc, err := yaee.NewLocator(initialWorld(t, 100))
	if err != nil {
		t.Fatal(err)
	}
	nan, inf := math.NaN(), math.Inf(1)
	for _, d := range [][3]float64{{0, 0, 0}, {nan, 0, 1}, {0, nan, 1}, {1, 0, nan}, {inf, 0, 0}, {0, -inf, 1}} {
		if tri, wt := loc.Locate(d[0], d[1], d[2]); tri != -1 || wt != [3]float64{} {
			t.Errorf("Locate(%v) = %d, %v, want -1 and zero weights", d, tri, wt)
		}
		if s := loc.Nearest(d[0], d[1], d[2]); s != -1 {
			t.Errorf("Nearest(%v) = %d, want -1", d, s)
		}
	}
	// The extremes of float64 are valid.
	for _, d := range [][3]float64{{math.MaxFloat64, math.MaxFloat64, 0}, {0, 0, -math.SmallestNonzeroFloat64}} {
		if tri, _ := loc.Locate(d[0], d[1], d[2]); tri < 0 {
			t.Errorf("Locate(%v) = %d", d, tri)
		}
		if s := loc.Nearest(d[0], d[1], d[2]); s < 0 {
			t.Errorf("Nearest(%v) = %d", d, s)
		}
	}
}

func TestNewLocatorInvalidWorld(t *testing.T) {
	if _, err := yaee.NewLocator(nil); !errors.Is(err, yaee.ErrInvalidWorld) {
		t.Errorf("nil world: got %v, want ErrInvalidWorld", err)
	}
	w := initialWorld(t, 100)
	w.Triangles = w.Triangles[1:]
	if _, err := yaee.NewLocator(w); !errors.Is(err, yaee.ErrInvalidWorld) {
		t.Errorf("a world missing a triangle: got %v, want ErrInvalidWorld", err)
	}
}

// A Locator keeps its own copy of the world.
func TestLocatorKeepsACopy(t *testing.T) {
	w := initialWorld(t, 100)
	loc, err := yaee.NewLocator(w)
	if err != nil {
		t.Fatal(err)
	}
	p := w.Positions[7]
	clear(w.Positions)
	clear(w.Triangles)
	if s := loc.Nearest(p[0], p[1], p[2]); s != 7 {
		t.Errorf("after the world was cleared, Nearest = %d, want 7", s)
	}
}

// Reading a world at a point of the caller's choosing: here latitude
// 40° north, longitude 20° east, taking the z axis as north.
func ExampleLocator() {
	cfg := yaee.DefaultConfig()
	cfg.Seed = 42
	cfg.SampleSpacing = 1000 // km; coarse, so that the example is quick
	cfg.Duration = 0
	world, err := yaee.Generate(context.Background(), cfg)
	if err != nil {
		fmt.Println(err)
		return
	}
	loc, err := yaee.NewLocator(world)
	if err != nil {
		fmt.Println(err)
		return
	}
	lat, lon := 40*math.Pi/180, 20*math.Pi/180
	x, y, z := math.Cos(lat)*math.Cos(lon), math.Cos(lat)*math.Sin(lon), math.Sin(lat)

	// A value that names a kind: take the nearest sample's.
	s := loc.Nearest(x, y, z)
	fmt.Println("nearest sample:", s, "crust type:", world.Attribute(yaee.AttrCrustType).Uint8s[s])

	// A value that varies continuously: interpolate.
	t, wt := loc.Locate(x, y, z)
	c := world.Triangles[t]
	elevation := world.Attribute(yaee.AttrElevation).Float64s
	fmt.Printf("triangle %d, samples %v, weights %.3f\n", t, c, wt)
	fmt.Printf("elevation: %.3f km\n", wt[0]*elevation[c[0]]+wt[1]*elevation[c[1]]+wt[2]*elevation[c[2]])

	// Output:
	// nearest sample: 102 crust type: 0
	// triangle 60, samples [115 81 102], weights [0.405 0.085 0.509]
	// elevation: -6.000 km
}
