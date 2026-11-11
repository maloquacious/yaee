# Datafile format

This document specifies version 1 of the `yaee` datafile.
It is complete: a reader can be written from it without any other source.

A datafile holds one generated world: the samples spread over a sphere, the triangulation that connects them, and named data for each sample.
The Go functions `yaee.Write` and `yaee.Read` write and read it.

The conventional file name extension is `.yaee`.
Nothing depends on it.

## Conventions

- The file is a sequence of bytes.
  Offsets and lengths are in bytes.
- Every multi-byte number is **little-endian**.
  There is no padding or alignment anywhere.
- The file is not compressed.
- The same world always encodes to the same bytes.
  The file holds no timestamp, host name, or other data that is not part of the world.

| Type | Width | Encoding |
|------|-------|----------|
| `u8` | 1 | Unsigned integer |
| `u32` | 4 | Unsigned integer, little-endian |
| `u64` | 8 | Unsigned integer, little-endian |
| `i32` | 4 | Signed two's complement integer, little-endian |
| `f32` | 4 | IEEE 754 binary32, little-endian |
| `f64` | 8 | IEEE 754 binary64, little-endian |
| `bytes[n]` | n | n bytes, as they are |

Lengths are in kilometres (km) and times in millions of years (My).

## Structure

A file is a 12-byte header followed by sections, the last of which has the kind `END `.

```text
file    = header section ... section
header  = identifier version
section = kind length payload checksum
```

### Header

| Offset | Width | Type | Field | Value |
|--------|-------|------|-------|-------|
| 0 | 8 | `bytes[8]` | identifier | The ASCII text `YAEEDATA`: hexadecimal `59 41 45 45 44 41 54 41` |
| 8 | 4 | `u32` | format version | `1` |

### Section

Every section has the same envelope.
Offsets are from the start of the section; L is the payload length.

| Offset | Width | Type | Field | Meaning |
|--------|-------|------|-------|---------|
| 0 | 4 | `bytes[4]` | kind | Four ASCII characters naming what the payload holds |
| 4 | 8 | `u64` | length | L, the number of bytes in the payload. May be 0 |
| 12 | L | `bytes[L]` | payload | Laid out as the kind requires |
| 12 + L | 4 | `u32` | checksum | CRC-32 of the 12 + L bytes before it: kind, length, and payload |

The next section, if any, starts at offset 16 + L.

The checksum is the CRC-32 used by zlib, gzip, and PNG (CRC-32/ISO-HDLC): polynomial `0x04C11DB7`, reflected input and output (reflected polynomial `0xEDB88320`), initial value `0xFFFFFFFF`, final XOR `0xFFFFFFFF`.
The CRC-32 of the nine ASCII bytes `123456789` is `0xCBF43926`.
It is what `zlib.crc32` returns in Python, `crc32.ChecksumIEEE` in Go, and `java.util.zip.CRC32` in Java.

### Section kinds and their order

A version 1 file written by this library has exactly these sections, in this order:

| Order | Kind | Hexadecimal | Count | Payload |
|-------|------|-------------|-------|---------|
| 1 | `META` | `4d 45 54 41` | 1 | The library version and the configuration, as JSON |
| 2 | `SMPL` | `53 4d 50 4c` | 1 | Sample positions |
| 3 | `TRIS` | `54 52 49 53` | 1 | Triangles |
| 4 | `ATTR` | `41 54 54 52` | 0 or more | One per-sample attribute each |
| 5 | `END ` | `45 4e 44 20` | 1 | Empty. The fourth character is a space |

A later version of the library may add sections of other kinds, anywhere between the header and `END `, without changing the format version.
A reader must skip a section whose kind it does not know; see "Reading a file".

## Section payloads

### `META`

The payload is one JSON object, encoded as UTF-8, with no byte order mark and no trailing newline.

| Member | JSON type | Meaning |
|--------|-----------|---------|
| `library_version` | string | The version of the `yaee` library that generated the world, as a semantic version such as `0.17.0-alpha` |
| `config` | object | The configuration that generated the world |

Members of `config`:

| Member | JSON type | Unit | Range | Meaning |
|--------|-----------|------|-------|---------|
| `seed` | integer | none | 0 to 18446744073709551615 (2^64 − 1) | The seed of all randomness |
| `radius_km` | number | km | greater than 0 | Radius of the sphere at sea level |
| `sample_spacing_km` | number | km | greater than 0 | Distance between neighbouring samples |
| `duration_my` | number | My | at least 0, and at most 1,000,000 × `time_step_my` | Span of time simulated. 0 gives the initial world, in which no plate has moved |
| `time_step_my` | number | My | greater than 0 | Length of one simulation step |
| `plates` | integer | none | 1 to 1000, and at most N | Number of tectonic plates at the start |
| `plate_warp_amplitude_km` | number | km | at least 0 | How far the boundaries of the initial plates are bent away from the straight boundaries of Voronoi cells |
| `plate_warp_feature_size_km` | number | km | greater than 0 | Length of the largest bends in the boundaries of the initial plates |
| `land_coverage` | number | none; a fraction | 0 to 1 | Share of the sphere's area that is continental crust at the start |
| `ridge_elevation_km` | number | km | less than 0, greater than `abyssal_elevation_km` | Highest elevation of oceanic crust: that of a mid-ocean ridge |
| `abyssal_elevation_km` | number | km | less than `ridge_elevation_km`, greater than `trench_elevation_km` | Elevation of the abyssal plains |
| `trench_elevation_km` | number | km | less than `abyssal_elevation_km` | Elevation of the deepest oceanic trench. No elevation is below it |
| `max_continental_elevation_km` | number | km | greater than 0 | Highest elevation of continental crust. No elevation is above it |
| `max_plate_speed_km_per_my` | number | km/My | at least 0 | Greatest speed of any point of a plate over the surface. 1 km/My equals 1 mm/y |
| `oceanic_dampening_km_per_my` | number | km/My | at least 0 | Rate at which oceanic crust at sea level sinks as it ages. See "After the start" under "Attributes written" |
| `subduction_distance_km` | number | km | at least 0 | How far behind a subduction front the overriding plate is raised. 0 gives no uplift |
| `subduction_uplift_km_per_my` | number | km/My | at least 0 | Base rate at which subduction raises the overriding plate. 0 gives no uplift |
| `continental_erosion_km_per_my` | number | km/My | at least 0 | Rate at which continental crust at `max_continental_elevation_km` is eroded. 0 gives no erosion |
| `sediment_accretion_km_per_my` | number | km/My | at least 0 | Rate at which sediment fills an oceanic trench that is no longer being formed. 0 gives trenches that stay |
| `collision_penetration_km` | number | km | at least 0 | How far the continental crust of one plate goes beneath that of another before the two collide. See "After the start" under "Attributes written" |
| `collision_distance_km` | number | km | at least 0 | Greatest distance from a continental collision at which land is raised. 0 gives collisions that raise nothing |
| `collision_coefficient_per_km` | number | 1/km | at least 0 | Rise in km at the suture of a continental collision per km² of the continent that changes plates. 0 gives collisions that raise nothing |

