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
| `library_version` | string | The version of the `yaee` library that generated the world, as a semantic version such as `0.15.0-alpha` |
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

Library version 0.15.0-alpha writes these attributes, in this order:

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
In this library version the plates move as rigid bodies, carrying their crust; oceanic crust ages and sinks; new oceanic crust forms where plates part; where plates meet, one subducts beneath the other, which is raised; trenches form along subduction fronts; and land erodes.
Continents do not yet collide, and plates do not yet split.

Where crust of two plates overlaps, one plate's crust subducts and is discarded; the other plate is the overriding plate there:

- Oceanic crust subducts beneath continental crust.
- Of two oceanic crusts, that of the plate whose oceanic crust is older on average subducts; at equal ages, that of the higher-numbered plate.
- Of two continental crusts, that of the higher-numbered plate subducts. Continental crust is therefore lost during a run: with the default configuration the continental share of the samples falls from 0.30 to about 0.17.

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
The default world has N = 942,059 and, with the attributes that library version 0.15.0-alpha writes, is 133,773,268 bytes.

## Example

The file `testdata/example.yaee` in the repository is 1743 bytes.
It is the world generated by library version 0.15.0-alpha from this configuration, which gives N = 6 samples and T = 8 triangles, and, because `duration_my` is 0, the initial world:

```json
{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.04,"subduction_distance_km":1800,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.03,"sediment_accretion_km_per_my":0.3}
```

Its `META` payload is these 540 bytes of text:

```json
{"library_version":"0.15.0-alpha","config":{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.04,"subduction_distance_km":1800,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.03,"sediment_accretion_km_per_my":0.3}}
```

Every byte of the file is listed below.
The first column is the offset in hexadecimal, the second the bytes in hexadecimal, and the third the field they encode.
A field longer than 16 bytes continues on the following lines.

