// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"math"
	"reflect"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// at returns the unit position at the given longitude, in degrees from
// the meridian where the plates of halves converge, and latitude.
func at(lon, lat float64) sphere.Vec {
	lo, la := (lon+90)*math.Pi/180, lat*math.Pi/180
	return sphere.Vec{X: math.Cos(la) * math.Cos(lo), Y: math.Cos(la) * math.Sin(lo), Z: math.Sin(la)}
}

// within reports whether p is within the angle deg, in degrees, of c.
func within(p, c sphere.Vec, deg float64) bool {
	return sphere.Angle(p, c) <= deg*math.Pi/180
}

// continentalArea returns the area in km² of the continental crust of
// plate k, or of every plate if k is negative.
func continentalArea(w *model.World, k int) float64 {
	var sum float64
	for i, a := range w.Mesh.PointAreas() {
		if w.CrustType[i] == land && (k < 0 || int(w.Plate[i]) == k) {
			sum += a
		}
	}
	return sum * w.Radius * w.Radius
}

// island returns a world of two plates as halves makes them, at rest:
// plate 0, at positive longitudes, is ocean but for a round terrane of
// the radius deg degrees whose edge reaches the boundary on the equator,
// and plate 1 is all continental. It also returns the terrane's
// samples.
func island(t testing.TB, n int, deg float64) (*model.World, []int32) {
	t.Helper()
	w := halves(t, n, [2]uint8{ocean, land}, [2]float64{60, 0}, [2]float64{}, westEast)
	var terrane []int32
	for i, p := range w.Mesh.Points {
		if w.Plate[i] == 0 && within(p, at(deg, 0), deg) {
			w.CrustType[i], w.Thickness[i], w.Elevation[i], w.OceanicAge[i] = land, 35, landLevel, 0
			terrane = append(terrane, int32(i))
		}
	}
	if len(terrane) < 20 {
		t.Fatalf("the terrane has only %d samples", len(terrane))
	}
	return w, terrane
}

// contact returns the collision between the plates of island as
// Converge would report it: the positions of the terrane's samples that
// have a neighbour in plate 1.
func contact(w *model.World, terrane []int32, over, under int32) Collision {
	ev := Collision{Over: over, Under: under}
	for _, i := range terrane {
		for _, j := range w.Mesh.Neighbours(int(i)) {
			if w.Plate[j] == 1 {
				ev.At = append(ev.At, w.Mesh.Points[i])
				break
			}
		}
	}
	return ev
}

