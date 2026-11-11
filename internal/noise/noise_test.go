// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package noise

import (
	"math"
	"math/rand/v2"
	"sync"
	"testing"
)

// fibonacci returns n points spread evenly over the unit sphere.
func fibonacci(n int) [][3]float64 {
	pts := make([][3]float64, n)
	for i := range pts {
		z := 1 - float64(2*i+1)/float64(n)
		r := math.Sqrt(1 - z*z)
		f := float64(i) * 0.6180339887498949
		sin, cos := math.Sincos(2 * math.Pi * (f - math.Floor(f)))
		pts[i] = [3]float64{r * cos, r * sin, z}
	}
	return pts
}

func randomPoint(rng *rand.Rand, extent float64) (x, y, z float64) {
	return (rng.Float64()*2 - 1) * extent, (rng.Float64()*2 - 1) * extent, (rng.Float64()*2 - 1) * extent
}

// The sum, over every lattice corner in reach of a point, of the largest
// magnitude that corner's term can have must stay below 1/amplitude, so
// that no choice of gradients takes the noise outside [−1, 1].
func TestAmplitudeBound(t *testing.T) {
	bound := func(x, y, z float64) float64 {
		s := (x + y + z) * skew
		fi, fj, fk := math.Floor(x+s), math.Floor(y+s), math.Floor(z+s)
		var sum float64
		for di := -1.0; di <= 2; di++ {
			for dj := -1.0; dj <= 2; dj++ {
				for dk := -1.0; dk <= 2; dk++ {
					i, j, k := fi+di, fj+dj, fk+dk
					u := (i + j + k) * unskew
					dx, dy, dz := x-(i-u), y-(j-u), z-(k-u)
					r2 := dx*dx + dy*dy + dz*dz
					if w := kernelRadius2 - r2; w > 0 {
						sum += w * w * w * w * math.Sqrt(r2)
					}
				}
			}
		}
		return sum
	}
	rng := rand.New(rand.NewPCG(1, 2))
	var best float64
	var at [3]float64
	for range 100000 {
		p := [3]float64{rng.Float64(), rng.Float64(), rng.Float64()}
		if b := bound(p[0], p[1], p[2]); b > best {
			best, at = b, p
		}
	}
	for step := 0.02; step > 1e-7; step *= 0.999 {
		p := [3]float64{at[0] + step*rng.NormFloat64(), at[1] + step*rng.NormFloat64(), at[2] + step*rng.NormFloat64()}
		if b := bound(p[0], p[1], p[2]); b > best {
			best, at = b, p
		}
	}
	if best < 0.009288 || best > 0.009290 {
		t.Errorf("largest bound = %.7f, want 0.009289 as the comment on amplitude says", best)
	}
	if amplitude*best >= 1 {
		t.Errorf("amplitude %v times the bound %v is %v, want less than 1", amplitude, best, amplitude*best)
	}
}

// Only four corners can contribute at a point, the four that At visits:
// the kernel of every other corner must be zero there.
func TestKernelSupport(t *testing.T) {
	rng := rand.New(rand.NewPCG(3, 4))
	n := New(99)
	for range 20000 {
		x, y, z := randomPoint(rng, 50)
		lx, ly, lz := x*latticeScale, y*latticeScale, z*latticeScale
		s := (lx + ly + lz) * skew
		fi, fj, fk := math.Floor(lx+s), math.Floor(ly+s), math.Floor(lz+s)
		var sum float64
		for di := -2.0; di <= 3; di++ {
			for dj := -2.0; dj <= 3; dj++ {
				for dk := -2.0; dk <= 3; dk++ {
					i, j, k := fi+di, fj+dj, fk+dk
					u := (i + j + k) * unskew
					sum += n.corner(int64(i), int64(j), int64(k), lx-(i-u), ly-(j-u), lz-(k-u))
				}
			}
		}
		if got, want := n.At(x, y, z), amplitude*sum; math.Abs(got-want) > 1e-12 {
			t.Fatalf("At(%v, %v, %v) = %v, but the sum over all nearby corners is %v", x, y, z, got, want)
		}
	}
}