<!-- example:begin -->
```text
0000  59 41 45 45 44 41 54 41                          identifier "YAEEDATA"
0008  01 00 00 00                                      format version = 1

000c  4d 45 54 41                                      section kind "META"
0010  1c 02 00 00 00 00 00 00                          payload length = 540
0018  7b 22 6c 69 62 72 61 72 79 5f 76 65 72 73 69 6f  payload: the JSON text shown above
0028  6e 22 3a 22 30 2e 31 35 2e 30 2d 61 6c 70 68 61
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
0228  65 72 5f 6d 79 22 3a 30 2e 33 7d 7d
0234  4b 32 a4 a3                                      CRC-32 of the 552 bytes from offset 000c = a3a4324b

0238  53 4d 50 4c                                      section kind "SMPL"
023c  94 00 00 00 00 00 00 00                          payload length = 148
0244  06 00 00 00                                      N = 6 samples
0248  cd f4 a8 62 4c b0 e1 3f                          sample 0 x = 0.5527707983925666
0250  00 00 00 00 00 00 00 00                          sample 0 y = 0
0258  ab aa aa aa aa aa ea 3f                          sample 0 z = 0.8333333333333334
0260  8d d1 d2 b3 3f 6f e4 bf                          sample 1 x = -0.6385801803758554
0268  17 00 ef a0 40 b8 e2 bf                          sample 1 y = -0.5849917548403053
0270  00 00 00 00 00 00 e0 3f                          sample 1 z = 0.5
0278  2a 45 a8 20 65 11 b6 3f                          sample 2 x = 0.08620292708680624
0280  29 36 2e 29 7e 6e ef 3f                          sample 2 y = 0.9822378925616181
0288  54 55 55 55 55 55 c5 3f                          sample 2 z = 0.16666666666666663
0290  be 62 fd e5 9d 32 e3 3f                          sample 3 x = 0.5999288074482008
0298  da ae 4b 50 3f 0a e9 bf                          sample 3 y = -0.7825008934282451
02a0  58 55 55 55 55 55 c5 bf                          sample 3 z = -0.16666666666666674
02a8  a3 33 61 bd 07 4a eb bf                          sample 4 x = -0.8527868937322761
02b0  7c 4c 49 e9 eb 4e c3 3f                          sample 4 y = 0.15084599390920406
02b8  00 00 00 00 00 00 e0 bf                          sample 4 z = -0.5
02c0  66 a1 22 2d 8d d9 dd 3f                          sample 5 x = 0.46640328796140407
02c8  21 af 85 f6 ed fc d2 3f                          sample 5 y = 0.29668759416994034
02d0  aa aa aa aa aa aa ea bf                          sample 5 z = -0.8333333333333333
02d8  c0 a8 d3 44                                      CRC-32 of the 160 bytes from offset 0238 = 44d3a8c0

02dc  54 52 49 53                                      section kind "TRIS"
02e0  64 00 00 00 00 00 00 00                          payload length = 100
02e8  08 00 00 00                                      T = 8 triangles
02ec  00 00 00 00 05 00 00 00 02 00 00 00              triangle 0 = samples 0, 5, 2
02f8  00 00 00 00 03 00 00 00 05 00 00 00              triangle 1 = samples 0, 3, 5
0304  05 00 00 00 03 00 00 00 04 00 00 00              triangle 2 = samples 5, 3, 4
0310  02 00 00 00 04 00 00 00 01 00 00 00              triangle 3 = samples 2, 4, 1
031c  04 00 00 00 03 00 00 00 01 00 00 00              triangle 4 = samples 4, 3, 1
0328  03 00 00 00 00 00 00 00 01 00 00 00              triangle 5 = samples 3, 0, 1
0334  05 00 00 00 04 00 00 00 02 00 00 00              triangle 6 = samples 5, 4, 2
0340  01 00 00 00 00 00 00 00 02 00 00 00              triangle 7 = samples 1, 0, 2
034c  3f 76 78 72                                      CRC-32 of the 112 bytes from offset 02dc = 7278763f

0350  41 54 54 52                                      section kind "ATTR"
0354  21 00 00 00 00 00 00 00                          payload length = 33
035c  05                                               name length = 5
035d  70 6c 61 74 65                                   name "plate"
0362  00                                               unit length = 0
0363                                                   unit ""
0363  02                                               element type = 2 (int32)
0364  01                                               components = 1
0365  01 00 00 00                                      plate of sample 0 = 1
0369  01 00 00 00                                      plate of sample 1 = 1
036d  00 00 00 00                                      plate of sample 2 = 0
0371  01 00 00 00                                      plate of sample 3 = 1
0375  00 00 00 00                                      plate of sample 4 = 0
0379  00 00 00 00                                      plate of sample 5 = 0
037d  cb 98 75 80                                      CRC-32 of the 45 bytes from offset 0350 = 807598cb

0381  41 54 54 52                                      section kind "ATTR"
0385  14 00 00 00 00 00 00 00                          payload length = 20
038d  0a                                               name length = 10
038e  63 72 75 73 74 5f 74 79 70 65                    name "crust_type"
0398  00                                               unit length = 0
0399                                                   unit ""
0399  01                                               element type = 1 (uint8)
039a  01                                               components = 1
039b  00                                               crust_type of sample 0 = 0
039c  00                                               crust_type of sample 1 = 0
039d  01                                               crust_type of sample 2 = 1
039e  00                                               crust_type of sample 3 = 0
039f  00                                               crust_type of sample 4 = 0
03a0  01                                               crust_type of sample 5 = 1
03a1  d9 55 40 92                                      CRC-32 of the 32 bytes from offset 0381 = 924055d9

03a5  41 54 54 52                                      section kind "ATTR"
03a9  3f 00 00 00 00 00 00 00                          payload length = 63
03b1  09                                               name length = 9
03b2  74 68 69 63 6b 6e 65 73 73                       name "thickness"
03bb  02                                               unit length = 2
03bc  6b 6d                                            unit "km"
03be  04                                               element type = 4 (float64)
03bf  01                                               components = 1
03c0  00 00 00 00 00 00 1c 40                          thickness of sample 0 = 7
03c8  00 00 00 00 00 00 1c 40                          thickness of sample 1 = 7
03d0  00 00 00 00 00 80 41 40                          thickness of sample 2 = 35
03d8  00 00 00 00 00 00 1c 40                          thickness of sample 3 = 7
03e0  00 00 00 00 00 00 1c 40                          thickness of sample 4 = 7
03e8  00 00 00 00 00 80 41 40                          thickness of sample 5 = 35
03f0  2c c8 e1 5a                                      CRC-32 of the 75 bytes from offset 03a5 = 5ae1c82c

03f4  41 54 54 52                                      section kind "ATTR"
03f8  3f 00 00 00 00 00 00 00                          payload length = 63
0400  09                                               name length = 9
0401  65 6c 65 76 61 74 69 6f 6e                       name "elevation"
040a  02                                               unit length = 2
040b  6b 6d                                            unit "km"
040d  04                                               element type = 4 (float64)
040e  01                                               components = 1
040f  00 00 00 00 00 00 f0 bf                          elevation of sample 0 = -1
0417  00 00 00 00 00 00 f0 bf                          elevation of sample 1 = -1
041f  9a 99 99 99 99 99 b9 3f                          elevation of sample 2 = 0.1
0427  00 00 00 00 00 00 f0 bf                          elevation of sample 3 = -1
042f  00 00 00 00 00 00 f0 bf                          elevation of sample 4 = -1
0437  9a 99 99 99 99 99 b9 3f                          elevation of sample 5 = 0.1
043f  bb 5d c4 cb                                      CRC-32 of the 75 bytes from offset 03f4 = cbc45dbb

0443  41 54 54 52                                      section kind "ATTR"
0447  41 00 00 00 00 00 00 00                          payload length = 65
044f  0b                                               name length = 11
0450  6f 63 65 61 6e 69 63 5f 61 67 65                 name "oceanic_age"
045b  02                                               unit length = 2
045c  4d 79                                            unit "My"
045e  04                                               element type = 4 (float64)
045f  01                                               components = 1
0460  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 0 = 202.7325540540822
0468  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 1 = 202.7325540540822
0470  00 00 00 00 00 00 00 00                          oceanic_age of sample 2 = 0
0478  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 3 = 202.7325540540822
0480  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 4 = 202.7325540540822
0488  00 00 00 00 00 00 00 00                          oceanic_age of sample 5 = 0
0490  cb b1 9d b9                                      CRC-32 of the 77 bytes from offset 0443 = b99db1cb

0494  41 54 54 52                                      section kind "ATTR"
0498  a3 00 00 00 00 00 00 00                          payload length = 163
04a0  0f                                               name length = 15
04a1  72 69 64 67 65 5f 64 69 72 65 63 74 69 6f 6e     name "ridge_direction"
04b0  00                                               unit length = 0
04b1                                                   unit ""
04b1  04                                               element type = 4 (float64)
04b2  03                                               components = 3
04b3  00 00 00 00 00 00 00 00                          ridge_direction x of sample 0 = 0
04bb  00 00 00 00 00 00 00 00                          ridge_direction y of sample 0 = 0
04c3  00 00 00 00 00 00 00 00                          ridge_direction z of sample 0 = 0
04cb  00 00 00 00 00 00 00 00                          ridge_direction x of sample 1 = 0
04d3  00 00 00 00 00 00 00 00                          ridge_direction y of sample 1 = 0
04db  00 00 00 00 00 00 00 00                          ridge_direction z of sample 1 = 0
04e3  00 00 00 00 00 00 00 00                          ridge_direction x of sample 2 = 0
04eb  00 00 00 00 00 00 00 00                          ridge_direction y of sample 2 = 0
04f3  00 00 00 00 00 00 00 00                          ridge_direction z of sample 2 = 0
04fb  00 00 00 00 00 00 00 00                          ridge_direction x of sample 3 = 0
0503  00 00 00 00 00 00 00 00                          ridge_direction y of sample 3 = 0
050b  00 00 00 00 00 00 00 00                          ridge_direction z of sample 3 = 0
0513  00 00 00 00 00 00 00 00                          ridge_direction x of sample 4 = 0
051b  00 00 00 00 00 00 00 00                          ridge_direction y of sample 4 = 0
0523  00 00 00 00 00 00 00 00                          ridge_direction z of sample 4 = 0
052b  00 00 00 00 00 00 00 00                          ridge_direction x of sample 5 = 0
0533  00 00 00 00 00 00 00 00                          ridge_direction y of sample 5 = 0
053b  00 00 00 00 00 00 00 00                          ridge_direction z of sample 5 = 0
0543  92 07 59 35                                      CRC-32 of the 175 bytes from offset 0494 = 35590792

0547  41 54 54 52                                      section kind "ATTR"
054b  41 00 00 00 00 00 00 00                          payload length = 65
0553  0b                                               name length = 11
0554  6f 72 6f 67 65 6e 79 5f 61 67 65                 name "orogeny_age"
055f  02                                               unit length = 2
0560  4d 79                                            unit "My"
0562  04                                               element type = 4 (float64)
0563  01                                               components = 1
0564  00 00 00 00 00 00 00 00                          orogeny_age of sample 0 = 0
056c  00 00 00 00 00 00 00 00                          orogeny_age of sample 1 = 0
0574  00 00 00 00 00 00 00 00                          orogeny_age of sample 2 = 0
057c  00 00 00 00 00 00 00 00                          orogeny_age of sample 3 = 0
0584  00 00 00 00 00 00 00 00                          orogeny_age of sample 4 = 0
058c  00 00 00 00 00 00 00 00                          orogeny_age of sample 5 = 0
0594  ae bc 7a 7f                                      CRC-32 of the 77 bytes from offset 0547 = 7f7abcae

0598  41 54 54 52                                      section kind "ATTR"
059c  16 00 00 00 00 00 00 00                          payload length = 22
05a4  0c                                               name length = 12
05a5  6f 72 6f 67 65 6e 79 5f 74 79 70 65              name "orogeny_type"
05b1  00                                               unit length = 0
05b2                                                   unit ""
05b2  01                                               element type = 1 (uint8)
05b3  01                                               components = 1
05b4  00                                               orogeny_type of sample 0 = 0
05b5  00                                               orogeny_type of sample 1 = 0
05b6  00                                               orogeny_type of sample 2 = 0
05b7  00                                               orogeny_type of sample 3 = 0
05b8  00                                               orogeny_type of sample 4 = 0
05b9  00                                               orogeny_type of sample 5 = 0
05ba  68 3c d7 22                                      CRC-32 of the 34 bytes from offset 0598 = 22d73c68

05be  41 54 54 52                                      section kind "ATTR"
05c2  a2 00 00 00 00 00 00 00                          payload length = 162
05ca  0e                                               name length = 14
05cb  66 6f 6c 64 5f 64 69 72 65 63 74 69 6f 6e        name "fold_direction"
05d9  00                                               unit length = 0
05da                                                   unit ""
05da  04                                               element type = 4 (float64)
05db  03                                               components = 3
05dc  00 00 00 00 00 00 00 00                          fold_direction x of sample 0 = 0
05e4  00 00 00 00 00 00 00 00                          fold_direction y of sample 0 = 0
05ec  00 00 00 00 00 00 00 00                          fold_direction z of sample 0 = 0
05f4  00 00 00 00 00 00 00 00                          fold_direction x of sample 1 = 0
05fc  00 00 00 00 00 00 00 00                          fold_direction y of sample 1 = 0
0604  00 00 00 00 00 00 00 00                          fold_direction z of sample 1 = 0
060c  00 00 00 00 00 00 00 00                          fold_direction x of sample 2 = 0
0614  00 00 00 00 00 00 00 00                          fold_direction y of sample 2 = 0
061c  00 00 00 00 00 00 00 00                          fold_direction z of sample 2 = 0
0624  00 00 00 00 00 00 00 00                          fold_direction x of sample 3 = 0
062c  00 00 00 00 00 00 00 00                          fold_direction y of sample 3 = 0
0634  00 00 00 00 00 00 00 00                          fold_direction z of sample 3 = 0
063c  00 00 00 00 00 00 00 00                          fold_direction x of sample 4 = 0
0644  00 00 00 00 00 00 00 00                          fold_direction y of sample 4 = 0
064c  00 00 00 00 00 00 00 00                          fold_direction z of sample 4 = 0
0654  00 00 00 00 00 00 00 00                          fold_direction x of sample 5 = 0
065c  00 00 00 00 00 00 00 00                          fold_direction y of sample 5 = 0
0664  00 00 00 00 00 00 00 00                          fold_direction z of sample 5 = 0
066c  78 68 10 90                                      CRC-32 of the 174 bytes from offset 05be = 90106878

0670  41 54 54 52                                      section kind "ATTR"
0674  3f 00 00 00 00 00 00 00                          payload length = 63
067c  06                                               name length = 6
067d  75 70 6c 69 66 74                                name "uplift"
0683  05                                               unit length = 5
0684  6b 6d 2f 4d 79                                   unit "km/My"
0689  04                                               element type = 4 (float64)
068a  01                                               components = 1
068b  00 00 00 00 00 00 00 00                          uplift of sample 0 = 0
0693  00 00 00 00 00 00 00 00                          uplift of sample 1 = 0
069b  00 00 00 00 00 00 00 00                          uplift of sample 2 = 0
06a3  00 00 00 00 00 00 00 00                          uplift of sample 3 = 0
06ab  00 00 00 00 00 00 00 00                          uplift of sample 4 = 0
06b3  00 00 00 00 00 00 00 00                          uplift of sample 5 = 0
06bb  b0 da e2 db                                      CRC-32 of the 75 bytes from offset 0670 = dbe2dab0

06bf  45 4e 44 20                                      section kind "END "
06c3  00 00 00 00 00 00 00 00                          payload length = 0
06cb  1f 11 8b 42                                      CRC-32 of the 12 bytes from offset 06bf = 428b111f
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
0.15.0-alpha 6 8
[0.5527707983925666 0 0.8333333333333334] [0 5 2]
[1 1 0 1 0 0]
[0 0 1 0 0 1]
[-1 -1 0.1 -1 -1 0.1]
202.7325540540822
[0 0 0]
```