// Issue #27: the terranes of a constructed world are the regions
// constructed, in order of their lowest sample.
func TestTerranes(t *testing.T) {
	w := halves(t, 4000, [2]uint8{ocean, ocean}, [2]float64{60, 60}, [2]float64{}, westEast)
	// Three caps: one inside each plate, and one across the boundary,
	// which is two terranes.
	caps := []sphere.Vec{at(-60, 20), at(70, -30), at(0, 50)}
	want := map[[2]int][]int32{} // by cap and plate
	for i, p := range w.Mesh.Points {
		for c, centre := range caps {
			if within(p, centre, 12) {
				w.CrustType[i] = land
				key := [2]int{c, int(w.Plate[i])}
				want[key] = append(want[key], int32(i))
			}
		}
	}
	if len(want) != 4 {
		t.Fatalf("constructed %d regions, want 4", len(want))
	}
	areas := w.Mesh.PointAreas()
	terranes, of := Terranes(w, nil)
	if len(terranes) != 4 {
		t.Fatalf("found %d terranes, want 4", len(terranes))
	}
	var total [2]float64
	for n, tr := range terranes {
		if n > 0 && terranes[n-1].Samples[0] >= tr.Samples[0] {
			t.Errorf("terrane %d starts at sample %d, not after terrane %d", n, tr.Samples[0], n-1)
		}
		if !slices.IsSorted(tr.Samples) {
			t.Errorf("terrane %d: samples are not in order", n)
		}
		found := false
		for key, samples := range want {
			if slices.Equal(samples, tr.Samples) {
				found = true
				if int(tr.Plate) != key[1] {
					t.Errorf("terrane %d: plate %d, want %d", n, tr.Plate, key[1])
				}
				// A round terrane wholly inside a plate holds its centroid,
				// near the centre it was built about.
				if key[0] < 2 {
					if !slices.Contains(tr.Samples, w.Locator.Nearest(tr.Centroid)) {
						t.Errorf("terrane %d: the centroid is outside the terrane", n)
					}
					if a := sphere.Angle(tr.Centroid, caps[key[0]]) * 180 / math.Pi; a > 1 {
						t.Errorf("terrane %d: the centroid is %.2f° from the centre of its cap", n, a)
					}
				}
			}
		}
		if !found {
			t.Errorf("terrane %d (%d samples from %d) is not one of the regions constructed", n, len(tr.Samples), tr.Samples[0])
		}
		var area float64
		for _, i := range tr.Samples {
			area += areas[i]
			if of[i] != int32(n) {
				t.Fatalf("sample %d is in terrane %d but of gives %d", i, n, of[i])
			}
		}
		if math.Abs(tr.Area-area*w.Radius*w.Radius) > 1e-6*tr.Area {
			t.Errorf("terrane %d: area %v, want %v", n, tr.Area, area*w.Radius*w.Radius)
		}
		if math.Abs(tr.Centroid.Len()-1) > 1e-12 {
			t.Errorf("terrane %d: the centroid has length %v", n, tr.Centroid.Len())
		}
		total[tr.Plate] += tr.Area
	}
	for k := range total {
		if want := continentalArea(w, k); math.Abs(total[k]-want) > 1e-6*want {
			t.Errorf("plate %d: its terranes have area %v, its continental crust %v", k, total[k], want)
		}
	}
	for i, n := range of {
		if (n >= 0) != (w.CrustType[i] == land) {
			t.Fatalf("sample %d: terrane %d, crust type %d", i, n, w.CrustType[i])
		}
	}

	// The same world gives the same terranes.
	again, of2 := Terranes(w, areas)
	if !reflect.DeepEqual(terranes, again) || !slices.Equal(of, of2) {
		t.Error("two calls on the same world gave different terranes")
	}

	// A subducted sample is in no terrane.
	gone := terranes[0].Samples[0]
	w.Subducted[gone] = true
	if _, of := Terranes(w, areas); of[gone] != -1 {
		t.Errorf("a subducted sample is in terrane %d", of[gone])
	}
}

// Issue #27: one sample joins two regions into one terrane, and one
// oceanic sample between them keeps them two.
func TestTerranesJoinedByOneSample(t *testing.T) {
	w := halves(t, 2000, [2]uint8{ocean, ocean}, [2]float64{60, 60}, [2]float64{}, westEast)
	mesh := w.Mesh
	// A sample well inside plate 1, and two of its neighbours that are
	// not neighbours of each other.
	c := int(w.Locator.Nearest(at(-90, 0)))
	var a, b int32 = -1, -1
	for _, x := range mesh.Neighbours(c) {
		for _, y := range mesh.Neighbours(c) {
			if x != y && !slices.Contains(mesh.Neighbours(int(x)), y) {
				a, b = x, y
			}
		}
	}
	if a < 0 {
		t.Fatal("no two neighbours of the sample are apart")
	}
	w.CrustType[a], w.CrustType[b] = land, land
	if terranes, _ := Terranes(w, nil); len(terranes) != 2 {
		t.Fatalf("apart: %d terranes, want 2", len(terranes))
	}
	w.CrustType[c] = land
	terranes, _ := Terranes(w, nil)
	if len(terranes) != 1 || len(terranes[0].Samples) != 3 {
		t.Fatalf("joined: %d terranes, want 1 of 3 samples", len(terranes))
	}
	// The joining sample in another plate joins nothing.
	w.Plate[c] = 0
	if terranes, _ := Terranes(w, nil); len(terranes) != 3 {
		t.Fatalf("joined through another plate: %d terranes, want 3", len(terranes))
	}
}

// Issue #28: the radius of influence grows as the square root of the
// terrane's area and of the relative speed, up to r_c.
func TestCollisionRadius(t *testing.T) {
	p := earth()
	mean := 4 * math.Pi * p.Radius * p.Radius / float64(p.Plates)
	for _, c := range []struct {
		name    string
		v, area float64
		want    float64
	}{
		{"a mean plate at v0", 100, mean, 4200},
		{"a quarter of the area", 100, mean / 4, 2100},
		{"a quarter of the speed", 25, mean, 2100},
		{"a sixteenth of both together", 25, mean / 4, 1050},
		{"limited to r_c", 200, 3 * mean, 4200},
		{"no relative speed", 0, mean, 0},
		{"no area", 100, 0, 0},
	} {
		if got := CollisionRadius(p, c.v, c.area); math.Abs(got-c.want) > 1e-9 {
			t.Errorf("%s: r = %v, want %v", c.name, got, c.want)
		}
	}
	p.MaxPlateSpeed = 0
	if got := CollisionRadius(p, 50, mean); got != 0 {
		t.Errorf("v0 = 0: r = %v, want 0", got)
	}
}