`seed` can exceed 2^53.
A JSON parser that reads every number as an IEEE 754 double corrupts such a seed; use a parser that keeps 64-bit integers if the seed matters to you.

A later version of the library may add members to either object without changing the format version.
A reader must ignore members it does not know.

The number of samples is in the `SMPL` section.
Do not compute it from `config`.

### `SMPL`

| Offset | Width | Type | Field | Meaning |
|--------|-------|------|-------|---------|
| 0 | 4 | `u32` | N | The number of samples. At most 357,913,941 |
| 4 | 24 × N | `f64` × 3 × N | positions | For each sample in order: x, y, z |

The payload length is exactly 4 + 24 × N.

Samples are numbered from 0 to N − 1 in the order stored.
The x coordinate of sample i is at payload offset 4 + 24 × i, y at 12 + 24 × i, and z at 20 + 24 × i.

A position is a unit vector from the centre of the sphere: x² + y² + z² = 1 to within 1e-12.
It has no unit.

- The +z axis passes through the north pole.
- Latitude in radians is `asin(z)`, from −π/2 at the south pole to π/2 at the north pole.
- Longitude in radians is `atan2(y, x)`, from −π to π, positive toward the east.
  Longitude 0 is the half-plane y = 0, x > 0.
- The sample's point at sea level, in km from the centre, is `radius_km` × (x, y, z).
- The sample's point on the surface, in km from the centre, is (`radius_km` + elevation) × (x, y, z), where elevation is the sample's value of the `elevation` attribute.

Sample 0 is the nearest to the north pole and sample N − 1 the nearest to the south pole; z strictly decreases as the sample number increases.
The samples are spread evenly: each represents the same area, 4π × `radius_km`² / N km².

### `TRIS`

| Offset | Width | Type | Field | Meaning |
|--------|-------|------|-------|---------|
| 0 | 4 | `u32` | T | The number of triangles |
| 4 | 12 × T | `u32` × 3 × T | corners | For each triangle in order: the sample numbers of its three corners |

The payload length is exactly 4 + 12 × T.
Every corner is less than N.

The triangles are the Delaunay triangulation of the samples on the sphere.

- T = 2 × N − 4.
- The triangles cover the sphere with no gaps and no overlaps.
- The corners of each triangle are listed counter-clockwise as seen from outside the sphere.
- Every sample is a corner of at least three triangles.
- Every edge belongs to exactly two triangles.
  If one triangle has corners a then b in that cyclic order, the other has b then a.

**Neighbours.** Two samples are neighbours when some triangle has both as corners.
To build every sample's list of neighbours: for each triangle (a, b, c), add b and c to the set of a, a and c to the set of b, and a and b to the set of c.
The sets together hold 6 × N − 12 entries, an average of just under 6 per sample.

### `ATTR`

Each `ATTR` section holds one attribute: one named kind of data, with a value for every sample.

| Order | Width | Type | Field | Meaning |
|-------|-------|------|-------|---------|
| 1 | 1 | `u8` | name length | A, from 1 to 255 |
| 2 | A | `bytes[A]` | name | UTF-8, not terminated. Unique within the file |
| 3 | 1 | `u8` | unit length | U, from 0 to 255 |
| 4 | U | `bytes[U]` | unit | UTF-8, not terminated. Empty when the values have no unit |
| 5 | 1 | `u8` | element type | A code from the table below |
| 6 | 1 | `u8` | components | C, from 1 to 255: the number of elements per sample |
| 7 | S × C × N | element × C × N | data | For each sample in order, its C elements |

Element types:

| Code | Name | S, bytes per element | Encoding |
|------|------|----------------------|----------|
| 1 | uint8 | 1 | `u8` |
| 2 | int32 | 4 | `i32` |
| 3 | float32 | 4 | `f32` |
| 4 | float64 | 8 | `f64` |

The payload length is exactly 1 + A + 1 + U + 2 + S × C × N.
The data begins at payload offset D = 4 + A + U.
Component k (from 0) of sample i is at payload offset D + S × (C × i + k).

A scalar attribute has C = 1.
A vector attribute has C = 3 and stores x, y, z on the same axes as the positions.

`SMPL` comes before every `ATTR`, so N is known when an attribute is read.

A later version of the library may add attributes, and may add element type codes, without changing the format version.

- An attribute with a name the reader does not know is valid data.
  A reader that has no use for it skips the section.
- An attribute with an element type code the reader does not know cannot be decoded, because S is unknown.
  The reader must skip the section, using the section's length, and carry on.

#### Attributes written

Library version 0.17.0-alpha writes these attributes, in this order:

| Name | Element type | Components | Unit | Range | Meaning |
|------|--------------|------------|------|-------|---------|
| `plate` | 2 (int32) | 1 | none (empty) | 0 to `plates` − 1 | The number of the tectonic plate the sample belongs to |
| `crust_type` | 1 (uint8) | 1 | none (empty) | 0 or 1 | Type of the crust: 0 is oceanic, 1 is continental |
| `thickness` | 4 (float64) | 1 | `km` | greater than 0 | Thickness of the crust |
| `elevation` | 4 (float64) | 1 | `km` | `trench_elevation_km` to `max_continental_elevation_km` | Height of the surface above sea level. Negative values are below sea level; 0 is sea level |
| `oceanic_age` | 4 (float64) | 1 | `My` | at least 0 | Time since oceanic crust formed at a ridge. 0 for continental crust |
| `ridge_direction` | 4 (float64) | 3 | none (empty) | each component −1 to 1 | Direction of the ridge at which oceanic crust formed, as a vector (x, y, z) along the ridge |
| `orogeny_age` | 4 (float64) | 1 | `My` | at least 0 | Time since the mountain building that last affected the sample began. 0 where `orogeny_type` is 0 |
| `orogeny_type` | 1 (uint8) | 1 | none (empty) | 0, 1, or 2 | Kind of mountain building that last affected the sample: 0 is none, 1 is Andean (an oceanic plate subducts beneath the sample's plate), 2 is Himalayan (two continents collide) |
| `fold_direction` | 4 (float64) | 3 | none (empty) | each component finite | Direction in which the crust is folded, as a vector (x, y, z) |
| `uplift` | 4 (float64) | 1 | `km/My` | 0 to 2 × `subduction_uplift_km_per_my` | Rate at which subduction raised the sample during the last step of the simulation. 0 where nothing was raised |

Names in code font in the Range column, and below, are members of `config` in the `META` section.
No value of any attribute is NaN or infinite.

`plate`:

- In a world whose plates have not moved (`duration_my` 0, or `max_plate_speed_km_per_my` 0) every plate number from 0 to `plates` − 1 is used by at least one sample, and the samples of one plate form a single connected region: any two of them are joined by a path of neighbouring samples of the same plate. Neighbours are defined under `TRIS`.
- Once plates have moved, neither holds. A plate may be in several pieces, and a plate number may be used by no sample. Do not assume either.
- A plate boundary runs between two neighbouring samples whose plate numbers differ. No sample lies on a boundary.

`crust_type`:

- A coast runs between two neighbouring samples whose crust types differ. No sample lies on a coast.
- Continental crust may lie below sea level, and oceanic crust that subduction has raised may lie above it: `crust_type` says what the crust is made of, and `elevation` says whether it is under water. Do not derive one from the other.
- Each sample stands for a share of the sphere's area. The share of continental samples differs from `land_coverage` by at most 1.5/N in the initial world. It changes as plates move.

`ridge_direction` and `fold_direction`:

- The components are on the same axes as the positions in `SAMP`.
- A vector is either (0, 0, 0), meaning no direction is known, or perpendicular to the sample's position.
- A `ridge_direction` that is not (0, 0, 0) has length 1. It is (0, 0, 0) for continental crust, and for oceanic crust that did not form at a ridge during the run: the initial ocean, and crust that closed a hole within one plate.
- A `ridge_direction` and its negative mean the same ridge. The sign is that of (**p** − **q**) × **p**, with **p** the sample's position when the crust formed and **q** the nearest point of the ridge, so it is opposite on the two sides of a ridge.
- A `fold_direction` that is not (0, 0, 0) may have any length. It points the way the subducting plate moved relative to the sample's plate, and its length is the distance the two plates converged while the sample was behind a subduction front, in units of 100 km.

##### At the start

The initial world (`duration_my` 0) holds the initial crust.
With d the distance in km, along the surface, from a sample to the nearest sample that has a neighbour of the other crust type (0 for such a sample; infinite if the world has one crust type), and s(x) = x² × (3 − 2x) for x from 0 to 1 and s(x) = 1 for x greater than 1:

| Attribute | Oceanic sample | Continental sample |
|-----------|----------------|--------------------|
| `thickness` | 7 | 35 |
| `elevation` | `ridge_elevation_km` + (`abyssal_elevation_km` − `ridge_elevation_km`) × s(d / 500) | `max_continental_elevation_km` × (0.01 + 0.04 × s(d / 500)) |
| `oceanic_age` | (`trench_elevation_km` / `oceanic_dampening_km_per_my`) × ln((`abyssal_elevation_km` − `trench_elevation_km`) / (`ridge_elevation_km` − `trench_elevation_km`)); 202.7325540540822 with the default configuration; 0 if `oceanic_dampening_km_per_my` is 0 | 0 |
| `ridge_direction` | (0, 0, 0) | (0, 0, 0) |
| `orogeny_age` | 0 | 0 |
| `orogeny_type` | 0 | 0 |
| `fold_direction` | (0, 0, 0) | (0, 0, 0) |
| `uplift` | 0 | 0 |

With the default configuration the initial `elevation` is therefore −6 to −1 for oceanic samples and 0.1 to 0.5 for continental samples.

##### After the start

The simulation runs for `duration_my` in steps of `time_step_my`; if `duration_my` is not a multiple of `time_step_my`, the last step is the shorter remainder.
In this library version the plates move as rigid bodies, carrying their crust; oceanic crust ages and sinks; new oceanic crust forms where plates part; where plates meet, one subducts beneath the other, which is raised; trenches form along subduction fronts; continents that meet collide and join; and land erodes.
Plates do not yet split.

Where crust of two plates overlaps, one plate's crust subducts and is discarded; the other plate is the overriding plate there:

- Oceanic crust subducts beneath continental crust.
- Of two oceanic crusts, that of the plate whose oceanic crust is older on average subducts; at equal ages, that of the higher-numbered plate.
- Of two continental crusts, that of the higher-numbered plate subducts, until the two collide (below).

A subduction front is the edge of an overriding plate along which another plate moves beneath it.

Every step of length δt My, for every sample of oceanic crust:

- `oceanic_age` increases by δt.
- `elevation` z becomes z − (1 − z / `trench_elevation_km`) × `oceanic_dampening_km_per_my` × δt.
  It approaches `trench_elevation_km` and never passes it.
  Crust that formed at `ridge_elevation_km` is, t My later, within 0.02 km of `trench_elevation_km` + (`ridge_elevation_km` − `trench_elevation_km`) × exp(−`oceanic_dampening_km_per_my` × t / |`trench_elevation_km`|) with the default configuration: −1 at 0 My, −3.97 at 100 My, −6.69 at 250 My.

Every step, for every sample whose `orogeny_type` is not 0, `orogeny_age` increases by δt.

Every step, for every sample of an overriding plate, of either crust type, that is less than `subduction_distance_km` behind a subduction front of its plate, measured through the plate:

- `uplift` becomes u = `subduction_uplift_km_per_my` × f × g × h × (1 − max(z, 0) / `max_continental_elevation_km`), with z the sample's `elevation` and:
  - f from the distance d in km to the front and d₀ = `subduction_distance_km` / 6: with s(x) = x² × (3 − 2x), f = s(d / d₀) for d up to d₀ and f = 1 − s((d − d₀) / (`subduction_distance_km` − d₀)) beyond it. f is 0 at the front, 1 at d₀, and 0 at `subduction_distance_km`;
  - g = v / `max_plate_speed_km_per_my`, with v the speed in km/My at which the two plates move past each other at the sample. g is from 0 to 2;
  - h = ((z_s − `trench_elevation_km`) / (`max_continental_elevation_km` − `trench_elevation_km`))², with z_s the `elevation` of the subducting crust at the nearest point of the front, without its trench.
- `elevation` becomes z + u × δt, and at most `max_continental_elevation_km`.
- `fold_direction` gains 0.01 × δt × (the velocity in km/My of the subducting plate relative to the sample's plate), and is then made perpendicular to the sample's position.
- If u is greater than 0 and the new `elevation` is above 0, and either the old `elevation` was not above 0 or `orogeny_type` is 0, then `orogeny_type` becomes 1 and `orogeny_age` 0. `crust_type` does not change.

`uplift` is 0 for every other sample.

Every step, for every sample of oceanic crust of a subducting plate that is less than 150 km in front of a subduction front it is passing beneath, at the distance d km: the sample is lowered into a trench, to z_t + (z − z_t) × (1 − (1 − d / 150)²), with z_t = `trench_elevation_km` and z its `elevation` without the trench.
At the front the trench is at `trench_elevation_km`.
A trench that is no longer within 150 km of a front rises by `sediment_accretion_km_per_my` × δt each step until the crust is back at its elevation without the trench.

Every step, for every sample of continental crust whose `elevation` z is above 0: z becomes z × max(0, 1 − `continental_erosion_km_per_my` × δt / `max_continental_elevation_km`).
Land that is not raised is, t My later, at z × exp(−`continental_erosion_km_per_my` × t / `max_continental_elevation_km`), within 0.005 km with the default configuration; it never falls below 0.
Continental crust at or below 0 and oceanic crust are not eroded.

For each pair of plates, the distance D in km by which their continental crusts have gone into each other is tracked.
It starts at 0.
Every step in which continental crust of one plate moves beneath continental crust of the other, D grows by c × δt, with c the greatest speed in km/My at which the one moves into the edge of the other.
When D is at least `collision_penetration_km`, the plates collide, in that step:

- The continents in contact are the connected regions of continental crust, one or more on each plate, that touch where the plates meet.
  Those of the plate on which their total area is smaller leave that plate and join the other; if the areas are equal, those of the higher-numbered plate do.
  `plate` changes for their samples, and for samples of the same plate that they enclose. No crust is created or removed.
- For each such region, with area a in km², centre q (the mean position of its samples), and v the speed in km/My at which the two plates move past each other at q:
  - r = `collision_distance_km` × min(1, √((v / `max_plate_speed_km_per_my`) × (a / a₀))), with a₀ = 4π × `radius_km`² / `plates`.
  - s = `collision_coefficient_per_km` × a, and s′ = `max_continental_elevation_km` × (1 − exp(−s / `max_continental_elevation_km`)).
  - Every sample of continental crust less than r km from the suture, on either side of it, is raised: its `elevation` z becomes z + s′ × (1 − (d / r)²)² × (1 − max(z, 0) / `max_continental_elevation_km`). For a sample of the plate that the region joined, d is its distance in km from the region; for a sample of the region, d is its distance from the continental crust of that plate.
  - For each sample raised, `orogeny_type` becomes 2, `orogeny_age` becomes 0, and `fold_direction` becomes the unit vector from q toward the sample, made perpendicular to the sample's position (its length is then cos(θ / 2), with θ the angle between q and the sample).
- D returns to 0.

D also returns to 0 when the two continents stop converging.
A region of continental crust narrower than `collision_penetration_km` is consumed before it can collide.
`uplift` does not include the rise that a collision gives.

Every step, the rotation axis of each subducting plate turns toward its subduction fronts (slab pull).
The speed of the plate's fastest point does not change, so no point of a plate moves faster than `max_plate_speed_km_per_my`.

With `max_plate_speed_km_per_my` 0 no plate moves and nothing subducts: the world is the initial world with its oceanic crust aged and lowered, and its land eroded, by `duration_my`.

In the world that results:

- A sample's values are those of the crust that has moved to its position, interpolated between the up to three samples that crust came from.
  Oceanic and continental crust are never blended with each other: a coast is a step in `elevation` and `thickness`.
- Where plates have moved apart, the gap holds oceanic crust that formed at the ridge midway between them: `crust_type` 0, `thickness` 7, `orogeny_type` 0, `orogeny_age` 0, `fold_direction` (0, 0, 0), `uplift` 0.
  Its `oceanic_age` is 0 on the ridge and grows with distance from it, at the rate of 1 My for each (speed at which the plate leaves the ridge, in km/My) km.
  Its `elevation` is `ridge_elevation_km` on the ridge and falls with age as above; when it forms it is never above 0.
  Its `ridge_direction` has length 1.
  The ridge is the boundary between the two plates until they move again.
- Where plates have moved over one another, the crust of the overriding plate is kept and the other's is discarded.
- `uplift` is the value of the last step, interpolated like every other value. It is not a mean over the run.
- `orogeny_age` is interpolated among the source samples that have an `orogeny_type` other than 0.
- `oceanic_age` is at most its initial value plus `duration_my`.
- The crust of the initial ocean keeps sinking: with the default configuration it is 453 My old and at about −8.5 km after 250 My. `abyssal_elevation_km` is not a lower limit.

## Size

For N samples the file is 12 + (16 + J) + (20 + 24 × N) + (20 + 12 × (2 × N − 4)) + 16 bytes, where J is the length of the `META` JSON, plus 16 + 4 + A + U + S × C × N for each attribute.
The default world has N = 942,059 and, with the attributes that library version 0.17.0-alpha writes, is 133,773,268 bytes.

## Example

The file `testdata/example.yaee` in the repository is 1843 bytes.
It is the world generated by library version 0.17.0-alpha from this configuration, which gives N = 6 samples and T = 8 triangles, and, because `duration_my` is 0, the initial world:

```json
{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.04,"subduction_distance_km":1800,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.03,"sediment_accretion_km_per_my":0.3,"collision_penetration_km":300,"collision_distance_km":4200,"collision_coefficient_per_km":0.000013}
```

Its `META` payload is these 640 bytes of text:

```json
{"library_version":"0.17.0-alpha","config":{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.04,"subduction_distance_km":1800,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.03,"sediment_accretion_km_per_my":0.3,"collision_penetration_km":300,"collision_distance_km":4200,"collision_coefficient_per_km":0.000013}}
```

Every byte of the file is listed below.
The first column is the offset in hexadecimal, the second the bytes in hexadecimal, and the third the field they encode.
A field longer than 16 bytes continues on the following lines.

<!-- example:begin -->
```text
0000  59 41 45 45 44 41 54 41                          identifier "YAEEDATA"
0008  01 00 00 00                                      format version = 1

000c  4d 45 54 41                                      section kind "META"
0010  80 02 00 00 00 00 00 00                          payload length = 640
0018  7b 22 6c 69 62 72 61 72 79 5f 76 65 72 73 69 6f  payload: the JSON text shown above
0028  6e 22 3a 22 30 2e 31 37 2e 30 2d 61 6c 70 68 61
0038  22 2c 22 63 6f 6e 66 69 67 22 3a 7b 22 73 65 65
0048  64 22 3a 34 32 2c 22 72 61 64 69 75 73 5f 6b 6d
0058  22 3a 36 33 37 30 2c 22 73 61 6d 70 6c 65 5f 73
0068  70 61 63 69 6e 67 5f 6b 6d 22 3a 31 30 30 30 30
0078  2c 22 64 75 72 61 74 69 6f 6e 5f 6d 79 22 3a 30
0088  2c 22 74 69 6d 65 5f 73 74 65 70 5f 6d 79 22 3a
0098  32 2c 22 70 6c 61 74 65 73 22 3a 32 2c 22 70 6c
00a8  61 74 65 5f 77 61 72 70 5f 61 6d 70 6c 69 74 75
00b8  64 65 5f 6b 6d 22 3a 31 35 30 30 2c 22 70 6c 61
00c8  74 65 5f 77 61 72 70 5f 66 65 61 74 75 72 65 5f
00d8  73 69 7a 65 5f 6b 6d 22 3a 32 35 30 30 2c 22 6c
00e8  61 6e 64 5f 63 6f 76 65 72 61 67 65 22 3a 30 2e
00f8  33 2c 22 72 69 64 67 65 5f 65 6c 65 76 61 74 69
0108  6f 6e 5f 6b 6d 22 3a 2d 31 2c 22 61 62 79 73 73
0118  61 6c 5f 65 6c 65 76 61 74 69 6f 6e 5f 6b 6d 22
0128  3a 2d 36 2c 22 74 72 65 6e 63 68 5f 65 6c 65 76
0138  61 74 69 6f 6e 5f 6b 6d 22 3a 2d 31 30 2c 22 6d
0148  61 78 5f 63 6f 6e 74 69 6e 65 6e 74 61 6c 5f 65
0158  6c 65 76 61 74 69 6f 6e 5f 6b 6d 22 3a 31 30 2c
0168  22 6d 61 78 5f 70 6c 61 74 65 5f 73 70 65 65 64
0178  5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a 31 30 30 2c
0188  22 6f 63 65 61 6e 69 63 5f 64 61 6d 70 65 6e 69
0198  6e 67 5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a 30 2e
01a8  30 34 2c 22 73 75 62 64 75 63 74 69 6f 6e 5f 64
01b8  69 73 74 61 6e 63 65 5f 6b 6d 22 3a 31 38 30 30
01c8  2c 22 73 75 62 64 75 63 74 69 6f 6e 5f 75 70 6c
01d8  69 66 74 5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a 30
01e8  2e 36 2c 22 63 6f 6e 74 69 6e 65 6e 74 61 6c 5f
01f8  65 72 6f 73 69 6f 6e 5f 6b 6d 5f 70 65 72 5f 6d
0208  79 22 3a 30 2e 30 33 2c 22 73 65 64 69 6d 65 6e
0218  74 5f 61 63 63 72 65 74 69 6f 6e 5f 6b 6d 5f 70
0228  65 72 5f 6d 79 22 3a 30 2e 33 2c 22 63 6f 6c 6c
0238  69 73 69 6f 6e 5f 70 65 6e 65 74 72 61 74 69 6f
0248  6e 5f 6b 6d 22 3a 33 30 30 2c 22 63 6f 6c 6c 69
0258  73 69 6f 6e 5f 64 69 73 74 61 6e 63 65 5f 6b 6d
0268  22 3a 34 32 30 30 2c 22 63 6f 6c 6c 69 73 69 6f
0278  6e 5f 63 6f 65 66 66 69 63 69 65 6e 74 5f 70 65
0288  72 5f 6b 6d 22 3a 30 2e 30 30 30 30 31 33 7d 7d
0298  db d3 d8 b7                                      CRC-32 of the 652 bytes from offset 000c = b7d8d3db

029c  53 4d 50 4c                                      section kind "SMPL"
02a0  94 00 00 00 00 00 00 00                          payload length = 148
02a8  06 00 00 00                                      N = 6 samples
02ac  cd f4 a8 62 4c b0 e1 3f                          sample 0 x = 0.5527707983925666
02b4  00 00 00 00 00 00 00 00                          sample 0 y = 0
02bc  ab aa aa aa aa aa ea 3f                          sample 0 z = 0.8333333333333334
02c4  8d d1 d2 b3 3f 6f e4 bf                          sample 1 x = -0.6385801803758554
02cc  17 00 ef a0 40 b8 e2 bf                          sample 1 y = -0.5849917548403053
02d4  00 00 00 00 00 00 e0 3f                          sample 1 z = 0.5
02dc  2a 45 a8 20 65 11 b6 3f                          sample 2 x = 0.08620292708680624
02e4  29 36 2e 29 7e 6e ef 3f                          sample 2 y = 0.9822378925616181
02ec  54 55 55 55 55 55 c5 3f                          sample 2 z = 0.16666666666666663
02f4  be 62 fd e5 9d 32 e3 3f                          sample 3 x = 0.5999288074482008
02fc  da ae 4b 50 3f 0a e9 bf                          sample 3 y = -0.7825008934282451
0304  58 55 55 55 55 55 c5 bf                          sample 3 z = -0.16666666666666674
030c  a3 33 61 bd 07 4a eb bf                          sample 4 x = -0.8527868937322761
0314  7c 4c 49 e9 eb 4e c3 3f                          sample 4 y = 0.15084599390920406
031c  00 00 00 00 00 00 e0 bf                          sample 4 z = -0.5
0324  66 a1 22 2d 8d d9 dd 3f                          sample 5 x = 0.46640328796140407
032c  21 af 85 f6 ed fc d2 3f                          sample 5 y = 0.29668759416994034
0334  aa aa aa aa aa aa ea bf                          sample 5 z = -0.8333333333333333
033c  c0 a8 d3 44                                      CRC-32 of the 160 bytes from offset 029c = 44d3a8c0

0340  54 52 49 53                                      section kind "TRIS"
0344  64 00 00 00 00 00 00 00                          payload length = 100
034c  08 00 00 00                                      T = 8 triangles
0350  00 00 00 00 05 00 00 00 02 00 00 00              triangle 0 = samples 0, 5, 2
035c  00 00 00 00 03 00 00 00 05 00 00 00              triangle 1 = samples 0, 3, 5
0368  05 00 00 00 03 00 00 00 04 00 00 00              triangle 2 = samples 5, 3, 4
0374  02 00 00 00 04 00 00 00 01 00 00 00              triangle 3 = samples 2, 4, 1
0380  04 00 00 00 03 00 00 00 01 00 00 00              triangle 4 = samples 4, 3, 1
038c  03 00 00 00 00 00 00 00 01 00 00 00              triangle 5 = samples 3, 0, 1
0398  05 00 00 00 04 00 00 00 02 00 00 00              triangle 6 = samples 5, 4, 2
03a4  01 00 00 00 00 00 00 00 02 00 00 00              triangle 7 = samples 1, 0, 2
03b0  3f 76 78 72                                      CRC-32 of the 112 bytes from offset 0340 = 7278763f

03b4  41 54 54 52                                      section kind "ATTR"
03b8  21 00 00 00 00 00 00 00                          payload length = 33
03c0  05                                               name length = 5
03c1  70 6c 61 74 65                                   name "plate"
03c6  00                                               unit length = 0
03c7                                                   unit ""
03c7  02                                               element type = 2 (int32)
03c8  01                                               components = 1
03c9  01 00 00 00                                      plate of sample 0 = 1
03cd  01 00 00 00                                      plate of sample 1 = 1
03d1  00 00 00 00                                      plate of sample 2 = 0
03d5  01 00 00 00                                      plate of sample 3 = 1
03d9  00 00 00 00                                      plate of sample 4 = 0
03dd  00 00 00 00                                      plate of sample 5 = 0
03e1  cb 98 75 80                                      CRC-32 of the 45 bytes from offset 03b4 = 807598cb

03e5  41 54 54 52                                      section kind "ATTR"
03e9  14 00 00 00 00 00 00 00                          payload length = 20
03f1  0a                                               name length = 10
03f2  63 72 75 73 74 5f 74 79 70 65                    name "crust_type"
03fc  00                                               unit length = 0
03fd                                                   unit ""
03fd  01                                               element type = 1 (uint8)
03fe  01                                               components = 1
03ff  00                                               crust_type of sample 0 = 0
0400  00                                               crust_type of sample 1 = 0
0401  01                                               crust_type of sample 2 = 1
0402  00                                               crust_type of sample 3 = 0
0403  00                                               crust_type of sample 4 = 0
0404  01                                               crust_type of sample 5 = 1
0405  d9 55 40 92                                      CRC-32 of the 32 bytes from offset 03e5 = 924055d9

0409  41 54 54 52                                      section kind "ATTR"
040d  3f 00 00 00 00 00 00 00                          payload length = 63
0415  09                                               name length = 9
0416  74 68 69 63 6b 6e 65 73 73                       name "thickness"
041f  02                                               unit length = 2
0420  6b 6d                                            unit "km"
0422  04                                               element type = 4 (float64)
0423  01                                               components = 1
0424  00 00 00 00 00 00 1c 40                          thickness of sample 0 = 7
042c  00 00 00 00 00 00 1c 40                          thickness of sample 1 = 7
0434  00 00 00 00 00 80 41 40                          thickness of sample 2 = 35
043c  00 00 00 00 00 00 1c 40                          thickness of sample 3 = 7
0444  00 00 00 00 00 00 1c 40                          thickness of sample 4 = 7
044c  00 00 00 00 00 80 41 40                          thickness of sample 5 = 35
0454  2c c8 e1 5a                                      CRC-32 of the 75 bytes from offset 0409 = 5ae1c82c

0458  41 54 54 52                                      section kind "ATTR"
045c  3f 00 00 00 00 00 00 00                          payload length = 63
0464  09                                               name length = 9
0465  65 6c 65 76 61 74 69 6f 6e                       name "elevation"
046e  02                                               unit length = 2
046f  6b 6d                                            unit "km"
0471  04                                               element type = 4 (float64)
0472  01                                               components = 1
0473  00 00 00 00 00 00 f0 bf                          elevation of sample 0 = -1
047b  00 00 00 00 00 00 f0 bf                          elevation of sample 1 = -1
0483  9a 99 99 99 99 99 b9 3f                          elevation of sample 2 = 0.1
048b  00 00 00 00 00 00 f0 bf                          elevation of sample 3 = -1
0493  00 00 00 00 00 00 f0 bf                          elevation of sample 4 = -1
049b  9a 99 99 99 99 99 b9 3f                          elevation of sample 5 = 0.1
04a3  bb 5d c4 cb                                      CRC-32 of the 75 bytes from offset 0458 = cbc45dbb

04a7  41 54 54 52                                      section kind "ATTR"
04ab  41 00 00 00 00 00 00 00                          payload length = 65
04b3  0b                                               name length = 11
04b4  6f 63 65 61 6e 69 63 5f 61 67 65                 name "oceanic_age"
04bf  02                                               unit length = 2
04c0  4d 79                                            unit "My"
04c2  04                                               element type = 4 (float64)
04c3  01                                               components = 1
04c4  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 0 = 202.7325540540822
04cc  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 1 = 202.7325540540822
04d4  00 00 00 00 00 00 00 00                          oceanic_age of sample 2 = 0
04dc  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 3 = 202.7325540540822
04e4  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 4 = 202.7325540540822
04ec  00 00 00 00 00 00 00 00                          oceanic_age of sample 5 = 0
04f4  cb b1 9d b9                                      CRC-32 of the 77 bytes from offset 04a7 = b99db1cb

04f8  41 54 54 52                                      section kind "ATTR"
04fc  a3 00 00 00 00 00 00 00                          payload length = 163
0504  0f                                               name length = 15
0505  72 69 64 67 65 5f 64 69 72 65 63 74 69 6f 6e     name "ridge_direction"
0514  00                                               unit length = 0
0515                                                   unit ""
0515  04                                               element type = 4 (float64)
0516  03                                               components = 3
0517  00 00 00 00 00 00 00 00                          ridge_direction x of sample 0 = 0
051f  00 00 00 00 00 00 00 00                          ridge_direction y of sample 0 = 0
0527  00 00 00 00 00 00 00 00                          ridge_direction z of sample 0 = 0
052f  00 00 00 00 00 00 00 00                          ridge_direction x of sample 1 = 0
0537  00 00 00 00 00 00 00 00                          ridge_direction y of sample 1 = 0
053f  00 00 00 00 00 00 00 00                          ridge_direction z of sample 1 = 0
0547  00 00 00 00 00 00 00 00                          ridge_direction x of sample 2 = 0
054f  00 00 00 00 00 00 00 00                          ridge_direction y of sample 2 = 0
0557  00 00 00 00 00 00 00 00                          ridge_direction z of sample 2 = 0
055f  00 00 00 00 00 00 00 00                          ridge_direction x of sample 3 = 0
0567  00 00 00 00 00 00 00 00                          ridge_direction y of sample 3 = 0
056f  00 00 00 00 00 00 00 00                          ridge_direction z of sample 3 = 0
0577  00 00 00 00 00 00 00 00                          ridge_direction x of sample 4 = 0
057f  00 00 00 00 00 00 00 00                          ridge_direction y of sample 4 = 0
0587  00 00 00 00 00 00 00 00                          ridge_direction z of sample 4 = 0
058f  00 00 00 00 00 00 00 00                          ridge_direction x of sample 5 = 0
0597  00 00 00 00 00 00 00 00                          ridge_direction y of sample 5 = 0
059f  00 00 00 00 00 00 00 00                          ridge_direction z of sample 5 = 0
05a7  92 07 59 35                                      CRC-32 of the 175 bytes from offset 04f8 = 35590792

05ab  41 54 54 52                                      section kind "ATTR"
05af  41 00 00 00 00 00 00 00                          payload length = 65
05b7  0b                                               name length = 11
05b8  6f 72 6f 67 65 6e 79 5f 61 67 65                 name "orogeny_age"
05c3  02                                               unit length = 2
05c4  4d 79                                            unit "My"
05c6  04                                               element type = 4 (float64)
05c7  01                                               components = 1
05c8  00 00 00 00 00 00 00 00                          orogeny_age of sample 0 = 0
05d0  00 00 00 00 00 00 00 00                          orogeny_age of sample 1 = 0
05d8  00 00 00 00 00 00 00 00                          orogeny_age of sample 2 = 0
05e0  00 00 00 00 00 00 00 00                          orogeny_age of sample 3 = 0
05e8  00 00 00 00 00 00 00 00                          orogeny_age of sample 4 = 0
05f0  00 00 00 00 00 00 00 00                          orogeny_age of sample 5 = 0
05f8  ae bc 7a 7f                                      CRC-32 of the 77 bytes from offset 05ab = 7f7abcae

05fc  41 54 54 52                                      section kind "ATTR"
0600  16 00 00 00 00 00 00 00                          payload length = 22
0608  0c                                               name length = 12
0609  6f 72 6f 67 65 6e 79 5f 74 79 70 65              name "orogeny_type"
0615  00                                               unit length = 0
0616                                                   unit ""
0616  01                                               element type = 1 (uint8)
0617  01                                               components = 1
0618  00                                               orogeny_type of sample 0 = 0
0619  00                                               orogeny_type of sample 1 = 0
061a  00                                               orogeny_type of sample 2 = 0
061b  00                                               orogeny_type of sample 3 = 0
061c  00                                               orogeny_type of sample 4 = 0
061d  00                                               orogeny_type of sample 5 = 0
061e  68 3c d7 22                                      CRC-32 of the 34 bytes from offset 05fc = 22d73c68

0622  41 54 54 52                                      section kind "ATTR"
0626  a2 00 00 00 00 00 00 00                          payload length = 162
062e  0e                                               name length = 14
062f  66 6f 6c 64 5f 64 69 72 65 63 74 69 6f 6e        name "fold_direction"
063d  00                                               unit length = 0
063e                                                   unit ""
063e  04                                               element type = 4 (float64)
063f  03                                               components = 3
0640  00 00 00 00 00 00 00 00                          fold_direction x of sample 0 = 0
0648  00 00 00 00 00 00 00 00                          fold_direction y of sample 0 = 0
0650  00 00 00 00 00 00 00 00                          fold_direction z of sample 0 = 0
0658  00 00 00 00 00 00 00 00                          fold_direction x of sample 1 = 0
0660  00 00 00 00 00 00 00 00                          fold_direction y of sample 1 = 0
0668  00 00 00 00 00 00 00 00                          fold_direction z of sample 1 = 0
0670  00 00 00 00 00 00 00 00                          fold_direction x of sample 2 = 0
0678  00 00 00 00 00 00 00 00                          fold_direction y of sample 2 = 0
0680  00 00 00 00 00 00 00 00                          fold_direction z of sample 2 = 0
0688  00 00 00 00 00 00 00 00                          fold_direction x of sample 3 = 0
0690  00 00 00 00 00 00 00 00                          fold_direction y of sample 3 = 0
0698  00 00 00 00 00 00 00 00                          fold_direction z of sample 3 = 0
06a0  00 00 00 00 00 00 00 00                          fold_direction x of sample 4 = 0
06a8  00 00 00 00 00 00 00 00                          fold_direction y of sample 4 = 0
06b0  00 00 00 00 00 00 00 00                          fold_direction z of sample 4 = 0
06b8  00 00 00 00 00 00 00 00                          fold_direction x of sample 5 = 0
06c0  00 00 00 00 00 00 00 00                          fold_direction y of sample 5 = 0
06c8  00 00 00 00 00 00 00 00                          fold_direction z of sample 5 = 0
06d0  78 68 10 90                                      CRC-32 of the 174 bytes from offset 0622 = 90106878

06d4  41 54 54 52                                      section kind "ATTR"
06d8  3f 00 00 00 00 00 00 00                          payload length = 63
06e0  06                                               name length = 6
06e1  75 70 6c 69 66 74                                name "uplift"
06e7  05                                               unit length = 5
06e8  6b 6d 2f 4d 79                                   unit "km/My"
06ed  04                                               element type = 4 (float64)
06ee  01                                               components = 1
06ef  00 00 00 00 00 00 00 00                          uplift of sample 0 = 0
06f7  00 00 00 00 00 00 00 00                          uplift of sample 1 = 0
06ff  00 00 00 00 00 00 00 00                          uplift of sample 2 = 0
0707  00 00 00 00 00 00 00 00                          uplift of sample 3 = 0
070f  00 00 00 00 00 00 00 00                          uplift of sample 4 = 0
0717  00 00 00 00 00 00 00 00                          uplift of sample 5 = 0
071f  b0 da e2 db                                      CRC-32 of the 75 bytes from offset 06d4 = dbe2dab0

0723  45 4e 44 20                                      section kind "END "
0727  00 00 00 00 00 00 00 00                          payload length = 0
072f  1f 11 8b 42                                      CRC-32 of the 12 bytes from offset 0723 = 428b111f
```
<!-- example:end -->

From the listing: sample 0 is at latitude asin(0.8333333333333334) = 0.9851 radians (56.44° north) and longitude atan2(0, 0.5527707983925666) = 0.
Its elevation is −1 km, so its point on the surface is 6369 × (0.5527707983925666, 0, 0.8333333333333334) km from the centre.
It belongs to plate 1, as do samples 1 and 3; samples 2, 4, and 5 belong to plate 0.
Samples 2 and 5 are continental crust, 35 km thick, at 0.1 km; the other four are oceanic crust, 7 km thick, 202.7325540540822 My old, at −1 km.
Every sample has a neighbour of the other crust type, so every sample is at its coastal elevation.
Its neighbours, from triangles 0, 1, 5, and 7, are samples 1, 2, 3, and 5.

This Go program reads the example and prints what the listing shows.
Run it from the repository root:

```go
package main

import (
	"fmt"
	"os"

	"github.com/maloquacious/yaee"
)

func main() {
	f, err := os.Open("testdata/example.yaee")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer f.Close()
	world, err := yaee.Read(f)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(world.Version, world.NumSamples(), world.NumTriangles())
	fmt.Println(world.Positions[0], world.Triangles[0])
	fmt.Println(world.Attribute("plate").Int32s)
	fmt.Println(world.Attribute("crust_type").Uint8s)
	fmt.Println(world.Attribute("elevation").Float64s)
	fmt.Println(world.Attribute("oceanic_age").Float64s[0])
	fmt.Println(world.Attribute("ridge_direction").Float64s[0:3])
}
```

Output:

```text
0.17.0-alpha 6 8
[0.5527707983925666 0 0.8333333333333334] [0 5 2]
[1 1 0 1 0 0]
[0 0 1 0 0 1]
[-1 -1 0.1 -1 -1 0.1]
202.7325540540822
[0 0 0]
```