func TestRange(t *testing.T) {
	rng := rand.New(rand.NewPCG(5, 6))
	fractals := []*Fractal{NewFractal(1, 1, 2, 0.5), NewFractal(2, 5, 2, 0.5), NewFractal(3, 8, 3, 0.9)}
	var largest float64
	for seed := range uint64(8) {
		n := New(seed)
		for range 60000 {
			x, y, z := randomPoint(rng, 500)
			v := n.At(x, y, z)
			if !(v >= -1 && v <= 1) {
				t.Fatalf("seed %d: At(%v, %v, %v) = %v, outside [-1, 1]", seed, x, y, z, v)
			}
			largest = max(largest, math.Abs(v))
			for _, f := range fractals {
				if v := f.At(x, y, z); !(v >= -1 && v <= 1) {
					t.Fatalf("Fractal.At(%v, %v, %v) = %v, outside [-1, 1]", x, y, z, v)
				}
			}
		}
	}
	// The range is used, not just respected.
	if largest < 0.75 {
		t.Errorf("largest |At| over 480,000 points = %v, want at least 0.75", largest)
	}
}

// Points 1e-6 apart differ by about 1e-6 times the slope, and the slope
// of one octave is bounded: it was never seen above 6 per unit.
func TestContinuity(t *testing.T) {
	rng := rand.New(rand.NewPCG(7, 8))
	n := New(11)
	f := NewFractal(12, 4, 2, 0.5) // highest frequency 8, so slope up to about 8 times that of one octave
	const h = 1e-6
	for range 150000 {
		x, y, z := randomPoint(rng, 20)
		dx, dy, dz := rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()
		s := h / math.Sqrt(dx*dx+dy*dy+dz*dz)
		dx, dy, dz = dx*s, dy*s, dz*s
		if d := math.Abs(n.At(x+dx, y+dy, z+dz) - n.At(x, y, z)); d > 8*h {
			t.Fatalf("Noise changes by %g across %g at (%v, %v, %v)", d, h, x, y, z)
		}
		if d := math.Abs(f.At(x+dx, y+dy, z+dz) - f.At(x, y, z)); d > 32*h {
			t.Fatalf("Fractal changes by %g across %g at (%v, %v, %v)", d, h, x, y, z)
		}
	}
}

// Continuity where it is most at risk: across the faces between
// simplices and at lattice corners.
func TestContinuityAcrossSimplexFaces(t *testing.T) {
	n := New(13)
	const h = 1e-9
	check := func(x, y, z float64) {
		t.Helper()
		// Undo the lattice scale and the skew to get the caller's point.
		u := (x + y + z) * unskew
		px, py, pz := (x-u)/latticeScale, (y-u)/latticeScale, (z-u)/latticeScale
		base := n.At(px, py, pz)
		for _, d := range [][3]float64{{h, 0, 0}, {-h, 0, 0}, {0, h, 0}, {0, -h, 0}, {0, 0, h}, {0, 0, -h}, {h, -h, h}} {
			if diff := math.Abs(n.At(px+d[0], py+d[1], pz+d[2]) - base); diff > 1e-7 {
				t.Fatalf("jump of %g at skewed point (%v, %v, %v)", diff, x, y, z)
			}
		}
	}
	rng := rand.New(rand.NewPCG(9, 10))
	for range 20000 {
		i, j, k := float64(rng.IntN(9)-4), float64(rng.IntN(9)-4), float64(rng.IntN(9)-4)
		a, b := rng.Float64(), rng.Float64()
		check(i, j, k)     // a lattice corner
		check(i+a, j+a, k) // on the face x0 = y0
		check(i+a, j, k+a) // on the face x0 = z0
		check(i, j+a, k+a) // on the face y0 = z0
		check(i, j+a, k+b) // on a face of the cube
		check(i+a, j, k+b)
		check(i+a, j+b, k)
	}
}

func TestSeeds(t *testing.T) {
	rng := rand.New(rand.NewPCG(11, 12))
	a, b, c := New(42), New(42), New(43)
	fa, fb, fc := NewFractal(42, 4, 2, 0.5), NewFractal(42, 4, 2, 0.5), NewFractal(43, 4, 2, 0.5)
	var differ, fdiffer int
	const points = 10000
	for range points {
		x, y, z := randomPoint(rng, 100)
		if a.At(x, y, z) != b.At(x, y, z) {
			t.Fatalf("two Noise values with seed 42 differ at (%v, %v, %v)", x, y, z)
		}
		if fa.At(x, y, z) != fb.At(x, y, z) {
			t.Fatalf("two Fractal values with seed 42 differ at (%v, %v, %v)", x, y, z)
		}
		if a.At(x, y, z) != c.At(x, y, z) {
			differ++
		}
		if fa.At(x, y, z) != fc.At(x, y, z) {
			fdiffer++
		}
	}
	if differ < points*99/100 || fdiffer < points*99/100 {
		t.Errorf("seeds 42 and 43 differ at %d (Noise) and %d (Fractal) of %d points, want nearly all", differ, fdiffer, points)
	}
}