// Issue #28: the surge is Δ_c · 𝒜 at the terrane, 0 at r and beyond.
func TestCollisionSurge(t *testing.T) {
	p := earth()
	const area, r = 2e5, 800.0
	peak := p.CollisionCoefficient * area // 2.6 km
	for _, c := range []struct{ d, want float64 }{
		{0, peak}, {r / 2, peak * 0.5625}, {r, 0}, {r + 1, 0}, {10 * r, 0},
	} {
		if got := CollisionSurge(p, area, c.d, r); math.Abs(got-c.want) > 1e-12 {
			t.Errorf("d = %v: surge %v, want %v", c.d, got, c.want)
		}
	}
	if got := CollisionSurge(p, area, 0, 0); got != 0 {
		t.Errorf("r = 0: surge %v, want 0", got)
	}
	for d := 0.0; d < r; d += 10 {
		if CollisionSurge(p, area, d+10, r) > CollisionSurge(p, area, d, r) {
			t.Fatalf("the surge grows with distance at %v km", d)
		}
	}
}

// Issue #28: no surge takes crust above z_c, and a small one is nearly
// the paper's.
func TestRaise(t *testing.T) {
	p := earth()
	zc := p.MaxContinentalElevation
	if got := Raise(p, 0, 0.01, 0.01); math.Abs(got-0.01) > 1e-5 {
		t.Errorf("a surge of 0.01 km from sea level gives %v", got)
	}
	if got, want := Raise(p, 0, 13, 13), zc*(1-math.Exp(-1.3)); math.Abs(got-want) > 1e-12 {
		t.Errorf("a surge of 13 km from sea level gives %v, want %v", got, want)
	}
	// Half the peak gives half the rise: the profile keeps its shape.
	if got, want := Raise(p, 0, 6.5, 13), zc*(1-math.Exp(-1.3))/2; math.Abs(got-want) > 1e-12 {
		t.Errorf("half a surge of 13 km gives %v, want %v", got, want)
	}
	if got := Raise(p, -2, 1, 1); !(got > -2 && got < -1) {
		t.Errorf("a surge of 1 km from −2 km gives %v", got)
	}
	for _, z := range []float64{-5, -0.1, 0, 0.5, 4, 9.9, zc} {
		last := z
		for _, peak := range []float64{0.001, 0.1, 1, 10, 100, 1e4} {
			for _, share := range []float64{0.01, 0.5, 1} {
				got := Raise(p, z, share*peak, peak)
				if got < z || got > zc {
					t.Fatalf("Raise(%v, %v, %v) = %v", z, share*peak, peak, got)
				}
			}
			if got := Raise(p, z, peak, peak); got < last {
				t.Fatalf("a greater surge raises %v km less: %v after %v", z, got, last)
			} else {
				last = got
			}
		}
	}
	for _, dz := range []float64{0, -1, math.NaN()} {
		if got := Raise(p, 3, dz, 5); got != 3 {
			t.Errorf("Raise(3, %v, 5) = %v, want 3", dz, got)
		}
	}
	if got := Raise(p, 12, 5, 5); got != 12 {
		t.Errorf("crust above z_c was moved to %v", got)
	}
}

// Issue #28: the fold direction is tangent to the sphere and points away
// from the terrane's centroid.
func TestCollisionFold(t *testing.T) {
	q := at(0, 0)
	for _, p := range []sphere.Vec{at(10, 0), at(-25, 30), at(3, -70), at(120, 5)} {
		f := CollisionFold(p, q)
		if math.Abs(f.Dot(p)) > 1e-12 {
			t.Errorf("at %v: the fold direction is not tangent to the sphere", p)
		}
		// Along the great circle from q to p, away from q.
		away := q.Scale(-1).Tangent(p).Normalize()
		if f.Normalize().Sub(away).Len() > 1e-9 {
			t.Errorf("at %v: the fold direction %v does not point away from the centroid (%v)", p, f.Normalize(), away)
		}
		if want := math.Cos(sphere.Angle(p, q) / 2); math.Abs(f.Len()-want) > 1e-12 {
			t.Errorf("at %v: length %v, want %v", p, f.Len(), want)
		}
	}
	if f := CollisionFold(q, q); f != (sphere.Vec{}) {
		t.Errorf("at the centroid: %v, want zero", f)
	}
}

