// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package tectonics

import (
	"math"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/maloquacious/yaee/internal/model"
	"github.com/maloquacious/yaee/internal/sphere"
)

// caps returns a world of n samples with one plate per centre, each a
// cap of the given angle in degrees about its centre, and a last plate
// that holds the rest of the sphere. crust[k] is the crust of plate k;
// the last plate is oceanic. No plate moves.
func caps(t testing.TB, n int, centres []sphere.Vec, deg []float64, crust []uint8) *model.World {
	t.Helper()
	w := newWorld(t, n, earth())
	w.NumPlates = len(centres) + 1
	w.PlateAxis = make([]sphere.Vec, w.NumPlates)
	w.PlateSpeed = make([]float64, w.NumPlates)
	w.PlateRotation = make([]sphere.Rotation, w.NumPlates)
	for k := range w.NumPlates {
		w.PlateAxis[k], w.PlateRotation[k] = sphere.Vec{Z: 1}, sphere.Identity()
	}
	for i, p := range w.Mesh.Points {
		k := len(centres)
		for c := range centres {
			if within(p, centres[c], deg[c]) {
				k = c
				break
			}
		}
		w.Plate[i] = int32(k)
		w.CrustType[i], w.Thickness[i], w.Elevation[i], w.OceanicAge[i] = ocean, 7, oceanFloor, 50
		if k < len(centres) && crust[k] == land {
			w.CrustType[i], w.Thickness[i], w.Elevation[i], w.OceanicAge[i] = land, 35, landLevel, 0
		}
	}
	return w
}

func TestRiftRate(t *testing.T) {
	p := earth()
	p.RiftingRate = 0.01
	a0 := 4 * math.Pi * p.Radius * p.Radius / float64(p.Plates)

	if got := RiftTransfer(1); got != 1 {
		t.Errorf("RiftTransfer(1) = %v, want 1", got)
	}
	if got := RiftTransfer(0); got != riftOceanic {
		t.Errorf("RiftTransfer(0) = %v, want %v", got, riftOceanic)
	}
	if lo, hi := RiftTransfer(0.3), RiftTransfer(0.6); !(lo < hi) {
		t.Errorf("RiftTransfer does not grow with the continental share: %v, %v", lo, hi)
	}

	// A wholly continental plate of the mean initial area rifts at λ_0.
	if got := RiftRate(p, a0, a0); math.Abs(got-0.01) > 1e-15 {
		t.Errorf("rate of a continental plate of area a0 = %v, want 0.01", got)
	}
	// The rate is proportional to the area.
	if got := RiftRate(p, 3*a0, 3*a0); math.Abs(got-0.03) > 1e-15 {
		t.Errorf("rate of a continental plate of area 3·a0 = %v, want 0.03", got)
	}
	if got := RiftRate(p, 2*a0, 0); math.Abs(got-0.02*riftOceanic) > 1e-15 {
		t.Errorf("rate of an oceanic plate of area 2·a0 = %v, want %v", got, 0.02*riftOceanic)
	}
	// Small plates and empty plates do not rift.
	for _, area := range []float64{0, 0.99 * riftMinArea * a0} {
		if got := RiftRate(p, area, area); got != 0 {
			t.Errorf("rate of a plate of area %v = %v, want 0", area, got)
		}
	}
	p.RiftingRate = 0
	if got := RiftRate(p, 5*a0, 5*a0); got != 0 {
		t.Errorf("rate with RiftingRate 0 = %v, want 0", got)
	}

	// The probability of a rift in a step.
	if got := RiftProbability(0, 2); got != 0 {
		t.Errorf("RiftProbability(0, 2) = %v, want 0", got)
	}
	if got, want := RiftProbability(0.25, 2), 1-math.Exp(-0.5); math.Abs(got-want) > 1e-15 {
		t.Errorf("RiftProbability(0.25, 2) = %v, want %v", got, want)
	}
	// Two half steps are one step.
	half := RiftProbability(0.01, 1)
	if got, want := 1-(1-half)*(1-half), RiftProbability(0.01, 2); math.Abs(got-want) > 1e-15 {
		t.Errorf("two steps of 1 My rift with probability %v, one of 2 My with %v", got, want)
	}
}