// Different seeds must give unrelated noise, not shifted or scaled
// copies.
func TestSeedsAreUncorrelated(t *testing.T) {
	rng := rand.New(rand.NewPCG(13, 14))
	a, b := New(1), New(2)
	var ab, aa, bb float64
	for range 200000 {
		x, y, z := randomPoint(rng, 200)
		va, vb := a.At(x, y, z), b.At(x, y, z)
		ab, aa, bb = ab+va*vb, aa+va*va, bb+vb*vb
	}
	if r := ab / math.Sqrt(aa*bb); math.Abs(r) > 0.02 {
		t.Errorf("correlation between seeds 1 and 2 = %v, want within 0.02 of zero", r)
	}
}

// The mean over a sphere is within 0.4 · featureSize/radius of zero.
func TestMeanOverSphere(t *testing.T) {
	pts := fibonacci(50000)
	for seed := range uint64(12) {
		for _, feature := range []float64{1.0 / 4, 1.0 / 16, 1.0 / 64} {
			freq := Frequency(1, feature)
			n, f := New(seed), NewFractal(seed, 5, 2, 0.5)
			var sum, fsum float64
			for _, p := range pts {
				sum += n.At(p[0]*freq, p[1]*freq, p[2]*freq)
				fsum += f.At(p[0]*freq, p[1]*freq, p[2]*freq)
			}
			mean, fmean := sum/float64(len(pts)), fsum/float64(len(pts))
			if tol := 0.4 * feature; math.Abs(mean) > tol || math.Abs(fmean) > tol {
				t.Errorf("seed %d, feature size %v of the radius: mean %v (Noise), %v (Fractal), want within %v of zero",
					seed, feature, mean, fmean, tol)
			}
		}
	}
}

// The documented meaning of 1 unit: correlation about 0.5 at distance
// 0.5, zero at distance 1, and near zero beyond. Also the documented
// standard deviations.
func TestCorrelation(t *testing.T) {
	rng := rand.New(rand.NewPCG(15, 16))
	n := New(21)
	corr := func(d float64) float64 {
		var ab, aa float64
		for range 100000 {
			x, y, z := randomPoint(rng, 300)
			dx, dy, dz := rng.NormFloat64(), rng.NormFloat64(), rng.NormFloat64()
			s := d / math.Sqrt(dx*dx+dy*dy+dz*dz)
			a := n.At(x, y, z)
			ab += a * n.At(x+dx*s, y+dy*s, z+dz*s)
			aa += a * a
		}
		return ab / aa
	}
	if c := corr(0.5); math.Abs(c-0.5) > 0.05 {
		t.Errorf("correlation at distance 0.5 = %v, want 0.5 ± 0.05", c)
	}
	if c := corr(1); math.Abs(c) > 0.03 {
		t.Errorf("correlation at distance 1 = %v, want 0 ± 0.03", c)
	}
	for _, d := range []float64{1.15, 1.3, 1.6, 2, 3, 10} {
		if c := corr(d); math.Abs(c) > 0.1 {
			t.Errorf("correlation at distance %v = %v, want within 0.1 of zero", d, c)
		}
	}

	std := func(at func(x, y, z float64) float64) float64 {
		var sum2 float64
		const points = 100000
		for range points {
			v := at(randomPoint(rng, 300))
			sum2 += v * v
		}
		return math.Sqrt(sum2 / points)
	}
	for _, c := range []struct {
		octaves int
		want    float64
	}{{1, 0.39}, {2, 0.29}, {4, 0.24}, {8, 0.22}} {
		if got := std(NewFractal(5, c.octaves, 2, 0.5).At); math.Abs(got-c.want) > 0.01 {
			t.Errorf("standard deviation of %d octaves = %v, want %v ± 0.01", c.octaves, got, c.want)
		}
	}
	if got := std(n.At); math.Abs(got-0.39) > 0.01 {
		t.Errorf("standard deviation of Noise = %v, want 0.39 ± 0.01", got)
	}
}