// Issue #28: Collide moves the smaller terrane, whichever plate was on
// top, and raises the land on both sides of the suture by the documented
// amounts and no further than r.
func TestCollide(t *testing.T) {
	for _, order := range [][2]int32{{0, 1}, {1, 0}} {
		w, terrane := island(t, 12000, 8)
		w.PlateSpeed = []float64{0.004, 0.004} // 25 km/My each at the equator
		s := NewSubduction(w)
		spacing := sphere.SampleSpacing(w.Radius, len(w.Mesh.Points))
		before := continentalArea(w, -1)
		terranes, _ := Terranes(w, nil)
		tr := terranes[slices.IndexFunc(terranes, func(tr Terrane) bool { return tr.Plate == 0 })]
		if !slices.Equal(tr.Samples, terrane) {
			t.Fatal("the terrane is not the one constructed")
		}
		r := CollisionRadius(w.Params, RelativeSpeed(w, 0, 1, tr.Centroid), tr.Area)
		peak := w.CollisionCoefficient * tr.Area
		if r < 6*spacing || peak < 1 {
			t.Fatalf("r = %.0f km and the surge %.2f km are too small to test", r, peak)
		}
		wasOne := slices.Clone(w.Plate)

		if moved := s.Collide(w, []Collision{contact(w, terrane, order[0], order[1])}); moved != 1 {
			t.Fatalf("order %v: %d terranes moved, want 1", order, moved)
		}
		if got := continentalArea(w, -1); got != before {
			t.Errorf("order %v: continental area %v, was %v", order, got, before)
		}
		if got := continentalArea(w, 0); got != 0 {
			t.Errorf("order %v: plate 0 still has %v km² of continental crust", order, got)
		}
		if !slices.Contains(w.Plate, 0) {
			t.Errorf("order %v: the oceanic remainder of plate 0 is gone", order)
		}
		// The terrane now moves with plate 1.
		for _, i := range terrane {
			if w.Plate[i] != 1 {
				t.Fatalf("order %v: sample %d of the terrane is in plate %d", order, i, w.Plate[i])
			}
		}
		if w.PlateSpeed[0] != 0.004 {
			t.Errorf("order %v: plate 0 changed speed", order)
		}

		// Distances by brute force.
		var suture []int32 // samples of plate 1 beside the terrane
		for _, i := range terrane {
			for _, j := range w.Mesh.Neighbours(int(i)) {
				if wasOne[j] == 1 {
					suture = append(suture, j)
				}
			}
		}
		nearest := func(p sphere.Vec, set []int32) float64 {
			d := math.Inf(1)
			for _, j := range set {
				d = min(d, sphere.Angle(p, w.Mesh.Points[j])*w.Radius)
			}
			return d
		}
		var raised, furthest, highest float64
		for i, p := range w.Mesh.Points {
			if w.Plate[i] != 1 {
				if w.OrogenyType[i] != model.OrogenyNone || w.Elevation[i] != oceanFloor {
					t.Fatalf("order %v: sample %d of plate 0 was changed", order, i)
				}
				continue
			}
			set := terrane
			if wasOne[i] == 0 {
				set = suture
			}
			d := nearest(p, set)
			want := Raise(w.Params, landLevel, CollisionSurge(w.Params, tr.Area, d, r), peak)
			// A sample's distance is to a sample half a spacing either way.
			slack := Raise(w.Params, landLevel, CollisionSurge(w.Params, tr.Area, max(0, d-spacing/2), r), peak) - want + 1e-9
			if got := w.Elevation[i]; math.Abs(got-want) > slack {
				t.Fatalf("order %v: sample %d (was plate %d, lon %.1f lat %.1f), %.0f km away, r %.0f: elevation %v, want %v ± %v", order, i, wasOne[i], longitude(p), math.Asin(p.Z)*180/math.Pi, d, r, got, want, slack)
			}
			if w.Elevation[i] > landLevel {
				raised++
				furthest = max(furthest, d)
				highest = max(highest, w.Elevation[i])
				if w.OrogenyType[i] != model.OrogenyHimalayan || w.OrogenyAge[i] != 0 {
					t.Fatalf("order %v: sample %d was raised without a Himalayan orogeny of age 0", order, i)
				}
				if f := w.FoldDirection[i]; f != CollisionFold(p, tr.Centroid) {
					t.Fatalf("order %v: sample %d has fold direction %v", order, i, f)
				}
			} else if w.OrogenyType[i] != model.OrogenyNone {
				t.Fatalf("order %v: sample %d has an orogeny and was not raised", order, i)
			}
			if w.Elevation[i] > w.MaxContinentalElevation {
				t.Fatalf("order %v: sample %d is above z_c", order, i)
			}
		}
		if furthest > r || furthest < r-2*spacing {
			t.Errorf("order %v: land was raised up to %.0f km away, want just under r = %.0f km", order, furthest, r)
		}
		// Beside the suture the rise is the whole surge, limited.
		if want := Raise(w.Params, landLevel, peak, peak); highest > want || highest < want-0.05*peak {
			t.Errorf("order %v: the highest land is at %v km, want just under %v", order, highest, want)
		}
		if raised < 50 {
			t.Errorf("order %v: only %v samples were raised", order, raised)
		}
	}
}