// The number of rifts per My matches the rate at two time steps, and a
// larger or more continental plate rifts more often.
func TestDrawRiftsMatchesRate(t *testing.T) {
	// Plate 0: a large continent. Plate 1: a small continent. Plate 2:
	// an oceanic plate the size of plate 0. Plate 3: too small to rift.
	// Plate 4: the rest, oceanic.
	centres := []sphere.Vec{at(0, 0), at(90, 0), at(180, 0), at(-90, 0)}
	w := caps(t, 4000, centres, []float64{40, 25, 40, 8}, []uint8{land, land, ocean, land})
	w.RiftingRate = 0.004
	areas := w.Mesh.PointAreas()
	area, continental := PlateAreas(w, areas)
	rate := make([]float64, w.NumPlates)
	for k := range rate {
		rate[k] = RiftRate(w.Params, area[k], continental[k])
	}
	if rate[3] != 0 {
		t.Fatalf("the small plate has rate %v, want 0", rate[3])
	}
	if !(rate[0] > rate[1] && rate[1] > 0 && rate[0] > rate[2] && rate[2] > 0) {
		t.Fatalf("rates %v: want the large continent above the small one and above the ocean of its size", rate)
	}

	const span = 40000.0 // My
	for _, dt := range []float64{2, 0.5} {
		rng := rand.New(rand.NewPCG(7, uint64(dt*8)))
		count := make([]float64, w.NumPlates)
		for range int(span / dt) {
			for _, k := range DrawRifts(w, areas, rng, dt) {
				count[k]++
			}
		}
		var total, want float64
		for k, c := range count {
			total += c
			want += rate[k] * span
			// Within four standard deviations of a Poisson count, and
			// the 1% by which at most one rift a step lowers it.
			if expect := rate[k] * span; math.Abs(c-expect) > 4*math.Sqrt(expect)+0.01*expect {
				t.Errorf("dt %v: plate %d rifted %v times in %v My, want about %.1f", dt, k, c, span, expect)
			}
		}
		if math.Abs(total-want) > 4*math.Sqrt(want)+0.01*want {
			t.Errorf("dt %v: %v rifts in %v My, want about %.1f", dt, total, span, want)
		}
		if !(count[0] > count[1] && count[0] > count[2]) {
			t.Errorf("dt %v: counts %v: the large continental plate did not rift most often", dt, count)
		}
	}

	// Nothing rifts, and nothing is drawn, with a rate or a speed of 0.
	for _, set := range []func(){func() { w.RiftingRate = 0 }, func() { w.RiftingRate, w.MaxPlateSpeed = 0.004, 0 }} {
		set()
		if got := DrawRifts(w, areas, nil, 1e6); got != nil {
			t.Errorf("DrawRifts = %v with RiftingRate %v and MaxPlateSpeed %v, want none", got, w.RiftingRate, w.MaxPlateSpeed)
		}
	}
}

// connected reports whether the samples of plate k are one region.
func connected(w *model.World, k int32) bool {
	start, total := -1, 0
	for i, p := range w.Plate {
		if p == k {
			total++
			if start < 0 {
				start = i
			}
		}
	}
	if total == 0 {
		return false
	}
	seen := map[int32]bool{int32(start): true}
	queue := []int32{int32(start)}
	for head := 0; head < len(queue); head++ {
		for _, j := range w.Mesh.Neighbours(int(queue[head])) {
			if w.Plate[j] == k && !seen[j] {
				seen[j] = true
				queue = append(queue, j)
			}
		}
	}
	return len(queue) == total
}

