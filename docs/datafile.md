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

### Reading a file

This is what the library's reader does. A reader that follows it accepts every file the library writes and every file a later version writes with format version 1.

1. Read the 12-byte header. If the identifier differs, the input is not a datafile. If the format version is not 1, the reader cannot read the file. Stop in either case.
2. Read sections one after another until the `END ` section. For each: read the kind and the length L, then L bytes of payload, then the checksum. L has no stated limit and may be wrong in a damaged file, so do not allocate L bytes before the data arrives.
3. Compute the checksum of every section, including one that will be skipped. If it differs from the one recorded, the file is corrupt: stop.
4. Act on the kind:
   - `META`, `SMPL`, `TRIS`: decode the payload. A second section of the same kind makes the file corrupt.
   - `TRIS` and `ATTR`: corrupt if no `SMPL` section has been read, because N is not known.
   - `ATTR`: corrupt if A is 0, C is 0, the name or the unit is not valid UTF-8, or an earlier `ATTR` section had the same name. These are checked before the element type, so they apply to an attribute that is then skipped. If the element type code is known, the payload length must be exactly as `ATTR` states.
   - `END `: corrupt if L is not 0, or if `META`, `SMPL`, or `TRIS` has not been read. Otherwise the file is complete: stop, and read nothing after the section's checksum. Bytes that follow are not part of the file.
   - Any other kind: skip the section.
5. If the input ends before the checksum of the `END ` section has been read, the file is truncated.

The reader enforces no order of sections beyond step 4. It accepts `META` after `SMPL`, and an `ATTR` section before `TRIS`.

The reader checks the lengths that each payload's layout states, and that every triangle corner is less than N. It does not check the other properties this document states of the contents: that positions have length 1 and decreasing z, that the triangles are the Delaunay triangulation, that values are within their ranges, or which attributes are present. Those are properties of what the library writes. A reader may rely on them for a file the library wrote, and need not verify them.

In `META`, the reader accepts any JSON object that has the members `library_version` and `config`, with any whitespace and member order. It ignores members it does not know, and of two members with the same name it keeps the last. It does not check the members of `config` against their ranges.

## Section payloads

### `META`

The payload is one JSON object, encoded as UTF-8, with no byte order mark and no trailing newline.

| Member | JSON type | Meaning |
|--------|-----------|---------|
| `library_version` | string | The version of the `yaee` library that generated the world, as a semantic version such as `0.21.1-alpha` |
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
| `abyssal_elevation_km` | number | km | less than `ridge_elevation_km`, greater than `trench_elevation_km` | Elevation of the abyssal plains: the elevation that oceanic crust sinks toward as it ages. Only a trench or the elevation noise puts oceanic crust below it |
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
| `rifting_rate_per_my` | number | 1/My | at least 0 | Mean number of times per My that a wholly continental plate of area a₀ = 4π × `radius_km`² / `plates` breaks into two to four plates. See "After the start" under "Attributes written". 0 gives plates that never break |
| `elevation_noise_amplitude_km` | number | km | at least 0 | Greatest change that the elevation noise makes to an `elevation`. See "After the start" under "Attributes written". 0 gives no noise |
| `elevation_noise_feature_size_km` | number | km | greater than 0 | Distance across the largest features of the elevation noise |

`seed` can exceed 2^53.
A JSON parser that reads every number as an IEEE 754 double corrupts such a seed; use a parser that keeps 64-bit integers if the seed matters to you.

A later version of the library may add members to either object without changing the format version.
A reader must ignore members it does not know.

The number of samples is in the `SMPL` section.
Do not compute it from `config`.

The default configuration, which this document refers to where it gives figures for the default world, is this `config` object:

```json
{"seed":0,"radius_km":6370,"sample_spacing_km":25,"duration_my":250,"time_step_my":2,"plates":40,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-2.5,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.17,"subduction_distance_km":600,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.1,"sediment_accretion_km_per_my":0.3,"collision_penetration_km":300,"collision_distance_km":1000,"collision_coefficient_per_km":5e-7,"rifting_rate_per_my":0.004,"elevation_noise_amplitude_km":0.5,"elevation_noise_feature_size_km":2000}
```