// Issue #28: a faster or larger collision reaches further.
func TestCollideReach(t *testing.T) {
	reach := func(deg, omega float64) (count int) {
		w, terrane := island(t, 6000, deg)
		w.PlateSpeed = []float64{omega, omega}
		NewSubduction(w).Collide(w, []Collision{contact(w, terrane, 0, 1)})
		for i := range w.OrogenyType {
			if w.OrogenyType[i] == model.OrogenyHimalayan && !slices.Contains(terrane, int32(i)) {
				count++
			}
		}
		return count
	}
	base, faster, larger, still := reach(8, 0.002), reach(8, 0.008), reach(12, 0.002), reach(8, 0)
	if !(faster > 2*base) || !(larger > base) {
		t.Errorf("samples raised: %d; %d at four times the speed; %d for a larger terrane", base, faster, larger)
	}
	if still != 0 {
		t.Errorf("plates at rest raised %d samples", still)
	}
}

// Issue #28: a lake of the terrane's plate inside the terrane goes with
// it, and a collision where one plate has no continental crust left at
// the contact does nothing.
func TestCollideEnclosedAndConsumed(t *testing.T) {
	w, terrane := island(t, 6000, 10)
	w.PlateSpeed = []float64{0.004, 0.004}
	lake := w.Locator.Nearest(at(10, 0))
	w.CrustType[lake], w.Elevation[lake], w.Thickness[lake] = ocean, oceanFloor, 7
	terrane = slices.DeleteFunc(terrane, func(i int32) bool { return i == lake })
	s := NewSubduction(w)
	ev := contact(w, terrane, 0, 1)
	if moved := s.Collide(w, []Collision{ev}); moved != 1 {
		t.Fatalf("%d terranes moved, want 1", moved)
	}
	if w.Plate[lake] != 1 {
		t.Error("the lake inside the terrane stayed with plate 0")
	}
	if w.CrustType[lake] != ocean || w.Elevation[lake] != oceanFloor {
		t.Error("the lake's crust was changed")
	}

	// Again: plate 0 now has no continental crust at the contact.
	plates, heights := slices.Clone(w.Plate), slices.Clone(w.Elevation)
	if moved := s.Collide(w, []Collision{ev}); moved != 0 {
		t.Fatalf("a collision with nothing to move moved %d terranes", moved)
	}
	if !slices.Equal(plates, w.Plate) || !slices.Equal(heights, w.Elevation) {
		t.Error("a collision with nothing to move changed the world")
	}
}

// collisions carries the world through n steps as Simulation does, resampling
// and colliding in each step that has a collision. It returns the step,
// counted from 1, of each collision that moved a terrane.
func collisions(t *testing.T, s *Subduction, w *model.World, n int, dt float64, each func(step int)) (at []int) {
	t.Helper()
	var elapsed float64
	for step := 1; step <= n; step++ {
		if err := s.Step(t.Context(), w, dt); err != nil {
			t.Fatal(err)
		}
		elapsed += dt
		if each != nil {
			each(step)
		}
		if pending := s.Pending(); len(pending) > 0 {
			if err := Resample(t.Context(), w, elapsed); err != nil {
				t.Fatal(err)
			}
			area := continentalArea(w, -1)
			if s.Collide(w, pending) > 0 {
				at = append(at, step)
			}
			if got := continentalArea(w, -1); got != area {
				t.Fatalf("step %d: the collision changed the continental area from %v to %v", step, area, got)
			}
			s.Reset(w)
			elapsed = 0
		}
	}
	return at
}