func TestRift(t *testing.T) {
	counts := map[int]int{}
	for seed := range uint64(40) {
		w := caps(t, 6000, []sphere.Vec{at(20, 10), at(-120, -30)}, []float64{35, 30}, []uint8{land, ocean})
		decorate(w)
		areas := w.Mesh.PointAreas()
		before := snap(w)
		plates := slices.Clone(w.Plate)
		parent := w.PlateAxis[0]

		n, err := Rift(t.Context(), w, areas, 0, rand.New(rand.NewPCG(seed, 99)))
		if err != nil {
			t.Fatal(err)
		}
		counts[n]++
		if n < riftMinFragments || n > riftMaxFragments {
			t.Fatalf("seed %d: %d fragments", seed, n)
		}
		if w.NumPlates != 3+n-1 || len(w.PlateAxis) != w.NumPlates || len(w.PlateSpeed) != w.NumPlates ||
			len(w.PlateRotation) != w.NumPlates || len(w.PlateOceanAge) != w.NumPlates {
			t.Fatalf("seed %d: %d fragments, NumPlates %d, %d axes, %d speeds, %d rotations, %d ocean ages", seed, n,
				w.NumPlates, len(w.PlateAxis), len(w.PlateSpeed), len(w.PlateRotation), len(w.PlateOceanAge))
		}

		// The fragments hold exactly the samples of the plate, under
		// the plate's number and the new numbers.
		fragment := func(k int32) bool { return k == 0 || k >= 3 }
		size := make([]int, w.NumPlates)
		for i, k := range w.Plate {
			if (plates[i] == 0) != fragment(k) || plates[i] != 0 && k != plates[i] {
				t.Fatalf("seed %d: sample %d went from plate %d to plate %d", seed, i, plates[i], k)
			}
			size[k]++
		}
		for k := range int32(w.NumPlates) {
			if size[k] == 0 {
				t.Fatalf("seed %d: plate %d has no sample", seed, k)
			}
			if fragment(k) && !connected(w, k) {
				t.Fatalf("seed %d: fragment %d is not one connected region", seed, k)
			}
		}

		// No crust attribute changed.
		after := snap(w)
		after.plate, after.axis, after.speed = before.plate, before.axis, before.speed
		if !after.equal(before) {
			t.Fatalf("seed %d: the rift changed the crust", seed)
		}

		// Motion: one angular speed, no faster than v_0, and every
		// fragment's centroid moves straight away from the plate's.
		var g sphere.Vec
		centroid := make([]sphere.Vec, w.NumPlates)
		for i, p := range w.Mesh.Points {
			if fragment(w.Plate[i]) {
				g = g.Add(p.Scale(areas[i]))
				centroid[w.Plate[i]] = centroid[w.Plate[i]].Add(p.Scale(areas[i]))
			}
		}
		g = g.Normalize()
		omega := w.PlateSpeed[0]
		if !(omega >= 0 && omega*w.Radius <= w.MaxPlateSpeed) {
			t.Fatalf("seed %d: angular speed %v is %v km/My at most, limit %v", seed, omega, omega*w.Radius, w.MaxPlateSpeed)
		}
		for k := range int32(w.NumPlates) {
			if !fragment(k) {
				continue
			}
			c := centroid[k].Normalize()
			centroid[k] = c
			if w.PlateSpeed[k] != omega || w.PlateRotation[k] != sphere.Identity() {
				t.Fatalf("seed %d: fragment %d has speed %v and rotation %v, want %v and the identity", seed, k,
					w.PlateSpeed[k], w.PlateRotation[k], omega)
			}
			if math.Abs(w.PlateAxis[k].Len()-1) > 1e-12 || w.PlateAxis[k] == parent {
				t.Fatalf("seed %d: fragment %d has axis %v", seed, k, w.PlateAxis[k])
			}
			away := c.Sub(g).Tangent(c).Normalize()
			v := Velocity(w, int(k), c)
			if omega > 0 && v.Dot(away) < 0.999*v.Len() {
				t.Fatalf("seed %d: fragment %d moves along %v at its centroid, away from the plate's centre is %v", seed, k, v, away)
			}
		}
		if omega == 0 {
			continue
		}

		// Adjacent fragments move apart: across every boundary, the
		// velocity of one relative to the other points from the
		// other's centroid toward its own.
		type pair struct{ a, b int32 }
		opening := map[pair][2]int{}
		for i, p := range w.Mesh.Points {
			a := w.Plate[i]
			if !fragment(a) {
				continue
			}
			for _, j := range w.Mesh.Neighbours(i) {
				b := w.Plate[j]
				if !fragment(b) || b <= a {
					continue
				}
				mid := p.Add(w.Mesh.Points[j]).Normalize()
				rel := Velocity(w, int(a), mid).Sub(Velocity(w, int(b), mid))
				o := opening[pair{a, b}]
				if rel.Dot(centroid[a].Sub(centroid[b])) > 0 {
					o[0]++
				}
				o[1]++
				opening[pair{a, b}] = o
			}
		}
		if len(opening) == 0 {
			t.Fatalf("seed %d: no two fragments are adjacent", seed)
		}
		for pr, o := range opening {
			if o[0] != o[1] {
				t.Errorf("seed %d: fragments %d and %d move apart at %d of the %d places on their boundary", seed, pr.a, pr.b, o[0], o[1])
			}
		}
	}
	for n := riftMinFragments; n <= riftMaxFragments; n++ {
		if counts[n] == 0 {
			t.Errorf("no rift of 40 gave %d fragments: %v", n, counts)
		}
	}
}