The library writes the payload without whitespace, with the members in the order of the two tables above, and each number with the fewest digits that give the same 64-bit value back.

### `SMPL`

| Offset | Width | Type | Field | Meaning |
|--------|-------|------|-------|---------|
| 0 | 4 | `u32` | N | The number of samples. At most 357,913,941, the most the library generates |
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

Library version 0.21.1-alpha writes these attributes, in this order:

| Name | Element type | Components | Unit | Range | Meaning |
|------|--------------|------------|------|-------|---------|
| `plate` | 2 (int32) | 1 | none (empty) | 0 to P − 1, with P the number of plates in the world (below) | The number of the tectonic plate the sample belongs to |
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

- P, the number of plates in the world, is not stored: it is 1 + the greatest value of `plate`. Every plate number from 0 to P − 1 is used by at least one sample.
- In the initial world (`duration_my` 0) P equals `plates`. After the start P can be less or greater: a plate that loses all its samples is removed, and a plate that breaks becomes two to four plates.
- A plate number identifies a plate within one file only. When a plate is removed, every plate with a higher number takes the next lower number, so plate k of a world is not plate k of the initial world, nor of the same configuration with another `duration_my`.
- In a world whose plates have not moved (`duration_my` 0, or `max_plate_speed_km_per_my` 0) the samples of one plate form a single connected region: any two of them are joined by a path of neighbouring samples of the same plate. Neighbours are defined under `TRIS`. Once plates have moved, a plate may be in several pieces.
- A plate boundary runs between two neighbouring samples whose plate numbers differ. No sample lies on a boundary.

`crust_type`:

- A coast runs between two neighbouring samples whose crust types differ. No sample lies on a coast.
- Continental crust may lie below sea level, where the elevation noise has lowered it, and oceanic crust that subduction has raised may lie above it: `crust_type` says what the crust is made of, and `elevation` says whether it is under water. Do not derive one from the other.
- Each sample stands for a share of the sphere's area. The share of continental samples differs from `land_coverage` by at most 1.5/N in the initial world. It changes as plates move.
- Once plates have moved, continents contain channels of oceanic crust 1 to 6 samples wide and up to hundreds of km long. In a default world (N = 942,059), oceanic samples in channels no wider than 4 samples are 1.3% to 1.8% of all samples, and 2.8% to 3.6% with `land_coverage` 0.5. Most lie inside a single plate and stay for the rest of the simulation. The generator does not remove them. A client that does not want seas this narrow must fill them itself.

`ridge_direction` and `fold_direction`:

- The components are on the same axes as the positions in `SMPL`.
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
| `oceanic_age` | 3 × \|`abyssal_elevation_km`\| / `oceanic_dampening_km_per_my`; 105.88235294117646 with the default configuration; 0 if `oceanic_dampening_km_per_my` is 0 | 0 |
| `ridge_direction` | (0, 0, 0) | (0, 0, 0) |
| `orogeny_age` | 0 | 0 |
| `orogeny_type` | 0 | 0 |
| `fold_direction` | (0, 0, 0) | (0, 0, 0) |
| `uplift` | 0 | 0 |

With the default configuration the initial `elevation` is therefore −6 to −2.5 for oceanic samples and 0.1 to 0.5 for continental samples.

##### After the start

The simulation runs for `duration_my` in steps of `time_step_my`; if `duration_my` is not a multiple of `time_step_my`, the last step is the shorter remainder.
In this library version the plates move as rigid bodies, carrying their crust; oceanic crust ages and sinks; new oceanic crust forms where plates part; where plates meet, one subducts beneath the other, which is raised; trenches form along subduction fronts; continents that meet collide and join; land erodes; and plates break into smaller plates that move apart.

Where crust of two plates overlaps, one plate's crust subducts and is discarded; the other plate is the overriding plate there:

- Oceanic crust subducts beneath continental crust.
- Of two oceanic crusts, that of the plate whose oceanic crust is older on average subducts; at equal ages, that of the higher-numbered plate.
- Of two continental crusts, that of the higher-numbered plate subducts, until the two collide (below).

A subduction front is the edge of an overriding plate along which another plate moves beneath it.

Every step of length δt My, for every sample of oceanic crust:

- `oceanic_age` increases by δt.
- With z the `elevation` without the sample's trench (below) and z_a = `abyssal_elevation_km`: if z is above z_a, `elevation` falls by (z − z_a) × min(1, `oceanic_dampening_km_per_my` × δt / |z_a|).
  Otherwise it does not change.
  z approaches z_a and never passes it.
  Crust that formed at `ridge_elevation_km` is, t My later, within 0.04 km of z_a + (`ridge_elevation_km` − z_a) × exp(−`oceanic_dampening_km_per_my` × t / |z_a|) with the default configuration: −2.5 at 0 My, −4.5 at 30 My, −5.8 at 100 My, −6.0 at 250 My.

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

Every step of δt My, each plate may break (rift), independently of the others:

- a is the plate's area in km² and x the share of it that is continental crust, both as of the last step in which the world was rebuilt on its samples, and a₀ = 4π × `radius_km`² / `plates`.
- The plate breaks in the step with probability 1 − exp(−λ × δt), with λ = `rifting_rate_per_my` × (0.25 + 0.75 × x) × a / a₀. The number of breaks per My therefore does not depend on `time_step_my`. A plate with a less than a₀ does not break, and no plate breaks if `rifting_rate_per_my` or `max_plate_speed_km_per_my` is 0.
- A plate that breaks is divided into n plates, n being 2, 3, or 4 with equal probability, along irregular lines. Its samples are divided among the n plates; `plate` changes and no other attribute does. One of the n keeps the plate's number, and the others take the numbers P, P + 1, … in the world as it then is.
- Each of the n plates gets a new rotation: its centre moves straight away from the centre of the plate that broke, all n at one angular speed v / `radius_km` radians per My, with v drawn uniformly from 0 to `max_plate_speed_km_per_my`. New oceanic crust then forms between them as between any plates that part.

After the last step, if `duration_my` is greater than 0, the elevation noise is added to every sample, of either crust type: `elevation` z becomes z + `elevation_noise_amplitude_km` × F, limited to the range `trench_elevation_km` to `max_continental_elevation_km`.
F depends on the sample's position and on `seed`, `radius_km`, and `elevation_noise_feature_size_km` only. It is from −1 to 1, with a mean near 0 and a standard deviation of about 0.26, and it changes smoothly over the sphere: its largest features are `elevation_noise_feature_size_km` across and its smallest a quarter of that.
The noise is not added step by step, so its size does not depend on `duration_my` or `time_step_my`.
No other attribute is changed by it. A reader cannot remove it: F is not stored.

A plate with no sample left, because all its crust has subducted or has joined another plate in a collision, is removed, and the plates are numbered again as described under `plate`.

With `max_plate_speed_km_per_my` 0 no plate moves, nothing subducts, and no plate breaks: the world is the initial world with its oceanic crust aged and lowered, and its land eroded, by `duration_my`, and with the elevation noise added.

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
- Oceanic crust is below `abyssal_elevation_km` only in a trench, or by at most `elevation_noise_amplitude_km` where the elevation noise has lowered it.
- Continental crust is below 0 only where the elevation noise has lowered it, and then by at most `elevation_noise_amplitude_km`.

## Size

For N samples the file is 12 + (16 + J) + (20 + 24 × N) + (20 + 12 × (2 × N − 4)) + 16 bytes, where J is the length of the `META` JSON, plus 16 + 4 + A + U + S × C × N for each attribute.
The default world has N = 942,059 and, with the attributes that library version 0.21.1-alpha writes, is 133,773,466 bytes.

## Example

The file `testdata/example.yaee` in the repository is 1941 bytes.
It is the world generated by library version 0.21.1-alpha from this configuration, which gives N = 6 samples and T = 8 triangles, and, because `duration_my` is 0, the initial world:

```json
{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-2.5,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.17,"subduction_distance_km":600,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.1,"sediment_accretion_km_per_my":0.3,"collision_penetration_km":300,"collision_distance_km":1000,"collision_coefficient_per_km":5e-7,"rifting_rate_per_my":0.004,"elevation_noise_amplitude_km":0.5,"elevation_noise_feature_size_km":2000}
```

Its `META` payload is these 738 bytes of text:

```json
{"library_version":"0.21.1-alpha","config":{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-2.5,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100,"oceanic_dampening_km_per_my":0.17,"subduction_distance_km":600,"subduction_uplift_km_per_my":0.6,"continental_erosion_km_per_my":0.1,"sediment_accretion_km_per_my":0.3,"collision_penetration_km":300,"collision_distance_km":1000,"collision_coefficient_per_km":5e-7,"rifting_rate_per_my":0.004,"elevation_noise_amplitude_km":0.5,"elevation_noise_feature_size_km":2000}}
```

Every byte of the file is listed below.
The first column is the offset in hexadecimal, the second the bytes in hexadecimal, and the third the field they encode.
A field longer than 16 bytes continues on the following lines.

<!-- example:begin -->
```text
0000  59 41 45 45 44 41 54 41                          identifier "YAEEDATA"
0008  01 00 00 00                                      format version = 1

000c  4d 45 54 41                                      section kind "META"
0010  e2 02 00 00 00 00 00 00                          payload length = 738
0018  7b 22 6c 69 62 72 61 72 79 5f 76 65 72 73 69 6f  payload: the JSON text shown above
0028  6e 22 3a 22 30 2e 32 31 2e 31 2d 61 6c 70 68 61
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
0108  6f 6e 5f 6b 6d 22 3a 2d 32 2e 35 2c 22 61 62 79
0118  73 73 61 6c 5f 65 6c 65 76 61 74 69 6f 6e 5f 6b
0128  6d 22 3a 2d 36 2c 22 74 72 65 6e 63 68 5f 65 6c
0138  65 76 61 74 69 6f 6e 5f 6b 6d 22 3a 2d 31 30 2c
0148  22 6d 61 78 5f 63 6f 6e 74 69 6e 65 6e 74 61 6c
0158  5f 65 6c 65 76 61 74 69 6f 6e 5f 6b 6d 22 3a 31
0168  30 2c 22 6d 61 78 5f 70 6c 61 74 65 5f 73 70 65
0178  65 64 5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a 31 30
0188  30 2c 22 6f 63 65 61 6e 69 63 5f 64 61 6d 70 65
0198  6e 69 6e 67 5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a
01a8  30 2e 31 37 2c 22 73 75 62 64 75 63 74 69 6f 6e
01b8  5f 64 69 73 74 61 6e 63 65 5f 6b 6d 22 3a 36 30
01c8  30 2c 22 73 75 62 64 75 63 74 69 6f 6e 5f 75 70
01d8  6c 69 66 74 5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a
01e8  30 2e 36 2c 22 63 6f 6e 74 69 6e 65 6e 74 61 6c
01f8  5f 65 72 6f 73 69 6f 6e 5f 6b 6d 5f 70 65 72 5f
0208  6d 79 22 3a 30 2e 31 2c 22 73 65 64 69 6d 65 6e
0218  74 5f 61 63 63 72 65 74 69 6f 6e 5f 6b 6d 5f 70
0228  65 72 5f 6d 79 22 3a 30 2e 33 2c 22 63 6f 6c 6c
0238  69 73 69 6f 6e 5f 70 65 6e 65 74 72 61 74 69 6f
0248  6e 5f 6b 6d 22 3a 33 30 30 2c 22 63 6f 6c 6c 69
0258  73 69 6f 6e 5f 64 69 73 74 61 6e 63 65 5f 6b 6d
0268  22 3a 31 30 30 30 2c 22 63 6f 6c 6c 69 73 69 6f
0278  6e 5f 63 6f 65 66 66 69 63 69 65 6e 74 5f 70 65
0288  72 5f 6b 6d 22 3a 35 65 2d 37 2c 22 72 69 66 74
0298  69 6e 67 5f 72 61 74 65 5f 70 65 72 5f 6d 79 22
02a8  3a 30 2e 30 30 34 2c 22 65 6c 65 76 61 74 69 6f
02b8  6e 5f 6e 6f 69 73 65 5f 61 6d 70 6c 69 74 75 64
02c8  65 5f 6b 6d 22 3a 30 2e 35 2c 22 65 6c 65 76 61
02d8  74 69 6f 6e 5f 6e 6f 69 73 65 5f 66 65 61 74 75
02e8  72 65 5f 73 69 7a 65 5f 6b 6d 22 3a 32 30 30 30
02f8  7d 7d
02fa  fe 26 25 0d                                      CRC-32 of the 750 bytes from offset 000c = 0d2526fe

02fe  53 4d 50 4c                                      section kind "SMPL"
0302  94 00 00 00 00 00 00 00                          payload length = 148
030a  06 00 00 00                                      N = 6 samples
030e  cd f4 a8 62 4c b0 e1 3f                          sample 0 x = 0.5527707983925666
0316  00 00 00 00 00 00 00 00                          sample 0 y = 0
031e  ab aa aa aa aa aa ea 3f                          sample 0 z = 0.8333333333333334
0326  8d d1 d2 b3 3f 6f e4 bf                          sample 1 x = -0.6385801803758554
032e  17 00 ef a0 40 b8 e2 bf                          sample 1 y = -0.5849917548403053
0336  00 00 00 00 00 00 e0 3f                          sample 1 z = 0.5
033e  2a 45 a8 20 65 11 b6 3f                          sample 2 x = 0.08620292708680624
0346  29 36 2e 29 7e 6e ef 3f                          sample 2 y = 0.9822378925616181
034e  54 55 55 55 55 55 c5 3f                          sample 2 z = 0.16666666666666663
0356  be 62 fd e5 9d 32 e3 3f                          sample 3 x = 0.5999288074482008
035e  da ae 4b 50 3f 0a e9 bf                          sample 3 y = -0.7825008934282451
0366  58 55 55 55 55 55 c5 bf                          sample 3 z = -0.16666666666666674
036e  a3 33 61 bd 07 4a eb bf                          sample 4 x = -0.8527868937322761
0376  7c 4c 49 e9 eb 4e c3 3f                          sample 4 y = 0.15084599390920406
037e  00 00 00 00 00 00 e0 bf                          sample 4 z = -0.5
0386  66 a1 22 2d 8d d9 dd 3f                          sample 5 x = 0.46640328796140407
038e  21 af 85 f6 ed fc d2 3f                          sample 5 y = 0.29668759416994034
0396  aa aa aa aa aa aa ea bf                          sample 5 z = -0.8333333333333333
039e  c0 a8 d3 44                                      CRC-32 of the 160 bytes from offset 02fe = 44d3a8c0

03a2  54 52 49 53                                      section kind "TRIS"
03a6  64 00 00 00 00 00 00 00                          payload length = 100
03ae  08 00 00 00                                      T = 8 triangles
03b2  00 00 00 00 05 00 00 00 02 00 00 00              triangle 0 = samples 0, 5, 2
03be  00 00 00 00 03 00 00 00 05 00 00 00              triangle 1 = samples 0, 3, 5
03ca  05 00 00 00 03 00 00 00 04 00 00 00              triangle 2 = samples 5, 3, 4
03d6  02 00 00 00 04 00 00 00 01 00 00 00              triangle 3 = samples 2, 4, 1
03e2  04 00 00 00 03 00 00 00 01 00 00 00              triangle 4 = samples 4, 3, 1
03ee  03 00 00 00 00 00 00 00 01 00 00 00              triangle 5 = samples 3, 0, 1
03fa  05 00 00 00 04 00 00 00 02 00 00 00              triangle 6 = samples 5, 4, 2
0406  01 00 00 00 00 00 00 00 02 00 00 00              triangle 7 = samples 1, 0, 2
0412  3f 76 78 72                                      CRC-32 of the 112 bytes from offset 03a2 = 7278763f

0416  41 54 54 52                                      section kind "ATTR"
041a  21 00 00 00 00 00 00 00                          payload length = 33
0422  05                                               name length = 5
0423  70 6c 61 74 65                                   name "plate"
0428  00                                               unit length = 0
0429                                                   unit ""
0429  02                                               element type = 2 (int32)
042a  01                                               components = 1
042b  01 00 00 00                                      plate of sample 0 = 1
042f  01 00 00 00                                      plate of sample 1 = 1
0433  00 00 00 00                                      plate of sample 2 = 0
0437  01 00 00 00                                      plate of sample 3 = 1
043b  00 00 00 00                                      plate of sample 4 = 0
043f  00 00 00 00                                      plate of sample 5 = 0
0443  cb 98 75 80                                      CRC-32 of the 45 bytes from offset 0416 = 807598cb

0447  41 54 54 52                                      section kind "ATTR"
044b  14 00 00 00 00 00 00 00                          payload length = 20
0453  0a                                               name length = 10
0454  63 72 75 73 74 5f 74 79 70 65                    name "crust_type"
045e  00                                               unit length = 0
045f                                                   unit ""
045f  01                                               element type = 1 (uint8)
0460  01                                               components = 1
0461  00                                               crust_type of sample 0 = 0
0462  00                                               crust_type of sample 1 = 0
0463  01                                               crust_type of sample 2 = 1
0464  00                                               crust_type of sample 3 = 0
0465  00                                               crust_type of sample 4 = 0
0466  01                                               crust_type of sample 5 = 1
0467  d9 55 40 92                                      CRC-32 of the 32 bytes from offset 0447 = 924055d9

046b  41 54 54 52                                      section kind "ATTR"
046f  3f 00 00 00 00 00 00 00                          payload length = 63
0477  09                                               name length = 9
0478  74 68 69 63 6b 6e 65 73 73                       name "thickness"
0481  02                                               unit length = 2
0482  6b 6d                                            unit "km"
0484  04                                               element type = 4 (float64)
0485  01                                               components = 1
0486  00 00 00 00 00 00 1c 40                          thickness of sample 0 = 7
048e  00 00 00 00 00 00 1c 40                          thickness of sample 1 = 7
0496  00 00 00 00 00 80 41 40                          thickness of sample 2 = 35
049e  00 00 00 00 00 00 1c 40                          thickness of sample 3 = 7
04a6  00 00 00 00 00 00 1c 40                          thickness of sample 4 = 7
04ae  00 00 00 00 00 80 41 40                          thickness of sample 5 = 35
04b6  2c c8 e1 5a                                      CRC-32 of the 75 bytes from offset 046b = 5ae1c82c

04ba  41 54 54 52                                      section kind "ATTR"
04be  3f 00 00 00 00 00 00 00                          payload length = 63
04c6  09                                               name length = 9
04c7  65 6c 65 76 61 74 69 6f 6e                       name "elevation"
04d0  02                                               unit length = 2
04d1  6b 6d                                            unit "km"
04d3  04                                               element type = 4 (float64)
04d4  01                                               components = 1
04d5  00 00 00 00 00 00 04 c0                          elevation of sample 0 = -2.5
04dd  00 00 00 00 00 00 04 c0                          elevation of sample 1 = -2.5
04e5  9a 99 99 99 99 99 b9 3f                          elevation of sample 2 = 0.1
04ed  00 00 00 00 00 00 04 c0                          elevation of sample 3 = -2.5
04f5  00 00 00 00 00 00 04 c0                          elevation of sample 4 = -2.5
04fd  9a 99 99 99 99 99 b9 3f                          elevation of sample 5 = 0.1
0505  a0 12 9a ad                                      CRC-32 of the 75 bytes from offset 04ba = ad9a12a0

0509  41 54 54 52                                      section kind "ATTR"
050d  41 00 00 00 00 00 00 00                          payload length = 65
0515  0b                                               name length = 11
0516  6f 63 65 61 6e 69 63 5f 61 67 65                 name "oceanic_age"
0521  02                                               unit length = 2
0522  4d 79                                            unit "My"
0524  04                                               element type = 4 (float64)
0525  01                                               components = 1
0526  78 78 78 78 78 78 5a 40                          oceanic_age of sample 0 = 105.88235294117646
052e  78 78 78 78 78 78 5a 40                          oceanic_age of sample 1 = 105.88235294117646
0536  00 00 00 00 00 00 00 00                          oceanic_age of sample 2 = 0
053e  78 78 78 78 78 78 5a 40                          oceanic_age of sample 3 = 105.88235294117646
0546  78 78 78 78 78 78 5a 40                          oceanic_age of sample 4 = 105.88235294117646
054e  00 00 00 00 00 00 00 00                          oceanic_age of sample 5 = 0
0556  34 ab 8f f6                                      CRC-32 of the 77 bytes from offset 0509 = f68fab34

055a  41 54 54 52                                      section kind "ATTR"
055e  a3 00 00 00 00 00 00 00                          payload length = 163
0566  0f                                               name length = 15
0567  72 69 64 67 65 5f 64 69 72 65 63 74 69 6f 6e     name "ridge_direction"
0576  00                                               unit length = 0
0577                                                   unit ""
0577  04                                               element type = 4 (float64)
0578  03                                               components = 3
0579  00 00 00 00 00 00 00 00                          ridge_direction x of sample 0 = 0
0581  00 00 00 00 00 00 00 00                          ridge_direction y of sample 0 = 0
0589  00 00 00 00 00 00 00 00                          ridge_direction z of sample 0 = 0
0591  00 00 00 00 00 00 00 00                          ridge_direction x of sample 1 = 0
0599  00 00 00 00 00 00 00 00                          ridge_direction y of sample 1 = 0
05a1  00 00 00 00 00 00 00 00                          ridge_direction z of sample 1 = 0
05a9  00 00 00 00 00 00 00 00                          ridge_direction x of sample 2 = 0
05b1  00 00 00 00 00 00 00 00                          ridge_direction y of sample 2 = 0
05b9  00 00 00 00 00 00 00 00                          ridge_direction z of sample 2 = 0
05c1  00 00 00 00 00 00 00 00                          ridge_direction x of sample 3 = 0
05c9  00 00 00 00 00 00 00 00                          ridge_direction y of sample 3 = 0
05d1  00 00 00 00 00 00 00 00                          ridge_direction z of sample 3 = 0
05d9  00 00 00 00 00 00 00 00                          ridge_direction x of sample 4 = 0
05e1  00 00 00 00 00 00 00 00                          ridge_direction y of sample 4 = 0
05e9  00 00 00 00 00 00 00 00                          ridge_direction z of sample 4 = 0
05f1  00 00 00 00 00 00 00 00                          ridge_direction x of sample 5 = 0
05f9  00 00 00 00 00 00 00 00                          ridge_direction y of sample 5 = 0
0601  00 00 00 00 00 00 00 00                          ridge_direction z of sample 5 = 0
0609  92 07 59 35                                      CRC-32 of the 175 bytes from offset 055a = 35590792

060d  41 54 54 52                                      section kind "ATTR"
0611  41 00 00 00 00 00 00 00                          payload length = 65
0619  0b                                               name length = 11
061a  6f 72 6f 67 65 6e 79 5f 61 67 65                 name "orogeny_age"
0625  02                                               unit length = 2
0626  4d 79                                            unit "My"
0628  04                                               element type = 4 (float64)
0629  01                                               components = 1
062a  00 00 00 00 00 00 00 00                          orogeny_age of sample 0 = 0
0632  00 00 00 00 00 00 00 00                          orogeny_age of sample 1 = 0
063a  00 00 00 00 00 00 00 00                          orogeny_age of sample 2 = 0
0642  00 00 00 00 00 00 00 00                          orogeny_age of sample 3 = 0
064a  00 00 00 00 00 00 00 00                          orogeny_age of sample 4 = 0
0652  00 00 00 00 00 00 00 00                          orogeny_age of sample 5 = 0
065a  ae bc 7a 7f                                      CRC-32 of the 77 bytes from offset 060d = 7f7abcae

065e  41 54 54 52                                      section kind "ATTR"
0662  16 00 00 00 00 00 00 00                          payload length = 22
066a  0c                                               name length = 12
066b  6f 72 6f 67 65 6e 79 5f 74 79 70 65              name "orogeny_type"
0677  00                                               unit length = 0
0678                                                   unit ""
0678  01                                               element type = 1 (uint8)
0679  01                                               components = 1
067a  00                                               orogeny_type of sample 0 = 0
067b  00                                               orogeny_type of sample 1 = 0
067c  00                                               orogeny_type of sample 2 = 0
067d  00                                               orogeny_type of sample 3 = 0
067e  00                                               orogeny_type of sample 4 = 0
067f  00                                               orogeny_type of sample 5 = 0
0680  68 3c d7 22                                      CRC-32 of the 34 bytes from offset 065e = 22d73c68

0684  41 54 54 52                                      section kind "ATTR"
0688  a2 00 00 00 00 00 00 00                          payload length = 162
0690  0e                                               name length = 14
0691  66 6f 6c 64 5f 64 69 72 65 63 74 69 6f 6e        name "fold_direction"
069f  00                                               unit length = 0
06a0                                                   unit ""
06a0  04                                               element type = 4 (float64)
06a1  03                                               components = 3
06a2  00 00 00 00 00 00 00 00                          fold_direction x of sample 0 = 0
06aa  00 00 00 00 00 00 00 00                          fold_direction y of sample 0 = 0
06b2  00 00 00 00 00 00 00 00                          fold_direction z of sample 0 = 0
06ba  00 00 00 00 00 00 00 00                          fold_direction x of sample 1 = 0
06c2  00 00 00 00 00 00 00 00                          fold_direction y of sample 1 = 0
06ca  00 00 00 00 00 00 00 00                          fold_direction z of sample 1 = 0
06d2  00 00 00 00 00 00 00 00                          fold_direction x of sample 2 = 0
06da  00 00 00 00 00 00 00 00                          fold_direction y of sample 2 = 0
06e2  00 00 00 00 00 00 00 00                          fold_direction z of sample 2 = 0
06ea  00 00 00 00 00 00 00 00                          fold_direction x of sample 3 = 0
06f2  00 00 00 00 00 00 00 00                          fold_direction y of sample 3 = 0
06fa  00 00 00 00 00 00 00 00                          fold_direction z of sample 3 = 0
0702  00 00 00 00 00 00 00 00                          fold_direction x of sample 4 = 0
070a  00 00 00 00 00 00 00 00                          fold_direction y of sample 4 = 0
0712  00 00 00 00 00 00 00 00                          fold_direction z of sample 4 = 0
071a  00 00 00 00 00 00 00 00                          fold_direction x of sample 5 = 0
0722  00 00 00 00 00 00 00 00                          fold_direction y of sample 5 = 0
072a  00 00 00 00 00 00 00 00                          fold_direction z of sample 5 = 0
0732  78 68 10 90                                      CRC-32 of the 174 bytes from offset 0684 = 90106878

0736  41 54 54 52                                      section kind "ATTR"
073a  3f 00 00 00 00 00 00 00                          payload length = 63
0742  06                                               name length = 6
0743  75 70 6c 69 66 74                                name "uplift"
0749  05                                               unit length = 5
074a  6b 6d 2f 4d 79                                   unit "km/My"
074f  04                                               element type = 4 (float64)
0750  01                                               components = 1
0751  00 00 00 00 00 00 00 00                          uplift of sample 0 = 0
0759  00 00 00 00 00 00 00 00                          uplift of sample 1 = 0
0761  00 00 00 00 00 00 00 00                          uplift of sample 2 = 0
0769  00 00 00 00 00 00 00 00                          uplift of sample 3 = 0
0771  00 00 00 00 00 00 00 00                          uplift of sample 4 = 0
0779  00 00 00 00 00 00 00 00                          uplift of sample 5 = 0
0781  b0 da e2 db                                      CRC-32 of the 75 bytes from offset 0736 = dbe2dab0

0785  45 4e 44 20                                      section kind "END "
0789  00 00 00 00 00 00 00 00                          payload length = 0
0791  1f 11 8b 42                                      CRC-32 of the 12 bytes from offset 0785 = 428b111f
```
<!-- example:end -->

From the listing: sample 0 is at latitude asin(0.8333333333333334) = 0.9851 radians (56.44° north) and longitude atan2(0, 0.5527707983925666) = 0.
Its elevation is −2.5 km, so its point on the surface is 6367.5 × (0.5527707983925666, 0, 0.8333333333333334) km from the centre.
It belongs to plate 1, as do samples 1 and 3; samples 2, 4, and 5 belong to plate 0.
Samples 2 and 5 are continental crust, 35 km thick, at 0.1 km; the other four are oceanic crust, 7 km thick, 105.88235294117646 My old, at −2.5 km.
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
0.21.1-alpha 6 8
[0.5527707983925666 0 0.8333333333333334] [0 5 2]
[1 1 0 1 0 0]
[0 0 1 0 0 1]
[-2.5 -2.5 0.1 -2.5 -2.5 0.1]
105.88235294117646
[0 0 0]
```
