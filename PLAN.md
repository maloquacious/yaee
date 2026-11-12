# Implementation plan

This is the plan for building the generator described in [CLAUDE.md](CLAUDE.md).
It covers the tectonic stage, from an empty repository to a tagged release, and the stages added since.
Erosion comes after it and is only outlined here.

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

**Time budget:** a default run finishes in under 2 minutes on the development machine (Apple M4, 10 processors).
Revised after milestone 4, which measured 4.4 s for a default run: 1.2 s to build the sphere and the initial world, then at most 50 resamplings of 76 ms each; a step between resamplings costs under a microsecond.
Milestone 5 brought a default run to 6.3 s.
Milestone 9 measured 36 s for the tuned default run.
Milestones 6 to 8 add work to every step and every resampling. The budget allows each of the 125 steps about a second, which is twelve times a resampling today.
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

| How much relief the generator produces | Decided 2026-11-11: the generator stops at the coarse crust. The paper's §5 amplification is on hold, not ruled out: it is not implemented unless the user asks, and the per-sample data it would need (Table 1) stays in the datafile. |
| Thin seas through continents ([#32](https://github.com/maloquacious/yaee/issues/32)) | Decided 2026-11-11: accepted, and left in the output. The generator does not close them; `docs/datafile.md` tells clients they are there. The measurements and causes are in the issue. |
| Climate and biomes | Decided 2026-11-12: not part of yaee. The client computes them; Hāmwold already has its own temperature, moisture, and biomes. |
| Where erosion acts (milestone 12) | Decided 2026-11-12: a stage after rock type that writes new attributes and leaves `elevation` as the tectonic stage made it. A minor version. Erosion inside the simulation would change `elevation` for every seed: a major version and a new module path. |
| Erosion solver (milestone 12) | Decided 2026-11-12: the implicit stream power solver of Cordonnier et al. 2016 §5, O(n) per iteration once the samples are ordered. The analytical solution of Tzathas et al. 2024 is the fallback if it misses the time budget. |
| Erodibility by rock type (milestone 12) | Decided 2026-11-12: yes. The erosion coefficient k is multiplied by a factor for each value of `rock_type`. |
| `rock_type` is alpha | Decided 2026-11-12: until its tuning is settled, a change to `rock_type` values or to the `Rock*` `Config` fields is a minor version, not a major one. `docs/datafile.md` and the `AttrRockType` doc comment say so. Every other attribute keeps the rule below. |
| Version bump for a new attribute | Decided 2026-11-12: minor. The datafile's format version stays 1, because the specification lets a later version add attributes. Every value already written for a given seed must stay the same; changing one is a major change. WORKFLOW.md states the rule. |

Open: none.

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
| λ0, its time window, and the transfer function f(x_P) for rifting (§4.4) | 8 |
| What happens to a plate that is entirely consumed | 8 |
| The low-frequency noise on elevation, its size, and when it is applied (§4.5) | 9 |

## Design

These choices apply to every milestone.

**Stages.** `Generate` runs a list of stages over the world data. Tectonics is the first, rock type (milestone 11) the second, and erosion is added later as a further stage.

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
| `yaee` (root) | `Config`, `DefaultConfig`, `Validate`, `Generate`, `World`, reading and writing the datafile, `Locator` (which triangle of samples contains a point, and which sample is nearest) |
| `internal/sphere` | Vectors, rotations, Fibonacci sampling, triangulation, adjacency, point location |
| `internal/model` | The working data of a world during generation, the stage interface, per-stage random streams |
| `internal/noise` | Seeded coherent noise in three dimensions |
| `internal/tectonics` | Plates, motion, resampling, and the four phenomena |
| `internal/render` | Check images, drawn from a `World` |
| `internal/stats` | Measurements of a `World`, for tuning and review |
| `internal/rocks` | The rock type stage |
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

**Done** in version 0.11.0-alpha. The reviewer accepted the full-scale plate and crust images of seeds 1, 2, and 3 on 2026-11-11. The code is `internal/noise` and `internal/tectonics` (`Plates`, `Crust`), with `sphere.Mesh.PointAreas` and `sphere.Mesh.Distances`.

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

**Done** in version 0.13.0-alpha. The code is `internal/tectonics`: `Motion`, `Advance`, `Velocity`, `Query`, `Resample`, and the `Simulation` stage.

A rotation axis and speed for each plate. Advancing all plates by a time step. Global resampling (§6, Meshing): for every sample, find the plates that cover it, interpolate crust data from the covering plate, and rebuild the plates from the result. Overlaps and gaps are resolved by placeholder rules, replaced in milestones 5 and 6. The resampling schedule.

Done when:

- With all speeds zero, resampling changes nothing.
- A single plate covering the sphere, carrying an analytic field, is turned through a large angle across several resamplings; the field matches the analytic answer within a stated error.
- No sample moves faster than the configured maximum speed.
- After every resampling, every sample belongs to exactly one plate.
- A full-scale run completes, images show plates drifting, and step and resampling times are recorded. Revise the time budget from these numbers.

What was built, where the paper and the plan left a choice:

- Each plate's axis is a uniform random point of the sphere. The speed of the plate's fastest sample is drawn uniformly from 0 to `MaxPlateSpeed` (v0, default 100 km/My), and the angular speed follows from it, so no sample moves faster than v0.
- A triangle whose corners are in different plates is divided between them: a point belongs to the plate of its nearest corner (greatest barycentric weight). A plate's data at a point is interpolated from the triangle's corners that are in that plate.
- Resampling happens when the fastest sample has moved 500 km since the last one, and after the last step. The paper's rule is a count of steps that amounts to 2000 km or more.
- Interpolation: scalars are blended; vectors are blended, turned with the plate, and projected onto the tangent plane; crust type and orogeny type come from the nearest corner. Each resampling adds an error proportional to the square of the sample spacing.
- Placeholders: a gap between plates gets new oceanic crust at z_r with age 0, and joins the nearest plate (replaced in milestone 5). Where plates overlap, continental crust is kept over oceanic, then the lowest-numbered plate (milestone 6 replaces this).
- After a resampling a plate may be in several pieces or have no samples. A plate whose fastest sample would exceed v0 after gaining samples has its angular speed lowered to the limit.
- `Duration` 0 gives the initial world. A `Duration` that is not a multiple of `TimeStep` ends with a shorter step. A run may have at most 1,000,000 steps.
- Plate axes and speeds are not written to the datafile. It holds per-sample data only.

Measured at full scale (942,059 samples, 250 My, 125 steps) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 4.4 s, 556 MB peak resident memory; 17.6 s with `GOMAXPROCS=1` |
| One step without a resampling | under 1 µs |
| One resampling | 76 ms; 381 ms with `GOMAXPROCS=1` |
| Datafile | 126,236,594 bytes |
| Runs with `GOMAXPROCS=1` and with the default | Identical datafile bytes |

### 5. Oceanic crust

Tracked by [#5](https://github.com/maloquacious/yaee/issues/5).

**Done** in version 0.14.0-alpha. The code is `internal/tectonics/ocean.go` (`AgeOcean`, `RidgeProfile`, and the gap filling of `Resample`) and `render.OceanicAge`.

Crust generated in the gaps between diverging plates (§4.3): distance to the ridge and to the nearest plate, the blend between interpolated elevation and the ridge profile, crust age, ridge direction. Oceanic dampening (§4.5), so that crust sinks toward the abyssal plain as it ages. Images of crust age.

Done when:

- Two plates diverging at a known speed produce a ridge on the line midway between them, at the ridge elevation z_r.
- Crust age at a point equals its distance from the ridge divided by the spreading speed, within a stated error.
- Ridge direction is parallel to the ridge.
- Dampening follows its closed-form solution: elevation approaches the trench elevation z_t with a time constant of 250 My, and never passes it. Abyssal plains near z_a are what 200 My of that gives; z_a is not a floor.
- No sample is left without crust after a resampling.

What was built, where the paper and the plan left a choice:

- The ridge is not traced as a line. Each gap sample finds its nearest border sample and its nearest border sample of another plate, at distances d_a ≤ d_b, by spreading the border samples through the gap. The ridge is where d_a = d_b, so with three or more plates round a gap it is the lines along which the two nearest plates are equally near. The distance to the ridge is taken as (d_b − d_a)/2, which makes the paper's α equal to (d_b − d_a)/(d_b + d_a).
- The age of new crust is α times the time since the last resampling: the gap opened steadily over that time, so this is the distance from the ridge divided by the speed at which the plate leaves it.
- The template ridge profile z_Γ is the dampening curve: the elevation of crust that formed at z_r and has been dampened for the sample's age. It is z_r on the ridge.
- New oceanic crust is never above sea level: border elevations above 0 count as 0 in the interpolated elevation z̄. The paper takes them as they are.
- Ridge direction is the paper's (p − q) × p, with the direction from the ridge taken from the two nearest border samples and averaged three times over neighbouring gap samples. Without the averaging it is uneven by 5° on average.
- A gap that only one plate borders has no ridge. It closes with the plate's elevation and the age of the gap.
- Dampening is the paper's formula as written, an explicit Euler step, applied with the ageing every time step. It is 0.013 km from the closed form after 250 My. A step too long for the formula lands on z_t. `OceanicDampening` (ε_o, default 0.04 km/My) is a `Config` field.
- Resampling no longer blends oceanic with continental crust. Before, oceanic age near a coast was averaged with the 0 of continental crust, and land at a coast was pulled below sea level by the deep ocean beside it. The paper blends across the coast.
- The initial ocean keeps sinking, as the formula says: from z_a to −8.5 km after 250 My. Whether the default world should start with younger ocean is for milestone 9.
- `yaee render -kind age` draws oceanic age.

Measured at full scale (942,059 samples, 250 My, 125 steps, 42 resamplings) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 6.3 s, 571 MB peak resident memory; 19.3 s with `GOMAXPROCS=1` |
| Datafile | 126,236,629 bytes |
| Runs with `GOMAXPROCS=1` and with the default | Identical datafile bytes |

Seen in the age image of seed 1: bands parallel to the ridges, youngest on the ridge. Also the footprints recorded in [#6](https://github.com/maloquacious/yaee/issues/6), which come from the overlap placeholder, and rows of teeth along some plate boundaries, whose cause has not been traced; check both again after milestone 6.

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

**Done** in versions 0.15.0-alpha and 0.16.0-alpha. The reviewer accepted the images on 2026-11-11, with the straight coasts and the too-high land below noted as known. The code is `internal/tectonics/subduction.go`, the overlap rule of `Resample`, and `render.Orogeny`.

What was built, where the paper and the plan left a choice:

- Subduction is resolved every time step, not at resampling (option 1 of #6). `Subduction.Advance` marks a sample as subducted when it lies beneath overriding crust of another plate at the end of a step, or at points no more than half a sample spacing apart along its path during the step. A plate holds nothing where its crust is subducted. This removed the footprints and the rows of teeth recorded after milestone 5.
- Which crust subducts (`Overrides`): oceanic beneath continental; of two oceanic crusts, that of the plate whose mean oceanic age at the last resampling is greater, the higher-numbered plate at equal ages; of two continental crusts, that of the higher-numbered plate. Comparing ages sample by sample made two plates consume parts of each other.
- Continental crust that meets continental crust is consumed until milestone 7 exists. The continental share of the default world falls from 0.30 to 0.17.
- A subduction front is the samples on the edge of an overriding plate that lie over another plate's subducted or overridden crust, where that plate moves into the edge with at least 0.3 of the relative speed. The edge's outward direction is measured over 4 sample spacings: over the nearest neighbours alone it is up to 33° off, and plates sliding past each other were given fronts.
- d(p) is the distance to the nearest front sample, found by spreading through the plate's samples and bounded by r_s. It is within 1.5 sample spacings of the true distance. It is recomputed only for plates whose front changed.
- f is two cubic smoothsteps: 0 at the front, 1 at d_0 = r_s/6, 0 at r_s. g is v/v_0, not limited to 1. h is the paper's z̃², with the elevation of the subducting crust taken at the nearest front sample, not at p.
- Uplift is multiplied by 1 − max(z, 0)/z_c, and elevation is limited to z_c. The paper gives no limit.
- The applied uplift rate of the last step is kept per sample and written as the `uplift` attribute.
- Fold direction gains β (s_P − s_Q) δt with β = 0.01 My/km. Orogeny becomes Andean with age 0 when uplifted crust emerges, or is raised above sea level with no orogeny; the age then counts from that moment, also while uplift continues. Crust type is not changed.
- Slab pull uses ε = 10⁻⁶ per km of front per My, multiplied by the sample spacing so that it does not depend on resolution (within 20% between two spacings). The plate's angular speed is rescaled so that its fastest sample keeps its speed; with the paper's rule nearly every subducting plate ended at v_0.
- A trench is a state, not a rate: oceanic crust of the subducting plate within 150 km of a front is lowered to z_t at the front, rising with the square of the distance. Sediment accretion raises a trench that is no longer being formed at ε_t until it is gone.
- Continental erosion is the paper's formula as written, for continental crust above sea level only. It is 0.0045 km from the closed form for 10 km of land over 250 My.
- `SubductionDistance` (r_s, 1800 km), `SubductionUplift` (u_0, 0.6 km/My), `ContinentalErosion` (ε_c, 0.03 km/My), and `SedimentAccretion` (ε_t, 0.3 km/My) are `Config` fields with the paper's values. d_0, β, ε, the trench width, and the front's convergence share are constants in the code.
- `yaee render -kind orogeny` draws orogeny type and age.
- Version 0.16.0-alpha roughened the initial plate boundaries (milestone 3's warp): eight octaves of noise with a gain of 0.7, down to 20 km, where there were four with a gain of 0.5; the largest bends are unchanged. Continents break along the initial boundaries when the plates first part, and with the smoother boundaries those coasts were straight lines hundreds of km long. A boundary now moves 0.41 of `PlateWarpAmplitude` on average.
- Straight coasts with corners remain where two continents have met: one continent's crust is consumed along the other plate's edge, which the consumption itself wears into a smooth arc, and the cut becomes a coast when the plates part again. This is for milestone 7 to remove.

Measured at full scale (942,059 samples, 250 My, 125 steps) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 26 s, 624 MB peak resident memory |
| Datafile | 133,773,268 bytes |
| Runs with `GOMAXPROCS=1` and with the default | Identical datafile bytes |
| Continental elevation, seed 1 | median 3.9 km, 90th percentile 6.0 km, maximum 7.8 km |
| Samples above sea level, seed 1 | 0.194 of all; 0.024 of all are oceanic crust above sea level |

For milestone 9: with the paper's constants the default world's land is far too high. r_s of 1800 km reaches across most continents, u_0 is 20 times ε_c, and the young ocean that subducts is shallow, which makes h large. Away from any front, erosion alone halves the initial land, from 0.1 km to 0.047 km in 250 My.

### 7. Continental collision

Tracked by [#7](https://github.com/maloquacious/yaee/issues/7), with sub-issues #27 and #28.

Terranes as connected regions of continental crust (§3). Detecting collision by interpenetration distance (§6; 300 km in the paper). Detaching a terrane from its plate and attaching it to the other (§4.2). The radius of influence, the elevation surge, and the new fold direction.

Done when:

- Terranes found on a constructed plate match the regions constructed.
- A terrane carried into a continent is transferred to the continent's plate once, and only after the interpenetration threshold is passed.
- The elevation surge matches the formula at the terrane, falls to zero at the radius of influence, and is zero beyond it.
- The radius of influence grows with terrane area and relative speed as the power law states.
- After a transfer, every sample still belongs to exactly one plate, and the total area of continental crust is unchanged.

**Done** in version 0.17.0-alpha. The reviewer accepted the images on 2026-11-11, with the too-high land below left for milestone 9. The code is `internal/tectonics/collision.go` (`Terranes`, `Subduction.Converge`, `Subduction.Collide`) and the resampling rule of `simulate`.

What was built, where the paper and the plan left a choice:

- Terranes are computed when a collision needs them, from the neighbour lists; nothing is kept between calls. They are numbered by their lowest sample.
- "Sufficient continental mass" is read as: continental crust on both plates where they meet, whatever the sizes. A terrane narrower than the threshold is consumed before it can collide, which is the paper's forced subduction.
- The interpenetration distance is kept per pair of plates and across resamplings: the sum over steps of the greatest closing speed at a front where continental crust goes beneath continental crust. The paper measures the overlap of the plates' triangles; ours is removed at every resampling, so there is none to measure. The distance returns to 0 when the pair has had no such front while the fastest sample moved 500 km.
- Until the collision, the continental crust of the higher-numbered plate is consumed, as in milestone 6. Each collision therefore loses a strip up to `CollisionPenetration` wide.
- A collision forces a resampling in its step, because a sample can change plates only while both plates' samples are where the mesh has them. A default run has about 20 more resamplings.
- Which side moves: the terranes in contact (within 4 sample spacings of the collision front) with the smaller total area leave their plate; at equal areas, those of the plate that was beneath. Samples of the same plate that the moved terranes enclose go with them. What is left of the plate keeps its movement.
- r = r_c √((v/v_0)(𝒜/𝒜_0)), both ratios under the root, and at most r_c. 𝒜_0 is 4πR²/`Plates`. This departs from the paper, which prints the area ratio outside the root, r = r_c √(v/v_0) · 𝒜/𝒜_0, and no limit. The printed formula was tried on 2026-11-11 (default configuration, seeds 1 to 5): the collision of two large continents raised both from coast to coast, and continental crust above 2 km went from 9.4% to 27.9% in seed 2. The reviewer decided to keep ours.
- The surge is limited by scaling its whole profile, so that it is at most z_c at the suture and still falls smoothly to 0 at r: Δ_c 𝒜 is replaced with z_c (1 − exp(−Δ_c 𝒜/z_c)). It is then multiplied by 1 − max(z, 0)/z_c, as uplift is. Cutting each sample off at z_c gave plateaus at z_c with a cliff at r.
- Both sides of the suture are raised: the terrane by the same profile, measured from the suture. The paper raises the collided plate only, which left a cliff of up to z_c along every suture. Oceanic crust is not raised.
- Distances are straight across the sphere, not through the plate.
- Fold direction is the paper's formula, which is the direction from the terrane's centroid to the sample, in the tangent plane. Issue #28 says "perpendicular to the direction from the centroid"; the formula it quotes says along it, and the formula was followed.
- `CollisionPenetration` (300 km), `CollisionDistance` (r_c, 4200 km), and `CollisionCoefficient` (Δ_c, 1.3 × 10⁻⁵ per km) are `Config` fields with the paper's values.
- The `uplift` attribute does not include a collision's surge, which is not a rate.

Measured at full scale (942,059 samples, 250 My, 125 steps) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 31 s, 850 MB peak resident memory |
| Datafile | 133,773,368 bytes |
| Runs with `GOMAXPROCS=1` and with the default | Identical datafile bytes |
| Collisions, seed 1 | 30, in 21 steps, moving 35 terranes |
| Continental share of the samples, seeds 1 to 3 | 0.280, 0.280, 0.283 (0.169 for seed 1 before this milestone) |
| Continental elevation, seed 1 | median 4.8 km, 90th percentile 6.5 km, maximum 9.9 km |

Seen in the images:

- The straight coasts left by continental consumption are gone, and continents merge.
- For milestone 9: the land is higher still. Δ_c 𝒜 is above z_c for any terrane over 770,000 km², so nearly every collision raises the suture to z_c, across up to 4200 km, and repeated collisions leave whole continents near z_c (seed 2).
- The orogeny image shows the edge of each region of influence as an arc: orogeny type and age change for every sample within r, however little it was raised.
- Thin lines of lower land inside high continents, and small plates of ocean floor enclosed by a continent, where slivers of a third plate lie along a suture. Not traced.
- With no rifting, plates only merge: 40 plates at the start, fewer with samples at the end. Milestone 8.

### 8. Rifting

Tracked by [#8](https://github.com/maloquacious/yaee/issues/8).

Rifting events drawn from the Poisson law (§4.4). Splitting a plate into two to four plates along warped Voronoi boundaries. Diverging motions for the fragments. Removing plates that have been entirely consumed.

Done when:

- Over many seeded trials, the number of rifting events matches the configured rate within sampling error.
- A rifted plate's fragments cover exactly the samples of the original, each fragment is connected, and crust data is unchanged by the split.
- Fragments move apart: new ocean opens along every fracture.
- Larger continental plates rift more often than smaller ones.
- A 250 My full-scale run neither collapses into one continent nor shatters into fragments, and the reviewer accepts the images.

**Done** in version 0.18.0-alpha. The reviewer accepted the images on 2026-11-11, with the too-high land below left for milestone 9 and the thin seas for [#32](https://github.com/maloquacious/yaee/issues/32). The code is `internal/tectonics/rift.go` (`DrawRifts`, `Rift`, `RemoveEmptyPlates`, `Subduction.Renumber`) and the resampling rule of `simulate`.

What was built, where the paper and the plan left a choice:

- λ_0 is a rate per My, the `Config` field `RiftingRate` (0.004): the mean number of rifts per My of a wholly continental plate of area 𝒜_0 = 4πR²/`Plates`. A plate rifts in a step of δt with probability 1 − exp(−λ δt), λ = λ_0 f(x_P) 𝒜/𝒜_0, so the number of rifts does not depend on the time step. The paper's P = λ e^(−λ) is the chance of exactly one event in an unstated window; ours is the chance of at least one, and agrees with it to first order.
- f(x_P) = 0.25 + 0.75 x_P: linear, 1 for a wholly continental plate, and above 0 for an oceanic one so that a large oceanic plate still breaks.
- A plate smaller than 𝒜_0 does not rift. With half that, the plates of a default run grew from 40 to 67 in 250 My; with 𝒜_0 the number stays near 40. No plate rifts if `MaxPlateSpeed` is 0.
- n is 2, 3, or 4 with equal chance. Centroids are samples of the plate drawn uniformly, kept apart as the initial centroids are. The fragments are the warped Voronoi cells of the centroids within the plate, made connected as the initial plates are, with the warp scaled to the size of a fragment.
- Motion: every fragment gets a new movement in place of the plate's. Its centroid moves straight away from the plate's centroid, all fragments at one angular speed v/R with v uniform from 0 to v_0. Any two fragments then move apart everywhere along their boundary; a test checks every boundary place over 40 rifts.
- A rift forces a resampling in its step, as a collision does, and is applied after the step's collisions.
- A plate that has no sample left is removed after each resampling, and the plates above it take the next lower numbers. A plate number therefore identifies a plate within one datafile only; the specification says so, and that the number of plates is 1 + the greatest value of `plate`. The fragment of the first centroid keeps the plate's number, and the others are appended.
- The simulation stage now draws random numbers, for rifting only.

Measured at full scale (942,059 samples, 250 My, 125 steps) on the development machine:

| Measure | Value |
|---------|-------|
| `yaee generate` | 40 s, 820 MB peak resident memory |
| Datafile | 133,773,396 bytes |
| Runs with `GOMAXPROCS=1` and with the default | Identical datafile bytes |
| With `RiftingRate` 0, seed 1 | The collisions (30) and elevation statistics of milestone 7; 29 plates, its empty plates removed |
| Steps with a rift, seeds 1 to 3 | 14, 10, 10 |
| Plates at the end, seeds 1 to 3 | 46, 40, 34 (29 for seed 1 without rifting) |
| Resamplings, seeds 1 to 3 | 66, 67, 79 (48 for seed 1 without rifting) |
| Collisions, seeds 1 to 3 | 64, 76, 97 (30 for seed 1 without rifting) |
| Continental share of the samples, seeds 1 to 3 | 0.274, 0.271, 0.269 |
| Continental elevation, seed 1 | median 7.4 km, 90th percentile 8.8 km, maximum 9.8 km |

Seen in the images:

- Land is in several continents instead of one mass, and the number of plates holds.
- For milestone 9: the land is higher again. Fragments set off at up to v_0 and run into their neighbours, so a run has two to three times the collisions, and each raises its suture toward z_c. The median continental elevation of seed 1 went from 4.8 km to 7.4 km.
- For milestone 9: oceanic crust above sea level went from 0.9% to 3 to 6% of the samples, along subduction fronts.
- Thin seas one or two samples wide run through some continents. The one traced (seed 2) lies inside a single plate: a rift that opened a narrow sea, after which the two fragments collided and joined with the sea still between them. A collision moves the terranes and what they enclose; it does not close the ocean between them. To be investigated after milestone 9: [#32](https://github.com/maloquacious/yaee/issues/32).
- At `RiftingRate` 0.006 and above the land breaks into many small pieces and the number of plates grows through the run (51 to 62 at 250 My for seeds 1 to 3).

### 9. An Earth-like default

Tracked by [#9](https://github.com/maloquacious/yaee/issues/9), with sub-issues #29 to #31.

**Done** in version 0.20.0-alpha. The reviewer accepted the full-scale images of seeds 1 to 5 on 2026-11-11; the thin seas through continents were the only distraction, and are [#32](https://github.com/maloquacious/yaee/issues/32). The code is `tectonics.AgeOcean`, `tectonics.ElevationNoise`, the defaults in `config.go`, and `internal/stats`.

Low-frequency noise (§4.5). Tuning `DefaultConfig` so that a default run looks like an Earth-like world. Profiling and speeding up the full-scale run. A summary command that reports statistics for a datafile.

The decision on relief was settled on 2026-11-11: see Decisions.

9.3 ([#31](https://github.com/maloquacious/yaee/issues/31)) is **done** in version 0.19.0-alpha: `yaee stats -in file` prints measurements of a datafile as JSON, and `yaee help stats` gives every member with its unit. The code is `internal/stats`. Beyond the issue's list it reports land that is oceanic crust, elevation by crust type with quantiles, continents and landmasses (connected regions whatever their plates), and the share of each orogeny type; tuning needs them. Fractions are of the sphere's area, not of the sample count.

9.1 ([#29](https://github.com/maloquacious/yaee/issues/29)) is **done** in version 0.20.0-alpha. The paper's constants gave a world whose continental crust had a median elevation of 7.4 km and whose ocean floor had a median of −1.4 km. What changed:

- **Oceanic dampening sinks crust toward `AbyssalElevation`, not `TrenchElevation`** (a departure from §4.5, documented on `tectonics.AgeOcean`). Only a trench is below the abyssal plain. The initial ocean is 3 × |z_a| / ε_o old (105.9 My by default), which only sets which plate subducts.
- **Six defaults differ from the paper's Table 3**: `RidgeElevation` −2.5 (paper −1), `OceanicDampening` 0.17 (0.04), `SubductionDistance` 600 (1800), `ContinentalErosion` 0.1 (0.03), `CollisionDistance` 1000 (4200), `CollisionCoefficient` 5e-7 (1.3e-5). Each field's doc comment gives the reason. The formulas of §4.1, §4.2, and the erosion of §4.5 are unchanged.
- **Elevation noise** (§4.5, last sentence) is a stage of its own, `tectonics.ElevationNoise`, applied once after the last step, to both crust types: `ElevationNoiseAmplitude` 0.5 km, `ElevationNoiseFeatureSize` 2000 km, three octaves. Applied every step it would grow with the number of steps. It puts 0.6% to 2% of the sphere's area under water as continental crust below sea level; an amplitude of 1 km flooded 3.5% and put a sea at the centre of every continent.
- `LandCoverage` stays 0.3. Subduction and collisions consume continental crust, so less than 0.3 remains.
- `tectonics` tests keep the paper's constants (`earth()`), because they state distances and elevations that follow from them.

Stated ranges for a default run, with the measurements of seeds 1 to 5 at full scale (`yaee stats`):

| Statistic | Range | Measured |
|-----------|-------|----------|
| `land_fraction` | 0.20 to 0.30 | 0.240 to 0.256 |
| `continental_fraction` | 0.22 to 0.30 | 0.255 to 0.265 |
| `oceanic_land_fraction` | 0 to 0.01 | 0.000 |
| `continental_elevation_km` p50 | 0.3 to 1.0 | 0.45 to 0.71 |
| `continental_elevation_km` p90 | 1.2 to 2.6 | 1.38 to 2.35 |
| `elevation_km` max | at most `MaxContinentalElevation` (10) | 4.3 to 6.3 |
| `oceanic_elevation_km` p50 | −5.5 to −4.0 | −5.1 to −4.4 |
| `elevation_km` min | at least `TrenchElevation` (−10) | −9.5 to −9.4 |
| `plates.count` | 30 to 70 | 46 to 49 |

For comparison, Earth's land is 0.29 of its surface, with a median elevation near 0.45 km, and its ocean floor has a median depth near 4 km.
The histogram of elevation has two peaks, the ocean floor from −6 to −4 km and the land from 0 to 0.5 km, with nearly nothing from −2 to −0.5 km.
`TestDefaultIsEarthLike` checks wider ranges at 100 km per sample for the same five seeds; it is skipped under `-short`.

9.2 ([#30](https://github.com/maloquacious/yaee/issues/30)) is **done**: the tuned default run takes 36 s and 807 MB of peak resident memory on the development machine, within the budget, so nothing was changed for speed. Per step: moving the plates and finding who overrides whom 50 ms, subduction fronts 41 ms, everything else 12 ms. Per resampling (71 in the run): resampling 145 ms, rebuilding the subduction state 123 ms, collisions 28 ms. `yaee generate` has `-cpuprofile` and `-memprofile`.

Done when:

- A default run finishes within the time budget.
- Over a set of seeds, land coverage stays within a stated range, and the distribution of elevation has two peaks, one for continents and one for ocean floor, as Earth's does.
- Elevations stay between the trench depth z_t and the maximum continental altitude z_c.
- A long test, skipped under `testing.Short`, checks those statistics at reduced scale.
- The reviewer accepts images from at least five seeds.

### 10. Release

Tracked by [#10](https://github.com/maloquacious/yaee/issues/10).

**Done** in version 1.0.0, tagged `v1.0.0`. The user confirmed the API as stable on 2026-11-11. A fresh agent given only `docs/datafile.md` and a generated file wrote a reader in Python whose output matched the Go reader's byte for byte; what it had to guess was fixed in the specification in version 0.21.1-alpha.

Review every exported name in the root package and remove any a client does not need. Complete the documentation for coding agents: package documentation with a runnable example, the command's usage, the datafile specification, and the README. Leave alpha as WORKFLOW.md describes.

Done when:

- Every exported name has a doc comment, and every `Config` field states its unit and default.
- The example in the package documentation runs as a test.
- A coding agent given only the datafile specification writes a reader in a language other than Go, and that reader reads a generated file correctly.
- The command's help output states every command, flag, default, and exit status.
- The version is tagged.

### 11. Rock type

Tracked by [#33](https://github.com/maloquacious/yaee/issues/33).

A stage after `tectonics.ElevationNoise` gives every sample a rock type, written as a new attribute `rock_type`. The client is Hāmwold, which places ore by rock type; which ores exist is the client's business, not the generator's.

The stage reads only what the tectonic stages already write: `crust_type`, `orogeny_type`, `orogeny_age`, `uplift`, `oceanic_age`, and, if needed, a distance to a plate boundary or subduction front found by a multi-source breadth-first pass over sample neighbours (O(n)). It adds no plate velocities or boundary stress to the datafile.

The starting classification, to be tuned:

| Crust | `orogeny_type` | Rock type |
|-------|----------------|-----------|
| Oceanic | any | igneous |
| Continental | 1, Andean | igneous |
| Continental | 2, Himalayan | metamorphic |
| Continental | 0, none | sedimentary |

A seeded noise field, with its size in km, roughens the edges between classes and puts patches of each class inside the others, so that `rock_type` is not a copy of `orogeny_type`.
For comparison, ProcGenesis WorldGen measured 42% metamorphic, 37% igneous, and 21% sedimentary on land (`~/Software/maloquacious/grift/ALGORITHMS.md` §7.2). It is a reference for the look, not a target, and it is not ported: do not port its §7.1 boundary classification, and do not copy code from that repository's `upstream/` directory, which has no licence grant.

Done when:

- The stage implements `model.Stage` and draws from its own random stream, seeded by the `Config` seed and the stage name.
- Every attribute written before this milestone is byte-identical for a given `Config` with and without the stage; a test checks it.
- `rock_type` is `u8`: 0 is "unclassified", and each class has its own value. No sample in a generated world is 0; a test checks it.
- With a noise amplitude of 0, every sample's class follows the table above exactly; a test checks it.
- The same seed gives the same `rock_type`. A different seed changes the noise, but not the class of samples far from any class edge; a test checks it.
- Every tuning number is a `Config` field whose doc comment gives its unit and default, or a constant whose comment says why it is not a field. Every length is in km, never in samples.
- `docs/datafile.md` specifies `rock_type`: its values, their meanings, and its range, plus any new `META` config members. `testdata/example.yaee` and its byte listing are regenerated, and the test that keeps them in step passes.
- `yaee stats` reports the share of each rock type by area, on land and overall.
- `cmd/yaee` renders a rock type image, and the reviewer accepts the full-scale images of seeds 1 to 3.
- The version is a minor bump from 1.0.0 (see Decisions), and is tagged.

**Done** in version 1.1.0. The reviewer accepted the full-scale images of seeds 1 to 3 on 2026-11-12. `rock_type` stays alpha (see Decisions). The code is `internal/rocks` (`RockType`), `render.RockType`, and the `rock_type` member of `yaee stats`.

What was built, where the issue left a choice:

- The starting table gave almost no sedimentary rock: in 250 My about 90% of continental crust has had an orogeny, and an orogeny type never wears off. Land shares for seeds 1 to 3 were 0.31 to 0.42 igneous, 0.51 to 0.68 metamorphic, and 0.01 to 0.07 sedimentary. Two rules were added before the table, as the issue suggested:
  - An orogeny older than `RockOrogenyMaxAge` (150 My) counts as none, so old belts are sedimentary.
  - Continental crust whose `uplift` is above `RockArcUplift` (0.01 km/My) is igneous, as an active arc. The orogeny type alone cannot find arcs: it records only the first orogeny of crust above sea level, so crust raised behind a subduction front is often Himalayan or old Andean. Any uplift above 0 covers 37% to 45% of continental land, since uplift reaches `SubductionDistance` behind a front; its median there is 0.01 to 0.02 km/My.
- The noise: a continental sample takes the rock type c with the greatest −min(d_c, `RockReach`) + `RockNoiseAmplitude` · F_c, with d_c the distance to the nearest continental sample of base type c and F_c one noise field per type. Oceanic crust is not a source, so a coast is not made igneous. With 2 · `RockNoiseAmplitude` above `RockReach` the noise also makes patches inside the interiors. Defaults: amplitude 500 km, reach 300 km, feature size 1000 km.
- Tuned by rerunning the stage alone on saved full-scale worlds. Without the uplift rule, a cutoff of 150 My leaves 0.05 to 0.12 of land igneous.

Measured at full scale, seeds 1 to 3 (`yaee stats`, shares of land area):

| Seed | Igneous | Metamorphic | Sedimentary |
|------|---------|-------------|-------------|
| 1 | 0.34 | 0.24 | 0.42 |
| 2 | 0.22 | 0.21 | 0.58 |
| 3 | 0.29 | 0.22 | 0.50 |

A default run takes 36 to 45 s, as before. Every attribute of library version 1.0.0 is byte-identical for the same `Config`; `TestRockTypeAddsOnly` checks it, and a world of seed 2 at 150 km was compared with one from `v1.0.0`.

### 12. Erosion

Tracked by [#34](https://github.com/maloquacious/yaee/issues/34). Its three decisions are settled: see Decisions.

A stage after rock type erodes the land by rivers, with the stream power law (Cordonnier et al. 2016 §3.1, equation 1; Tzathas et al. 2024 §3):

	dz/dt = u − k · A^m · s^n

with z the elevation, u the uplift rate, A the drainage area (the area of the land upstream of a sample, its own included), s the slope to the sample's receiver (the neighbour its water flows to), and k, m, n constants. Both papers take n = 1, which makes the implicit step solvable in one pass from the sea upstream; m is 0.5 in the first and 0.4 in the second.

The stage writes new attributes and changes no existing one:

| Attribute | Type | Unit | Meaning |
|-----------|------|------|---------|
| `eroded_elevation` | float64 | km | The elevation after erosion. Equal to `elevation` where the sample is under the sea. |
| `drainage_area` | float64 | km² | A at the end of the stage. 0 under the sea. |
| `receiver` | int32 | none | The sample this sample's water flows to, or −1 for a sample under the sea. Following `receiver` from any land sample reaches the sea. |

Rivers are data a client can use, and the stage computes them anyway, so they are written; the stage does not draw or name rivers.

The reason for the stage is the client: Hāmwold uses only the order of heights, and wants mountain ranges with a direction, not massifs (`~/Software/maloquacious/tecto/PORT_PLAN.md`, "Ranges"). tecto found that erosion made its high ground longer for its width. Judge this milestone by that measure, not by how the eroded relief looks.

Where the papers are silent or do not fit, decide in this milestone and write it up in the code comment:

| Gap | Notes |
|-----|-------|
| The boundary | The papers erode a bounded domain whose edge is the outlet. A sphere has no edge: every sample under the sea is an outlet, and only land is eroded. |
| Depressions | Cordonnier §4.3 routes water out of lakes through passes in O(n + M log M). A priority flood from the sea, O(n log n), is the usual alternative. Either way every land sample must drain to the sea. |
| The uplift u | `uplift` is the rate of the last step only. On land its median is 0.01 to 0.02 km/My, and 37% to 45% of continental land has some (milestone 11). Choose between that, 0 (the stage wears down relief the simulation built), or a mean over the run, which the simulation does not keep. |
| k, m, and the time eroded | The papers' k is for 50 km domains at metre resolution. Ours must be stated in km and My, independent of the sample spacing, and tuned at 25 km. |
| Hillslope or thermal erosion | Cordonnier §5 limits slopes to 30°. At 25 km spacing slopes are far below that, so the limit may never act; measure it. |
| Erodibility by rock type | A factor on k for each value of `rock_type`, as `Config` fields. Which rock type erodes fastest, and by how much, is tuned here. |
| What "high ground" and "elongated" mean | Define them in `yaee stats` before tuning, following tecto: the share of land above a height, the connected regions of it, and their length for their width. Length and width are measured on the sphere, in km. |

Done when:

- The stage implements `model.Stage`, runs after `rocks.RockType`, and draws from its own random stream, if it draws at all. Every attribute written before this milestone is byte-identical for a given `Config` with and without it; a test checks it.
- Tests define correctness:
  - A river of constant uplift and constant k reaches the analytical steady state of the stream power law with n = 1, s = (u / k) · A^(−m), within a stated tolerance.
  - Following `receiver` from every land sample reaches the sea without a cycle, and `drainage_area` at each outlet is the area of the land that drains to it.
  - With k = 0 and u = 0, `eroded_elevation` equals `elevation`. No sample under the sea changes.
  - The same physics at two sample spacings: on two grids, the eroded profile of one river agrees within a stated tolerance.
- The cost is O(n log n) per iteration or better, and a default run stays within the time budget of "Target"; state the stage's own time and memory at full scale.
- Every tuning number is a `Config` field with unit and default, or a constant whose comment says why it is not a field.
- `yaee stats` reports the high-ground measures, and the measured values for seeds 1 to 5, with and without erosion, are written down here. Erosion makes the high ground longer for its width, or the milestone says why not.
- `docs/datafile.md` specifies the three attributes, their ranges, and the rule exactly enough to recompute `drainage_area` from `receiver`. The example file and its listing are regenerated.
- `yaee render` draws the eroded elevation and the drainage, and the reviewer accepts the full-scale images of seeds 1 to 3.
- The new attributes are alpha, as `rock_type` is: a minor version may change them until tuned. The version is a minor bump, and is tagged.

## After this plan

Not planned in detail. Each needs its own plan when its turn comes.

- **Relief.** The paper's §5 amplification is on hold; see Decisions. Do not start it without being asked.