func TestRiftIsRepeatable(t *testing.T) {
	var first []int32
	for range 2 {
		w := caps(t, 3000, []sphere.Vec{at(0, 0)}, []float64{50}, []uint8{land})
		if _, err := Rift(t.Context(), w, w.Mesh.PointAreas(), 0, rand.New(rand.NewPCG(3, 4))); err != nil {
			t.Fatal(err)
		}
		if first == nil {
			first = slices.Clone(w.Plate)
		} else if !slices.Equal(first, w.Plate) {
			t.Fatal("the same random stream gave different fragments")
		}
	}
}

// A plate of one sample does not rift, and a plate of two or three
// samples rifts into no more fragments than it has samples.
func TestRiftSmallPlate(t *testing.T) {
	for size := 1; size <= 3; size++ {
		for seed := range uint64(10) {
			w := caps(t, 500, nil, nil, nil)
			w.NumPlates = 2
			w.PlateAxis = append(w.PlateAxis, sphere.Vec{Z: 1})
			w.PlateSpeed = append(w.PlateSpeed, 0)
			w.PlateRotation = append(w.PlateRotation, sphere.Identity())
			w.Plate[100] = 1
			for _, j := range w.Mesh.Neighbours(100)[:size-1] {
				w.Plate[j] = 1
			}
			n, err := Rift(t.Context(), w, w.Mesh.PointAreas(), 1, rand.New(rand.NewPCG(seed, 1)))
			if err != nil {
				t.Fatal(err)
			}
			if size == 1 && (n != 0 || w.NumPlates != 2) {
				t.Fatalf("a plate of one sample rifted into %d", n)
			}
			if size > 1 && (n < 2 || n > size || w.NumPlates != 2+n-1) {
				t.Fatalf("a plate of %d samples rifted into %d, NumPlates %d", size, n, w.NumPlates)
			}
			used := make([]int, w.NumPlates)
			for _, k := range w.Plate {
				used[k]++
			}
			if slices.Contains(used, 0) {
				t.Fatalf("size %d: samples per plate %v", size, used)
			}
		}
	}
}

func TestRemoveEmptyPlates(t *testing.T) {
	w := caps(t, 2000, []sphere.Vec{at(0, 0), at(90, 0), at(180, 0)}, []float64{20, 20, 20}, []uint8{land, ocean, land})
	for k := range w.NumPlates {
		w.PlateSpeed[k] = float64(k + 1)
		w.PlateAxis[k] = at(float64(10*k), 0)
	}
	w.PlateOceanAge = PlateOceanAges(w)

	// Nothing to remove.
	remap := RemoveEmptyPlates(w)
	if !slices.Equal(remap, []int32{0, 1, 2, 3}) || w.NumPlates != 4 {
		t.Fatalf("with no empty plate: remap %v, NumPlates %d", remap, w.NumPlates)
	}

	// Empty plates 0 and 2: plate 1 becomes 0, and plate 3 becomes 1.
	s := NewSubduction(w)
	s.Converge(w, 2)
	s.closed[1*4+3], s.closed[3*4+1], s.closed[2*4+3] = 111, 222, 333
	old := slices.Clone(w.Plate)
	for i, k := range w.Plate {
		if k == 0 || k == 2 {
			w.Plate[i] = 3
		}
	}
	remap = RemoveEmptyPlates(w)
	if !slices.Equal(remap, []int32{-1, 0, -1, 1}) {
		t.Fatalf("remap %v, want [-1 0 -1 1]", remap)
	}
	if w.NumPlates != 2 || len(w.PlateAxis) != 2 || len(w.PlateSpeed) != 2 || len(w.PlateRotation) != 2 || len(w.PlateOceanAge) != 2 {
		t.Fatalf("NumPlates %d, %d axes, %d speeds, %d rotations, %d ocean ages; want 2 of each", w.NumPlates,
			len(w.PlateAxis), len(w.PlateSpeed), len(w.PlateRotation), len(w.PlateOceanAge))
	}
	if w.PlateSpeed[0] != 2 || w.PlateSpeed[1] != 4 || w.PlateAxis[0] != at(10, 0) || w.PlateAxis[1] != at(30, 0) {
		t.Fatalf("the plates' movements did not follow them: speeds %v, axes %v", w.PlateSpeed, w.PlateAxis)
	}
	for i, k := range w.Plate {
		want := int32(1)
		if old[i] == 1 {
			want = 0
		}
		if k != want {
			t.Fatalf("sample %d of plate %d is now in plate %d, want %d", i, old[i], k, want)
		}
	}

	// The collision state follows the plates.
	s.Renumber(w, remap)
	s.Reset(w)
	if len(s.front) != 2 || len(s.closed) != 4 {
		t.Fatalf("after Renumber: %d plates and %d pairs, want 2 and 4", len(s.front), len(s.closed))
	}
	if s.closed[0*2+1] != 111 || s.closed[1*2+0] != 222 || s.closed[0] != 0 || s.closed[3] != 0 {
		t.Fatalf("after Renumber the interpenetration distances are %v, want [0 111 222 0]", s.closed)
	}
	if err := s.Step(t.Context(), w, 2); err != nil {
		t.Fatal(err)
	}
}

