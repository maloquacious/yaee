# Implementation plan

This is the plan for building the generator described in [CLAUDE.md](CLAUDE.md).
It covers the tectonic stage, from an empty repository to a tagged release.
Erosion and biomes come after it and are only outlined here.

Section references (§) are to *Procedural Tectonic Planets*; see [docs/research/papers.md](docs/research/papers.md).

## How to use this plan

- Work on the earliest milestone whose "done when" is unmet.
- A milestone is done when every item in its "done when" list is true, not when its code exists.
- Each milestone leaves `go run ./cmd/yaee` working at full scale, from milestone 2 onward.
- Each milestone is tracked by a GitHub issue, linked from its heading. Large milestones are split into sub-issues of that issue.
- The open decision below is the user's to make. Ask before the milestone that needs it; do not decide it silently.
- When a milestone is finished or the plan changes, update this file in the same commit.

## Target

A default run is an Earth-like world at about 25 km per data point:

| Quantity | Value |
|----------|-------|
| Radius | 6370 km |
| Samples | 942,059 |
| Triangles | about 1.88 million (2N − 4) |
| Simulated time | 250 My, the span the paper reports as giving realistic planets (§7.1) |
| Time step | 2 My, so 125 steps |
| Plates at start | 40, with 0.3 land coverage (the paper's test case, §7.4) |

**Time budget:** a default run finishes in under 10 minutes on the development machine.
That figure is a proposal, not a measurement; revise it after milestone 4, when the first real timings exist.
For comparison, the paper reports 1.9 s per step plus 13 s per resampling at 500,000 samples.

## Decisions

Settled:

| Decision | Answer |
|----------|--------|
| What the datafile holds | The native samples. It is not resampled to a latitude and longitude grid, hexes, or any other client grid. |
| Readers in other languages | We ship a Go reader only. Coding agents write readers for other languages from the datafile specification, so the specification must be complete enough to do that without reading our source. |
| Repeatable output | The same `Config` gives the same bytes on the same CPU architecture. ARM and AMD may differ, because they do not round floating point arithmetic identically. Do not spend effort making them agree. |
| Delaunay triangulation | We write our own, under `internal/`. No third-party triangulation library. |
| Who the documentation is for | Coding agents. They are the users of this library, command, and datafile. See "Documentation" under Design. |

Open:

| Decision | Needed by |
|----------|-----------|
| How much relief the generator produces (§5 amplification). Recorded as undecided in CLAUDE.md. Nothing in milestones 1 to 8 depends on the answer. | Before milestone 9 closes |

## Where the paper is silent

The paper gives the model's structure and its constants, but leaves these to the implementer.
Each is decided in the milestone shown, written up in the code comment, and added to `Config` only if a client would plausibly tune it.

| Gap | Milestone |
|-----|-----------|
| Initial elevation, thickness, and age of continental and oceanic crust | 3 |
| The noise function used to warp plate boundaries, and its parameters | 3 |
| Initial plate speeds and their distribution, up to the maximum v0 | 4 |
| How often to resample: the paper says every 10 to 60 steps "depending on the observed maximum plate speed" | 4 |
| The template ridge profile z_Γ (§4.3) | 5 |
| The exact cubic for the distance transfer f(d), and where its maximum sits (§4.1) | 6 |
| Values of β (fold direction weight) and ε (slab pull weight) | 6 |
| How elevation is kept at or below z_c: maximum uplift is 20 times maximum erosion, and the paper gives no limit | 6 |
| How a trench is formed, and which samples count as "in a trench" for sediment accretion | 6 |
| Which plate subducts when both are oceanic and close in age, and what "partial subduction" does | 6 |
| How a terrane is found and when two plates have "sufficient continental mass" (§4.2) | 7 |
| Whether the area ratio is inside the square root in the collision radius law (§4.2) | 7 |
| λ0 and the transfer function f(x_P) for rifting (§4.4) | 8 |
| What happens to a plate that is entirely consumed | 8 |

## Design

These choices apply to every milestone.

**Stages.** `Generate` runs a list of stages over the world data. Tectonics is the first and, in this plan, the only one. Erosion and biomes are added later as further stages.

**Samples are fixed; plates move.** The Fibonacci samples and their triangulation are computed once (§6). Each plate keeps a rotation. Between resamplings, a plate's samples stay where they were at the last resampling, and their true position is that position turned by the plate's rotation. Nothing is re-triangulated during a run.

**Finding what is at a point.** To ask what plate j holds at a point q, turn q back by plate j's rotation and locate it in the fixed triangulation; plate j covers q if the triangle found belongs to it. This one query serves resampling, collision detection, and image rendering.
This departs from the paper, which tracks intersections between boundary triangles using a bounding box hierarchy (§6). The paper's stated reason for that was interactive speed; ours is a batch run, and a single well-tested query is easier to get right. Revisit it only if profiling at full scale says so.

**Storage.** Per-sample data is held as one slice per attribute, indexed by sample number, not as a slice of structs. The attributes are those of the paper's Table 1 plus the plate number.

**Units.** Kilometres and millions of years throughout, as CLAUDE.md requires.

**Parallelism.** A step may be split across goroutines only when each output value depends on read-only input and is written by exactly one goroutine. No sums are accumulated across goroutines. No goroutine shares a random source with another. A test runs the generator with one processor and with many and compares the bytes.

**Repeatable output.** Tests compare bytes between runs on the same machine. No test stores expected bytes or a checksum produced on one architecture and checks it on another.

**Resolution independence.** Every rule is written in kilometres and years, never in "number of samples" or "number of steps". A test that passes on a 2,000-sample sphere must describe the same physics as a full-scale run.

**Documentation.** The readers are coding agents that will call the library, run the command, or write a datafile reader. Write for a reader who has only the document: state every unit, type, range, default, and byte layout exactly; give examples that run; say what happens on error. Leave out motivation, history, and tutorial narrative.

### Packages

| Package | Contents |
|---------|----------|
| `yaee` (root) | `Config`, `DefaultConfig`, `Validate`, `Generate`, `World`, reading and writing the datafile |
| `internal/sphere` | Vectors, rotations, Fibonacci sampling, triangulation, adjacency, point location |
| `internal/model` | The working data of a world during generation, the stage interface, per-stage random streams |
| `internal/noise` | Seeded coherent noise in three dimensions |
| `internal/tectonics` | Plates, motion, resampling, and the four phenomena |
| `internal/render` | Check images, drawn from a `World` |
| `cmd/yaee` | The development command |

## Milestones

### 1. The sphere

Tracked by [#1](https://github.com/maloquacious/yaee/issues/1), with sub-issues #11 to #14.

**Done** in version 0.5.0-alpha. The code is `internal/sphere`.

Vectors and rotations; Fibonacci sampling (§6); a Delaunay triangulation of the samples on the sphere; neighbour lists; point location (given a point, the triangle containing it and its barycentric weights).

What was built, where the plan left a choice:

- Rotations are unit quaternions. The zero value is not a rotation; use `sphere.Identity()`.
- The triangulation is the convex hull of the samples, built by incremental insertion with edge flips. Its one geometric test is computed exactly.
- Point location walks the mesh from a starting triangle found in a cube-map grid. A point on an edge or at a corner resolves to the lowest-numbered triangle that contains it.
- Barycentric weights are those of the point where the ray from the centre through the query crosses the flat triangle.

Measured at full scale (942,059 samples) on the development machine, a 10-core Apple arm64:

| Measure | Value |
|---------|-------|
| Sample, triangulate, and build the point-location index | 0.40 s on one core |
| Peak resident memory of that build | 120 MB |
| Point location, uniformly random points | 4.3 million queries/s on one core; 29 million/s on ten |
| Nearest sample, uniformly random points | 2.7 million queries/s on one core |
| Triangle area, as a fraction of the mean | 0.82 to 1.31 |
| Smallest distance between samples, as a fraction of the nominal spacing | 0.81 |

Reproduce with `go test ./internal/sphere -run xxx -bench FullScale`.

Done when:

- Sample count for a requested spacing is correct, and the spread of triangle areas is within a stated bound.
- The triangulation has exactly 2N − 4 triangles and 3N − 6 edges, every triangle is wound the same way, and on small spheres every triangle passes the empty circumcircle test.
- Point location returns the correct triangle for random points, for every sample, and at both poles; barycentric weights sum to one.
- Rotations preserve length and compose correctly.
- A full-scale sphere is built in seconds, and the time and memory are recorded in the commit message.

### 2. A working skeleton

Tracked by [#2](https://github.com/maloquacious/yaee/issues/2), with sub-issues #15 to #17.

**Done** in version 0.8.0-alpha. The code is the root package, `internal/model`, `internal/render`, and `cmd/yaee`; the datafile is specified in [docs/datafile.md](docs/datafile.md).

The public API, the datafile, the command, and images, around a world that is still trivial.

`Config` with seed, radius, sample spacing, duration, and time step. `DefaultConfig` and `Validate`. `Generate` returning a `World` with samples and a placeholder elevation. The datafile format: the native samples, versioned, embedding the `Config` and the library version, with named per-sample attributes so later stages can add to it. Reading and writing it. `cmd/yaee` with a command to generate a world and one to render images from a datafile. An equirectangular elevation image.

Done when:

- `go run ./cmd/yaee` generates a full-scale world and writes a datafile and an image under `tmp/`.
- A written datafile reads back equal to the world that produced it.
- Two runs with the same `Config` produce identical bytes, with one processor and with many.
- A file with an unknown format version, or a truncated file, is rejected with an error.
- `Validate` rejects each invalid field with an error naming it.
- The datafile format is specified in a document under `docs/`, complete enough that a coding agent can write a reader in another language from it alone.
- The root package imports no image or command-line library.

What was built, where the plan left a choice:

- `Config` encodes to JSON with the unit in each member's name, such as `radius_km`. `Validate` reports every invalid field, one per line, by its Go name.
- A `World` holds positions, triangles, and a list of attributes. An attribute has a name, a unit, an element type (uint8, int32, float32, or float64), and a component count.
- Elevation is a placeholder: 4(x² − y²) + 6yz − 1 km at unit position (x, y, z). Milestone 3 replaces it. Nothing is random yet; a test fails when that changes.
- Stages implement `model.Stage` and work on `model.World`, which holds one slice per attribute. Each stage draws from its own random stream, seeded by the `Config` seed and the hash of the stage's name. `model.ParallelRange` is the parallel loop that keeps output independent of the processor count.
- The datafile is a header and checksummed sections. It stores positions as float64 and stores the triangles; it does not store neighbour lists, and the specification says how to derive them. It is not compressed. Later stages add attributes or sections without a new format version.
- `testdata/example.yaee` is a 6-sample world listed byte by byte in the specification. When the generator's output changes, run `go test . -run TestExampleFile -update` to rewrite both.
- `render` rebuilds its nearest-sample index from the triangles in the file (`sphere.NewMesh`), so an image also shows that the stored triangulation is usable.
- `yaee generate` requires `-out` or `-write-config`; nothing is written by default. A bare `go run ./cmd/yaee` prints usage and exits with status 2.

Measured at full scale (942,059 samples) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 0.4 s, 211 MB peak resident memory |
| Datafile | 52,755,500 bytes |
| `yaee render` at 2048 × 1024 | 0.4 s, 214 MB peak resident memory |
| Repeat runs, with `GOMAXPROCS=1` and with the default | Identical datafile and image bytes |

### 3. Initial plates and crust

Tracked by [#3](https://github.com/maloquacious/yaee/issues/3), with sub-issues #18 to #20.

**Built** in version 0.11.0-alpha; the review of the images, the last item below, has been asked for and is not yet given. The code is `internal/noise` and `internal/tectonics` (`Plates`, `Crust`), with `sphere.Mesh.PointAreas` and `sphere.Mesh.Distances`.

Seeded coherent noise. Plate centroids distributed over the sphere; plates as the Voronoi cells of the centroids, with boundaries warped by noise (§3, Workflow). Continental and oceanic crust assigned to reach the configured land coverage. Initial elevation, thickness, and age. Images of plates and crust type.

Done when:

- Every sample belongs to exactly one plate, and every plate is a single connected region.
- The plate count matches the `Config`, and land coverage is within a stated tolerance of it.
- With warping turned off, each sample belongs to the plate with the nearest centroid.
- The person reviewing the images accepts the plate shapes as irregular but plausible.

What was built, where the paper and the plan left a choice (the paper gives no method or constant for any of this):

- Noise is simplex gradient noise with a kernel radius of √0.5, gradients hashed from the lattice coordinates and the seed, evaluated at three-dimensional positions so the sphere has no seam or pole. `noise.Fractal` sums octaves, each with its own seed, rotation, and offset.
- Centroids are uniform random points kept at least 0.5 · √(4πR²/P) apart. A sample goes to the plate whose centroid is nearest its position after the position is moved by three fractal noise sources (`PlateWarpAmplitude`, default 1500 km; `PlateWarpFeatureSize`, default 2500 km). Pieces the warp cuts off are given to neighbouring plates, so every plate is connected. The default is 40 plates.
- Continents are regions within plates, not whole plates. Plates are made continental, in random order, until they hold `LandCoverage` / 0.7 of the area; within them, samples are made continental in order of distance from the edge of the group of continental plates plus fractal noise, until the area is reached. Touching continental plates carry one continent. The continental share is within 1.5/N of `LandCoverage` (default 0.3).
- Initial values: thickness 7 km oceanic and 35 km continental; ocean floor at z_r at the coast falling to z_a 500 km out; land at 0.01 · z_c at the coast rising to 0.05 · z_c 500 km inland; oceanic age 202.7 My everywhere, the time the paper's dampening (§4.5) takes from z_r to z_a; no ridge direction, orogeny, or fold. z_r, z_a, z_t, and z_c are `Config` fields with the paper's Table 3 values as defaults.
- The datafile carries every parameter of the paper's Table 1 as an attribute: `plate`, `crust_type`, `thickness`, `elevation`, `oceanic_age`, `ridge_direction`, `orogeny_age`, `orogeny_type`, `fold_direction`.
- `yaee render -kind` selects `elevation`, `plates`, or `crust`.

Measured at full scale (942,059 samples) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 1.0 s, 420 MB peak resident memory |
| Datafile | 126,236,562 bytes |
| `yaee render -kind crust` at 2048 × 1024 | 0.4 s, 280 MB peak resident memory |
| Repeat runs, with `GOMAXPROCS=1` and with the default | Identical datafile bytes |

### 4. Motion and resampling

Tracked by [#4](https://github.com/maloquacious/yaee/issues/4), with sub-issues #21 and #22.

A rotation axis and speed for each plate. Advancing all plates by a time step. Global resampling (§6, Meshing): for every sample, find the plates that cover it, interpolate crust data from the covering plate, and rebuild the plates from the result. Overlaps and gaps are resolved by placeholder rules, replaced in milestones 5 and 6. The resampling schedule.

Done when:

- With all speeds zero, resampling changes nothing.
- A single plate covering the sphere, carrying an analytic field, is turned through a large angle across several resamplings; the field matches the analytic answer within a stated error.
- No sample moves faster than the configured maximum speed.
- After every resampling, every sample belongs to exactly one plate.
- A full-scale run completes, images show plates drifting, and step and resampling times are recorded. Revise the time budget from these numbers.

### 5. Oceanic crust

Tracked by [#5](https://github.com/maloquacious/yaee/issues/5).

Crust generated in the gaps between diverging plates (§4.3): distance to the ridge and to the nearest plate, the blend between interpolated elevation and the ridge profile, crust age, ridge direction. Oceanic dampening (§4.5), so that crust sinks toward the abyssal plain as it ages. Images of crust age.

Done when:

- Two plates diverging at a known speed produce a ridge on the line midway between them, at the ridge elevation z_r.
- Crust age at a point equals its distance from the ridge divided by the spreading speed, within a stated error.
- Ridge direction is parallel to the ridge.
- Dampening follows its closed-form solution: elevation approaches the trench elevation z_t with a time constant of 250 My, and never passes it. Abyssal plains near z_a are what 200 My of that gives; z_a is not a floor.
- No sample is left without crust after a resampling.

### 6. Subduction

Tracked by [#6](https://github.com/maloquacious/yaee/issues/6), with sub-issues #23 to #26.

Detecting convergence along plate boundaries. Choosing the subducting plate (§4.1). Distance from each sample to the subduction front. Uplift from the three transfer functions. Fold direction, orogeny age, and orogeny type. Slab pull. Continental erosion and sediment accretion in trenches (§4.5). Removing subducted crust at resampling. Images of orogeny.

Done when:

- Each transfer function matches its definition at its end points and at its maximum.
- An oceanic plate converging on a continental one at known speed raises the continental margin by the amount the uplift formula gives, at several distances from the front, and raises nothing beyond the subduction distance r_s.
- Oceanic crust always subducts beneath continental; of two oceanic plates, the older subducts.
- Uplift is zero when the relative speed is zero.
- Continental erosion follows its closed-form solution, and elevation never exceeds the maximum continental altitude z_c.
- Slab pull moves a plate's rotation axis toward its subduction front and leaves it unit length.
- In a full-scale run, mountain ranges appear along convergent continental margins and trenches along the fronts, and the reviewer accepts the images.

### 7. Continental collision

Tracked by [#7](https://github.com/maloquacious/yaee/issues/7), with sub-issues #27 and #28.

Terranes as connected regions of continental crust (§3). Detecting collision by interpenetration distance (§6; 300 km in the paper). Detaching a terrane from its plate and attaching it to the other (§4.2). The radius of influence, the elevation surge, and the new fold direction.

Done when:

- Terranes found on a constructed plate match the regions constructed.
- A terrane carried into a continent is transferred to the continent's plate once, and only after the interpenetration threshold is passed.
- The elevation surge matches the formula at the terrane, falls to zero at the radius of influence, and is zero beyond it.
- The radius of influence grows with terrane area and relative speed as the power law states.
- After a transfer, every sample still belongs to exactly one plate, and the total area of continental crust is unchanged.

### 8. Rifting

Tracked by [#8](https://github.com/maloquacious/yaee/issues/8).

Rifting events drawn from the Poisson law (§4.4). Splitting a plate into two to four plates along warped Voronoi boundaries. Diverging motions for the fragments. Removing plates that have been entirely consumed.

Done when:

- Over many seeded trials, the number of rifting events matches the configured rate within sampling error.
- A rifted plate's fragments cover exactly the samples of the original, each fragment is connected, and crust data is unchanged by the split.
- Fragments move apart: new ocean opens along every fracture.
- Larger continental plates rift more often than smaller ones.
- A 250 My full-scale run neither collapses into one continent nor shatters into fragments, and the reviewer accepts the images.

### 9. An Earth-like default

Tracked by [#9](https://github.com/maloquacious/yaee/issues/9), with sub-issues #29 to #31.

Low-frequency noise (§4.5). Tuning `DefaultConfig` so that a default run looks like an Earth-like world. Profiling and speeding up the full-scale run. A summary command that reports statistics for a datafile.

The open decision on relief is settled before this milestone closes.

Done when:

- A default run finishes within the time budget.
- Over a set of seeds, land coverage stays within a stated range, and the distribution of elevation has two peaks, one for continents and one for ocean floor, as Earth's does.
- Elevations stay between the trench depth z_t and the maximum continental altitude z_c.
- A long test, skipped under `testing.Short`, checks those statistics at reduced scale.
- The reviewer accepts images from at least five seeds.

### 10. Release

Tracked by [#10](https://github.com/maloquacious/yaee/issues/10).

Review every exported name in the root package and remove any a client does not need. Complete the documentation for coding agents: package documentation with a runnable example, the command's usage, the datafile specification, and the README. Leave alpha as WORKFLOW.md describes.

Done when:

- Every exported name has a doc comment, and every `Config` field states its unit and default.
- The example in the package documentation runs as a test.
- A coding agent given only the datafile specification writes a reader in a language other than Go, and that reader reads a generated file correctly.
- The command's help output states every command, flag, default, and exit status.
- The version is tagged.

## After this plan

Not planned in detail. Each needs its own plan when its turn comes.

- **Erosion.** A further stage after tectonics. References: Cordonnier et al. 2016, which combines tectonic uplift with fluvial erosion, and Tzathas et al. 2024, which gives an analytical erosion model. Both want an uplift rate per sample, which the tectonic stage computes; milestone 6 should keep it in the world data, not discard it after applying it.
- **Relief.** Depends on the open decision above.
- **Biomes.** A further stage after erosion. It will need climate inputs that nothing in this plan produces.
