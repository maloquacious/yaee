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
| `library_version` | string | The version of the `yaee` library that generated the world, as a semantic version such as `0.13.0-alpha` |
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

Library version 0.13.0-alpha writes these attributes, in this order:

| Name | Element type | Components | Unit | Range | Meaning |
|------|--------------|------------|------|-------|---------|
| `plate` | 2 (int32) | 1 | none (empty) | 0 to `plates` − 1 | The number of the tectonic plate the sample belongs to |
| `crust_type` | 1 (uint8) | 1 | none (empty) | 0 or 1 | Type of the crust: 0 is oceanic, 1 is continental |
| `thickness` | 4 (float64) | 1 | `km` | greater than 0 | Thickness of the crust |
| `elevation` | 4 (float64) | 1 | `km` | `trench_elevation_km` to `max_continental_elevation_km` | Height of the surface above sea level. Negative values are below sea level; 0 is sea level |
| `oceanic_age` | 4 (float64) | 1 | `My` | at least 0 | Time since oceanic crust formed at a ridge. 0 for continental crust |
| `ridge_direction` | 4 (float64) | 3 | none (empty) | each component −1 to 1 | Direction of the ridge at which oceanic crust formed, as a vector (x, y, z) |
| `orogeny_age` | 4 (float64) | 1 | `My` | at least 0 | Time since the mountain building that last affected the sample began. 0 where `orogeny_type` is 0 |
| `orogeny_type` | 1 (uint8) | 1 | none (empty) | 0, 1, or 2 | Kind of mountain building that last affected the sample: 0 is none, 1 is Andean (an oceanic plate subducts beneath the sample's plate), 2 is Himalayan (two continents collide) |
| `fold_direction` | 4 (float64) | 3 | none (empty) | each component finite | Direction in which the crust is folded, as a vector (x, y, z) |

Names in code font in the Range column, and below, are members of `config` in the `META` section.
No value of any attribute is NaN or infinite.

`plate`:

- In the initial world (`duration_my` 0, or `max_plate_speed_km_per_my` 0) every plate number from 0 to `plates` − 1 is used by at least one sample, and the samples of one plate form a single connected region: any two of them are joined by a path of neighbouring samples of the same plate. Neighbours are defined under `TRIS`.
- Once plates have moved, neither holds. A plate may be in several pieces, and a plate number may be used by no sample. Do not assume either.
- A plate boundary runs between two neighbouring samples whose plate numbers differ. No sample lies on a boundary.

`crust_type`:

- A coast runs between two neighbouring samples whose crust types differ. No sample lies on a coast.
- Continental crust may lie below sea level and oceanic crust is always below it: `crust_type` says what the crust is made of, and `elevation` says whether it is under water. Do not derive one from the other.
- Each sample stands for a share of the sphere's area. The share of continental samples differs from `land_coverage` by at most 1.5/N in the initial world. It changes as plates move.

`ridge_direction` and `fold_direction`:

- The components are on the same axes as the positions in `SAMP`.
- A vector is either (0, 0, 0), meaning no direction is known, or perpendicular to the sample's position.
- A `ridge_direction` that is not (0, 0, 0) has length 1. It is (0, 0, 0) for continental crust.
- A `fold_direction` that is not (0, 0, 0) may have any length.

The initial world (`duration_my` 0, or `max_plate_speed_km_per_my` 0) holds the initial crust.
With d the distance in km, along the surface, from a sample to the nearest sample that has a neighbour of the other crust type (0 for such a sample; infinite if the world has one crust type), and s(x) = x² × (3 − 2x) for x from 0 to 1 and s(x) = 1 for x greater than 1:

| Attribute | Oceanic sample | Continental sample |
|-----------|----------------|--------------------|
| `thickness` | 7 | 35 |
| `elevation` | `ridge_elevation_km` + (`abyssal_elevation_km` − `ridge_elevation_km`) × s(d / 500) | `max_continental_elevation_km` × (0.01 + 0.04 × s(d / 500)) |
| `oceanic_age` | (`trench_elevation_km` / 0.04) × ln((`abyssal_elevation_km` − `trench_elevation_km`) / (`ridge_elevation_km` − `trench_elevation_km`)); 202.7325540540822 with the default configuration | 0 |
| `ridge_direction` | (0, 0, 0) | (0, 0, 0) |
| `orogeny_age` | 0 | 0 |
| `orogeny_type` | 0 | 0 |
| `fold_direction` | (0, 0, 0) | (0, 0, 0) |

With the default configuration the initial `elevation` is therefore −6 to −1 for oceanic samples and 0.1 to 0.5 for continental samples.

In this library version the plates then move as rigid bodies for `duration_my`, carrying their crust, and nothing else happens: no ridge, trench, or mountain is formed.
In the world that results:

- A sample's values are those of the crust that has moved to its position, interpolated between the up to three samples that crust came from. A scalar is therefore a value of the initial crust or lies between two of them, and `thickness` and `elevation` lie between the oceanic and the continental values within about a sample spacing of a coast.
- Where plates have moved apart, the gap holds new oceanic crust: `crust_type` 0, `thickness` 7, `elevation` equal to `ridge_elevation_km`, `oceanic_age` 0, and every other attribute as in the table above.
- Where plates have moved over one another, the crust of one of them is kept and the other's is discarded.
- `oceanic_age` does not increase with time.

A later version replaces these values with simulated ones, under the same names, types, component counts, and units, and within the ranges of the first table.

### `END `

The payload is empty: the length is 0.
The file ends with the last byte of this section's checksum.
Because the kind and length are fixed, the section is always the same 16 bytes: `45 4e 44 20 00 00 00 00 00 00 00 00 1f 11 8b 42`.

## Reading a file

Follow these steps in order.
"Reject" means stop and report an error; do not use any data from a rejected file.

1. Read 12 bytes.
   If fewer are available, reject: the file is truncated.
2. If bytes 0 to 7 are not `59 41 45 45 44 41 54 41`, reject: the input is not a `yaee` datafile.
3. Read bytes 8 to 11 as a `u32`.
   If it is not 1, reject: the format version is not supported.
   Report the version found.
   Do not try to read a file of another version by these rules.
4. Read a section:
   1. Read 12 bytes: the kind and the length L.
      If fewer are available, reject: truncated.
   2. Read L bytes of payload.
      If fewer are available, reject: truncated.
      L comes from the file and may be wrong in a damaged file, so do not allocate L bytes before checking that the input has them.
   3. Read 4 bytes: the checksum.
      If fewer are available, reject: truncated.
   4. Compute the CRC-32 of the 12 + L bytes of kind, length, and payload.
      If it differs from the checksum, reject: the file is corrupt.
5. Act on the kind:
   - `META`: parse the payload as JSON.
     Reject if it is not a JSON object with a string `library_version` and an object `config`, or if this is the second `META`.
   - `SMPL`: read N.
     Reject if L is not 4 + 24 × N, or if this is the second `SMPL`.
     Read the positions.
   - `TRIS`: read T.
     Reject if no `SMPL` has been read, if L is not 4 + 12 × T, if any corner is N or more, or if this is the second `TRIS`.
     Read the corners.
   - `ATTR`: reject if no `SMPL` has been read.
     Read the name, unit, element type, and component count; reject if the payload ends before they do, if the name is empty, if the component count is 0, or if an earlier `ATTR` had the same name.
     If the element type code is unknown, skip the rest of the section.
     Otherwise reject if L is not 1 + A + 1 + U + 2 + S × C × N, then read the data, or skip it if the reader has no use for the attribute.
   - `END `: reject if L is not 0, or if `META`, `SMPL`, and `TRIS` have not each been read.
     Otherwise the file has been read successfully: stop.
   - Any other kind: ignore the payload.
     Its checksum has already been verified in step 4.
6. Go to step 4.

A file that ends before an `END ` section is truncated, however many complete sections came before; step 4.1 detects this.

Bytes after the `END ` section are not part of the datafile.
`yaee.Read` stops reading at the end of that section and does not look for more.

### Errors from the Go reader

`yaee.Read` returns an error that wraps one of these, testable with `errors.Is`:

| Error | Cause |
|-------|-------|
| `yaee.ErrNotDatafile` | Step 2 |
| `yaee.ErrUnsupportedVersion` | Step 3. The text includes `found version` followed by the number |
| `yaee.ErrTruncated` | The input ended in step 1 or step 4 |
| `yaee.ErrCorrupt` | A checksum mismatch in step 4, or any rejection in step 5 |

Any other error is the one returned by the `io.Reader` it was given.

## What changes the format version

The format version changes only when a reader that follows this document would misread the file.
These do not change it, and a version 1 reader must tolerate them:

- a new attribute;
- a new element type code;
- a new section kind;
- a new member in the `META` JSON.

## Size

For N samples the file is 12 + (16 + J) + (20 + 24 × N) + (20 + 12 × (2 × N − 4)) + 16 bytes, where J is the length of the `META` JSON, plus 16 + 4 + A + U + S × C × N for each attribute.
The default world has N = 942,059 and, with the attributes that library version 0.13.0-alpha writes, is 126,236,594 bytes.

## Example

The file `testdata/example.yaee` in the repository is 1493 bytes.
It is the world generated by library version 0.13.0-alpha from this configuration, which gives N = 6 samples and T = 8 triangles, and, because `duration_my` is 0, the initial world:

```json
{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100}
```

Its `META` payload is these 369 bytes of text:

```json
{"library_version":"0.13.0-alpha","config":{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":0,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500,"land_coverage":0.3,"ridge_elevation_km":-1,"abyssal_elevation_km":-6,"trench_elevation_km":-10,"max_continental_elevation_km":10,"max_plate_speed_km_per_my":100}}
```

Every byte of the file is listed below.
The first column is the offset in hexadecimal, the second the bytes in hexadecimal, and the third the field they encode.
A field longer than 16 bytes continues on the following lines.

<!-- example:begin -->
```text
0000  59 41 45 45 44 41 54 41                          identifier "YAEEDATA"
0008  01 00 00 00                                      format version = 1

000c  4d 45 54 41                                      section kind "META"
0010  71 01 00 00 00 00 00 00                          payload length = 369
0018  7b 22 6c 69 62 72 61 72 79 5f 76 65 72 73 69 6f  payload: the JSON text shown above
0028  6e 22 3a 22 30 2e 31 33 2e 30 2d 61 6c 70 68 61
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
0178  5f 6b 6d 5f 70 65 72 5f 6d 79 22 3a 31 30 30 7d
0188  7d
0189  bd a3 af 13                                      CRC-32 of the 381 bytes from offset 000c = 13afa3bd

018d  53 4d 50 4c                                      section kind "SMPL"
0191  94 00 00 00 00 00 00 00                          payload length = 148
0199  06 00 00 00                                      N = 6 samples
019d  cd f4 a8 62 4c b0 e1 3f                          sample 0 x = 0.5527707983925666
01a5  00 00 00 00 00 00 00 00                          sample 0 y = 0
01ad  ab aa aa aa aa aa ea 3f                          sample 0 z = 0.8333333333333334
01b5  8d d1 d2 b3 3f 6f e4 bf                          sample 1 x = -0.6385801803758554
01bd  17 00 ef a0 40 b8 e2 bf                          sample 1 y = -0.5849917548403053
01c5  00 00 00 00 00 00 e0 3f                          sample 1 z = 0.5
01cd  2a 45 a8 20 65 11 b6 3f                          sample 2 x = 0.08620292708680624
01d5  29 36 2e 29 7e 6e ef 3f                          sample 2 y = 0.9822378925616181
01dd  54 55 55 55 55 55 c5 3f                          sample 2 z = 0.16666666666666663
01e5  be 62 fd e5 9d 32 e3 3f                          sample 3 x = 0.5999288074482008
01ed  da ae 4b 50 3f 0a e9 bf                          sample 3 y = -0.7825008934282451
01f5  58 55 55 55 55 55 c5 bf                          sample 3 z = -0.16666666666666674
01fd  a3 33 61 bd 07 4a eb bf                          sample 4 x = -0.8527868937322761
0205  7c 4c 49 e9 eb 4e c3 3f                          sample 4 y = 0.15084599390920406
020d  00 00 00 00 00 00 e0 bf                          sample 4 z = -0.5
0215  66 a1 22 2d 8d d9 dd 3f                          sample 5 x = 0.46640328796140407
021d  21 af 85 f6 ed fc d2 3f                          sample 5 y = 0.29668759416994034
0225  aa aa aa aa aa aa ea bf                          sample 5 z = -0.8333333333333333
022d  c0 a8 d3 44                                      CRC-32 of the 160 bytes from offset 018d = 44d3a8c0

0231  54 52 49 53                                      section kind "TRIS"
0235  64 00 00 00 00 00 00 00                          payload length = 100
023d  08 00 00 00                                      T = 8 triangles
0241  00 00 00 00 05 00 00 00 02 00 00 00              triangle 0 = samples 0, 5, 2
024d  00 00 00 00 03 00 00 00 05 00 00 00              triangle 1 = samples 0, 3, 5
0259  05 00 00 00 03 00 00 00 04 00 00 00              triangle 2 = samples 5, 3, 4
0265  02 00 00 00 04 00 00 00 01 00 00 00              triangle 3 = samples 2, 4, 1
0271  04 00 00 00 03 00 00 00 01 00 00 00              triangle 4 = samples 4, 3, 1
027d  03 00 00 00 00 00 00 00 01 00 00 00              triangle 5 = samples 3, 0, 1
0289  05 00 00 00 04 00 00 00 02 00 00 00              triangle 6 = samples 5, 4, 2
0295  01 00 00 00 00 00 00 00 02 00 00 00              triangle 7 = samples 1, 0, 2
02a1  3f 76 78 72                                      CRC-32 of the 112 bytes from offset 0231 = 7278763f

02a5  41 54 54 52                                      section kind "ATTR"
02a9  21 00 00 00 00 00 00 00                          payload length = 33
02b1  05                                               name length = 5
02b2  70 6c 61 74 65                                   name "plate"
02b7  00                                               unit length = 0
02b8                                                   unit ""
02b8  02                                               element type = 2 (int32)
02b9  01                                               components = 1
02ba  01 00 00 00                                      plate of sample 0 = 1
02be  01 00 00 00                                      plate of sample 1 = 1
02c2  00 00 00 00                                      plate of sample 2 = 0
02c6  01 00 00 00                                      plate of sample 3 = 1
02ca  00 00 00 00                                      plate of sample 4 = 0
02ce  00 00 00 00                                      plate of sample 5 = 0
02d2  cb 98 75 80                                      CRC-32 of the 45 bytes from offset 02a5 = 807598cb

02d6  41 54 54 52                                      section kind "ATTR"
02da  14 00 00 00 00 00 00 00                          payload length = 20
02e2  0a                                               name length = 10
02e3  63 72 75 73 74 5f 74 79 70 65                    name "crust_type"
02ed  00                                               unit length = 0
02ee                                                   unit ""
02ee  01                                               element type = 1 (uint8)
02ef  01                                               components = 1
02f0  00                                               crust_type of sample 0 = 0
02f1  00                                               crust_type of sample 1 = 0
02f2  01                                               crust_type of sample 2 = 1
02f3  00                                               crust_type of sample 3 = 0
02f4  00                                               crust_type of sample 4 = 0
02f5  01                                               crust_type of sample 5 = 1
02f6  d9 55 40 92                                      CRC-32 of the 32 bytes from offset 02d6 = 924055d9

02fa  41 54 54 52                                      section kind "ATTR"
02fe  3f 00 00 00 00 00 00 00                          payload length = 63
0306  09                                               name length = 9
0307  74 68 69 63 6b 6e 65 73 73                       name "thickness"
0310  02                                               unit length = 2
0311  6b 6d                                            unit "km"
0313  04                                               element type = 4 (float64)
0314  01                                               components = 1
0315  00 00 00 00 00 00 1c 40                          thickness of sample 0 = 7
031d  00 00 00 00 00 00 1c 40                          thickness of sample 1 = 7
0325  00 00 00 00 00 80 41 40                          thickness of sample 2 = 35
032d  00 00 00 00 00 00 1c 40                          thickness of sample 3 = 7
0335  00 00 00 00 00 00 1c 40                          thickness of sample 4 = 7
033d  00 00 00 00 00 80 41 40                          thickness of sample 5 = 35
0345  2c c8 e1 5a                                      CRC-32 of the 75 bytes from offset 02fa = 5ae1c82c

0349  41 54 54 52                                      section kind "ATTR"
034d  3f 00 00 00 00 00 00 00                          payload length = 63
0355  09                                               name length = 9
0356  65 6c 65 76 61 74 69 6f 6e                       name "elevation"
035f  02                                               unit length = 2
0360  6b 6d                                            unit "km"
0362  04                                               element type = 4 (float64)
0363  01                                               components = 1
0364  00 00 00 00 00 00 f0 bf                          elevation of sample 0 = -1
036c  00 00 00 00 00 00 f0 bf                          elevation of sample 1 = -1
0374  9a 99 99 99 99 99 b9 3f                          elevation of sample 2 = 0.1
037c  00 00 00 00 00 00 f0 bf                          elevation of sample 3 = -1
0384  00 00 00 00 00 00 f0 bf                          elevation of sample 4 = -1
038c  9a 99 99 99 99 99 b9 3f                          elevation of sample 5 = 0.1
0394  bb 5d c4 cb                                      CRC-32 of the 75 bytes from offset 0349 = cbc45dbb

0398  41 54 54 52                                      section kind "ATTR"
039c  41 00 00 00 00 00 00 00                          payload length = 65
03a4  0b                                               name length = 11
03a5  6f 63 65 61 6e 69 63 5f 61 67 65                 name "oceanic_age"
03b0  02                                               unit length = 2
03b1  4d 79                                            unit "My"
03b3  04                                               element type = 4 (float64)
03b4  01                                               components = 1
03b5  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 0 = 202.7325540540822
03bd  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 1 = 202.7325540540822
03c5  00 00 00 00 00 00 00 00                          oceanic_age of sample 2 = 0
03cd  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 3 = 202.7325540540822
03d5  ba 1a 33 15 71 57 69 40                          oceanic_age of sample 4 = 202.7325540540822
03dd  00 00 00 00 00 00 00 00                          oceanic_age of sample 5 = 0
03e5  cb b1 9d b9                                      CRC-32 of the 77 bytes from offset 0398 = b99db1cb

03e9  41 54 54 52                                      section kind "ATTR"
03ed  a3 00 00 00 00 00 00 00                          payload length = 163
03f5  0f                                               name length = 15
03f6  72 69 64 67 65 5f 64 69 72 65 63 74 69 6f 6e     name "ridge_direction"
0405  00                                               unit length = 0
0406                                                   unit ""
0406  04                                               element type = 4 (float64)
0407  03                                               components = 3
0408  00 00 00 00 00 00 00 00                          ridge_direction x of sample 0 = 0
0410  00 00 00 00 00 00 00 00                          ridge_direction y of sample 0 = 0
0418  00 00 00 00 00 00 00 00                          ridge_direction z of sample 0 = 0
0420  00 00 00 00 00 00 00 00                          ridge_direction x of sample 1 = 0
0428  00 00 00 00 00 00 00 00                          ridge_direction y of sample 1 = 0
0430  00 00 00 00 00 00 00 00                          ridge_direction z of sample 1 = 0
0438  00 00 00 00 00 00 00 00                          ridge_direction x of sample 2 = 0
0440  00 00 00 00 00 00 00 00                          ridge_direction y of sample 2 = 0
0448  00 00 00 00 00 00 00 00                          ridge_direction z of sample 2 = 0
0450  00 00 00 00 00 00 00 00                          ridge_direction x of sample 3 = 0
0458  00 00 00 00 00 00 00 00                          ridge_direction y of sample 3 = 0
0460  00 00 00 00 00 00 00 00                          ridge_direction z of sample 3 = 0
0468  00 00 00 00 00 00 00 00                          ridge_direction x of sample 4 = 0
0470  00 00 00 00 00 00 00 00                          ridge_direction y of sample 4 = 0
0478  00 00 00 00 00 00 00 00                          ridge_direction z of sample 4 = 0
0480  00 00 00 00 00 00 00 00                          ridge_direction x of sample 5 = 0
0488  00 00 00 00 00 00 00 00                          ridge_direction y of sample 5 = 0
0490  00 00 00 00 00 00 00 00                          ridge_direction z of sample 5 = 0
0498  92 07 59 35                                      CRC-32 of the 175 bytes from offset 03e9 = 35590792

049c  41 54 54 52                                      section kind "ATTR"
04a0  41 00 00 00 00 00 00 00                          payload length = 65
04a8  0b                                               name length = 11
04a9  6f 72 6f 67 65 6e 79 5f 61 67 65                 name "orogeny_age"
04b4  02                                               unit length = 2
04b5  4d 79                                            unit "My"
04b7  04                                               element type = 4 (float64)
04b8  01                                               components = 1
04b9  00 00 00 00 00 00 00 00                          orogeny_age of sample 0 = 0
04c1  00 00 00 00 00 00 00 00                          orogeny_age of sample 1 = 0
04c9  00 00 00 00 00 00 00 00                          orogeny_age of sample 2 = 0
04d1  00 00 00 00 00 00 00 00                          orogeny_age of sample 3 = 0
04d9  00 00 00 00 00 00 00 00                          orogeny_age of sample 4 = 0
04e1  00 00 00 00 00 00 00 00                          orogeny_age of sample 5 = 0
04e9  ae bc 7a 7f                                      CRC-32 of the 77 bytes from offset 049c = 7f7abcae

04ed  41 54 54 52                                      section kind "ATTR"
04f1  16 00 00 00 00 00 00 00                          payload length = 22
04f9  0c                                               name length = 12
04fa  6f 72 6f 67 65 6e 79 5f 74 79 70 65              name "orogeny_type"
0506  00                                               unit length = 0
0507                                                   unit ""
0507  01                                               element type = 1 (uint8)
0508  01                                               components = 1
0509  00                                               orogeny_type of sample 0 = 0
050a  00                                               orogeny_type of sample 1 = 0
050b  00                                               orogeny_type of sample 2 = 0
050c  00                                               orogeny_type of sample 3 = 0
050d  00                                               orogeny_type of sample 4 = 0
050e  00                                               orogeny_type of sample 5 = 0
050f  68 3c d7 22                                      CRC-32 of the 34 bytes from offset 04ed = 22d73c68

0513  41 54 54 52                                      section kind "ATTR"
0517  a2 00 00 00 00 00 00 00                          payload length = 162
051f  0e                                               name length = 14
0520  66 6f 6c 64 5f 64 69 72 65 63 74 69 6f 6e        name "fold_direction"
052e  00                                               unit length = 0
052f                                                   unit ""
052f  04                                               element type = 4 (float64)
0530  03                                               components = 3
0531  00 00 00 00 00 00 00 00                          fold_direction x of sample 0 = 0
0539  00 00 00 00 00 00 00 00                          fold_direction y of sample 0 = 0
0541  00 00 00 00 00 00 00 00                          fold_direction z of sample 0 = 0
0549  00 00 00 00 00 00 00 00                          fold_direction x of sample 1 = 0
0551  00 00 00 00 00 00 00 00                          fold_direction y of sample 1 = 0
0559  00 00 00 00 00 00 00 00                          fold_direction z of sample 1 = 0
0561  00 00 00 00 00 00 00 00                          fold_direction x of sample 2 = 0
0569  00 00 00 00 00 00 00 00                          fold_direction y of sample 2 = 0
0571  00 00 00 00 00 00 00 00                          fold_direction z of sample 2 = 0
0579  00 00 00 00 00 00 00 00                          fold_direction x of sample 3 = 0
0581  00 00 00 00 00 00 00 00                          fold_direction y of sample 3 = 0
0589  00 00 00 00 00 00 00 00                          fold_direction z of sample 3 = 0
0591  00 00 00 00 00 00 00 00                          fold_direction x of sample 4 = 0
0599  00 00 00 00 00 00 00 00                          fold_direction y of sample 4 = 0
05a1  00 00 00 00 00 00 00 00                          fold_direction z of sample 4 = 0
05a9  00 00 00 00 00 00 00 00                          fold_direction x of sample 5 = 0
05b1  00 00 00 00 00 00 00 00                          fold_direction y of sample 5 = 0
05b9  00 00 00 00 00 00 00 00                          fold_direction z of sample 5 = 0
05c1  78 68 10 90                                      CRC-32 of the 174 bytes from offset 0513 = 90106878

05c5  45 4e 44 20                                      section kind "END "
05c9  00 00 00 00 00 00 00 00                          payload length = 0
05d1  1f 11 8b 42                                      CRC-32 of the 12 bytes from offset 05c5 = 428b111f
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
0.13.0-alpha 6 8
[0.5527707983925666 0 0.8333333333333334] [0 5 2]
[1 1 0 1 0 0]
[0 0 1 0 0 1]
[-1 -1 0.1 -1 -1 0.1]
202.7325540540822
[0 0 0]
```