// Issue #28: a terrane carried into a continent changes plates once, and
// only after the continents have closed the threshold distance.
func TestCollisionAfterPenetration(t *testing.T) {
	const omega, dt = 0.004, 2.0 // closing at 51 km/My on the equator
	w, terrane := island(t, 12000, 8)
	w.PlateSpeed = []float64{omega, omega}
	s := NewSubduction(w)
	closing := 2 * omega * w.Radius
	var before []float64 // the interpenetration after each step until the collision
	collided := false
	when := collisions(t, s, w, 30, dt, func(step int) {
		if len(s.Pending()) > 0 {
			collided = true
		}
		if !collided {
			before = append(before, s.Penetration(0, 1))
			if continentalArea(w, 0) == 0 {
				t.Fatalf("step %d: the terrane left plate 0 before any collision", step)
			}
		}
		for i, z := range w.Elevation {
			if z > w.MaxContinentalElevation {
				t.Fatalf("step %d: sample %d is at %v km, above z_c", step, i, z)
			}
		}
	})
	if len(when) != 1 {
		t.Fatalf("the terrane changed plates %d times (steps %v), want once", len(when), when)
	}
	// Every step before the collision left the distance under the
	// threshold, and the distance grew by the closing speed.
	for k, d := range before {
		if d >= w.CollisionPenetration {
			t.Errorf("step %d: interpenetration %v km without a collision", k+1, d)
		}
		if want := float64(k+1) * closing * dt; d > want+1e-9 || d < want-1.5*closing*dt {
			t.Errorf("step %d: interpenetration %v km, want about %v", k+1, d, want)
		}
	}
	if want := int(math.Ceil(w.CollisionPenetration / (closing * dt))); when[0] < want || when[0] > want+1 {
		t.Errorf("the collision came in step %d, want %d or %d", when[0], want, want+1)
	}
	if s.Penetration(0, 1) != 0 {
		t.Errorf("the interpenetration is %v after the collision, want 0", s.Penetration(0, 1))
	}
	if got := continentalArea(w, 0); got != 0 {
		t.Errorf("plate 0 has %v km² of continental crust after the collision", got)
	}
	if !slices.Contains(w.Plate, 0) {
		t.Error("the oceanic remainder of plate 0 is gone")
	}
	himalayan := 0
	for i := range w.OrogenyType {
		if w.OrogenyType[i] == model.OrogenyHimalayan {
			himalayan++
		}
	}
	if himalayan < 50 {
		t.Errorf("%d samples have a Himalayan orogeny", himalayan)
	}
	_ = terrane
}

// An ocean converging on a continent is subduction, not collision, and
// continents that part stop being tracked.
func TestNoCollisionWithoutContinents(t *testing.T) {
	w := halves(t, 4000, [2]uint8{ocean, land}, [2]float64{60, 0}, [2]float64{0.004, 0.004}, westEast)
	s := NewSubduction(w)
	if when := collisions(t, s, w, 12, 2, func(int) {
		if s.Penetration(0, 1) != 0 {
			t.Fatal("ocean beneath a continent was tracked as a collision")
		}
	}); len(when) != 0 {
		t.Fatalf("collisions in steps %v", when)
	}

	w = halves(t, 4000, [2]uint8{land, land}, [2]float64{}, [2]float64{0.004, 0.004}, westEast)
	w.CollisionPenetration = 1e9
	s = NewSubduction(w)
	steps(t, s, w, 2, 2)
	if d := s.Penetration(0, 1); !(d > 50) {
		t.Fatalf("two converging continents closed %v km in two steps", d)
	}
	// Turn both about the axis at right angles to the boundary, so that
	// they slide along it and nothing converges.
	w.PlateAxis = []sphere.Vec{{X: 1}, {X: -1}}
	if err := Resample(t.Context(), w, 4); err != nil {
		t.Fatal(err)
	}
	s.Reset(w)
	steps(t, s, w, 2*int(resampleDistance/(0.004*w.Radius*2)), 2)
	if d := s.Penetration(0, 1); d != 0 {
		t.Errorf("the interpenetration is %v km after the continents stopped converging, want 0", d)
	}
}