// No direction is favoured: the correlation at half a unit is the same
// along each axis, along the lattice's skew diagonal, and in general.
func TestIsotropy(t *testing.T) {
	rng := rand.New(rand.NewPCG(17, 18))
	n := New(31)
	s3 := 1 / math.Sqrt(3)
	for _, dir := range [][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}, {s3, s3, s3}, {s3, -s3, s3}, {0.6, 0.8, 0}} {
		var ab, aa float64
		for range 100000 {
			x, y, z := randomPoint(rng, 300)
			a := n.At(x, y, z)
			ab += a * n.At(x+0.5*dir[0], y+0.5*dir[1], z+0.5*dir[2])
			aa += a * a
		}
		if c := ab / aa; math.Abs(c-0.5) > 0.08 {
			t.Errorf("correlation at distance 0.5 along %v = %v, want 0.5 ± 0.08", dir, c)
		}
	}
}

func TestFractalOctaves(t *testing.T) {
	// One octave is a Noise, rotated and moved: same statistics, and
	// the sum is normalised by the amplitudes.
	f := NewFractal(9, 3, 2, 0.5)
	if len(f.octaves) != 3 {
		t.Fatalf("octaves = %d, want 3", len(f.octaves))
	}
	if got, want := f.norm, 1/1.75; math.Abs(got-want) > 1e-15 {
		t.Errorf("norm = %v, want %v", got, want)
	}
	for k, o := range f.octaves {
		if want := math.Pow(2, float64(k)); o.freq != want {
			t.Errorf("octave %d frequency = %v, want %v", k, o.freq, want)
		}
		if want := math.Pow(0.5, float64(k)); o.amp != want {
			t.Errorf("octave %d amplitude = %v, want %v", k, o.amp, want)
		}
		// The rotation matrix is orthonormal with determinant +1.
		r := o.rot
		for a := range 3 {
			for b := range 3 {
				dot := r[3*a]*r[3*b] + r[3*a+1]*r[3*b+1] + r[3*a+2]*r[3*b+2]
				want := 0.0
				if a == b {
					want = 1
				}
				if math.Abs(dot-want) > 1e-12 {
					t.Errorf("octave %d: rows %d and %d of the rotation have dot product %v", k, a, b, dot)
				}
			}
		}
		det := r[0]*(r[4]*r[8]-r[5]*r[7]) - r[1]*(r[3]*r[8]-r[5]*r[6]) + r[2]*(r[3]*r[7]-r[4]*r[6])
		if math.Abs(det-1) > 1e-12 {
			t.Errorf("octave %d: rotation determinant = %v, want 1", k, det)
		}
	}
	// Octaves are distinct noises.
	if f.octaves[0].noise == f.octaves[1].noise {
		t.Error("octaves 0 and 1 share a seed")
	}
}

func TestNewFractalPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"no octaves":       func() { NewFractal(1, 0, 2, 0.5) },
		"too many octaves": func() { NewFractal(1, MaxOctaves+1, 2, 0.5) },
		"lacunarity < 1":   func() { NewFractal(1, 2, 0.5, 0.5) },
		"lacunarity NaN":   func() { NewFractal(1, 2, math.NaN(), 0.5) },
		"gain 0":           func() { NewFractal(1, 2, 2, 0) },
		"gain infinite":    func() { NewFractal(1, 2, 2, math.Inf(1)) },
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: no panic", name)
				}
			}()
			fn()
		}()
	}
}

func TestFrequency(t *testing.T) {
	if got := Frequency(6370, 25); got != 254.8 {
		t.Errorf("Frequency(6370, 25) = %v, want 254.8", got)
	}
}

// Evaluation only reads, so goroutines sharing a Fractal get the values
// one goroutine gets. Run with -race.
func TestConcurrentUse(t *testing.T) {
	f := NewFractal(77, 4, 2, 0.5)
	pts := fibonacci(20000)
	want := make([]float64, len(pts))
	for i, p := range pts {
		want[i] = f.At(p[0]*3, p[1]*3, p[2]*3)
	}
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for i, p := range pts {
				if got := f.At(p[0]*3, p[1]*3, p[2]*3); got != want[i] {
					t.Errorf("point %d: got %v, want %v", i, got, want[i])
					return
				}
			}
		})
	}
	wg.Wait()
}

func BenchmarkNoise(b *testing.B) {
	n := New(1)
	var x float64
	for b.Loop() {
		x += 0.37
		n.At(x, x*0.5, x*0.25)
	}
}

func BenchmarkFractal5(b *testing.B) {
	f := NewFractal(1, 5, 2, 0.5)
	var x float64
	for b.Loop() {
		x += 0.37
		f.At(x, x*0.5, x*0.25)
	}
}