// A simulation with frequent rifts runs, splits plates, opens ocean
// between the fragments, leaves no plate number unused, and is
// repeatable.
func TestSimulateWithRifts(t *testing.T) {
	var first snapshot
	for round := range 2 {
		params := earth()
		params.Plates, params.RiftingRate, params.Duration = 6, 0.02, 60
		w := moving(t, 3000, 5, params)
		before := w.NumPlates
		var continental int
		for _, c := range w.CrustType {
			continental += int(c)
		}
		n, err := simulate(t.Context(), w, model.Stream(5, "simulation"), resampleDistance)
		if err != nil {
			t.Fatal(err)
		}
		if n == 0 {
			t.Fatal("no resampling")
		}
		used := make([]bool, w.NumPlates)
		for i, k := range w.Plate {
			if k < 0 || int(k) >= w.NumPlates {
				t.Fatalf("sample %d is in plate %d of %d", i, k, w.NumPlates)
			}
			used[k] = true
		}
		if slices.Contains(used, false) {
			t.Fatalf("a plate number is unused: %v", used)
		}
		if len(w.PlateAxis) != w.NumPlates || len(w.PlateSpeed) != w.NumPlates || len(w.PlateRotation) != w.NumPlates {
			t.Fatalf("NumPlates %d, %d axes, %d speeds, %d rotations", w.NumPlates, len(w.PlateAxis), len(w.PlateSpeed), len(w.PlateRotation))
		}
		if w.NumPlates <= before {
			t.Fatalf("%d plates after %v My of rifting, %d at the start", w.NumPlates, w.Duration, before)
		}
		checkCrust(t, w)
		if round == 0 {
			first = snap(w)
		} else if !snap(w).equal(first) {
			t.Fatal("two runs of the same world differ")
		}
	}
}

// New ocean opens along the fracture of a rifted plate.
func TestRiftOpensOcean(t *testing.T) {
	w := caps(t, 6000, []sphere.Vec{at(0, 0)}, []float64{40}, []uint8{land})
	w.MaxPlateSpeed = 100
	areas := w.Mesh.PointAreas()
	// A seed whose rift is not slow.
	var n int
	for seed := uint64(0); ; seed++ {
		trial := caps(t, 6000, []sphere.Vec{at(0, 0)}, []float64{40}, []uint8{land})
		var err error
		if n, err = Rift(t.Context(), trial, areas, 0, rand.New(rand.NewPCG(seed, 2))); err != nil {
			t.Fatal(err)
		}
		if trial.PlateSpeed[0]*trial.Radius > 40 {
			w = trial
			break
		}
	}
	fragment := slices.Clone(w.Plate)
	s := NewSubduction(w)
	steps(t, s, w, 5, 2)
	resample(t, w, 10)

	// Samples that were land of the plate and are now young ocean lie
	// between the fragments.
	var opened int
	for i := range w.Plate {
		if fragment[i] == 1 {
			continue // the surrounding ocean
		}
		if w.CrustType[i] == ocean && w.OceanicAge[i] <= 10 {
			opened++
		}
	}
	if opened < 20*(n-1) {
		t.Fatalf("%d fragments moved apart for 10 My and %d samples of new ocean opened between them", n, opened)
	}
}
