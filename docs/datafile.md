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
| `library_version` | string | The version of the `yaee` library that generated the world, as a semantic version such as `0.10.0-alpha` |
| `config` | object | The configuration that generated the world |

Members of `config`:

| Member | JSON type | Unit | Range | Meaning |
|--------|-----------|------|-------|---------|
| `seed` | integer | none | 0 to 18446744073709551615 (2^64 − 1) | The seed of all randomness |
| `radius_km` | number | km | greater than 0 | Radius of the sphere at sea level |
| `sample_spacing_km` | number | km | greater than 0 | Distance between neighbouring samples |
| `duration_my` | number | My | at least 0 | Span of time simulated |
| `time_step_my` | number | My | greater than 0 | Length of one simulation step |
| `plates` | integer | none | 1 to 1000, and at most N | Number of tectonic plates at the start |
| `plate_warp_amplitude_km` | number | km | at least 0 | How far the boundaries of the initial plates are bent away from the straight boundaries of Voronoi cells |
| `plate_warp_feature_size_km` | number | km | greater than 0 | Length of the largest bends in the boundaries of the initial plates |

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

Library version 0.10.0-alpha writes these attributes, in this order:

| Name | Element type | Components | Unit | Range | Meaning |
|------|--------------|------------|------|-------|---------|
| `plate` | 2 (int32) | 1 | none (empty) | 0 to `plates` − 1 | The number of the tectonic plate the sample belongs to |
| `elevation` | 4 (float64) | 1 | `km` | −6.606 to 3.000 | Height of the surface above sea level. Negative values are below sea level; 0 is sea level |

`plate`:

- `plates` is the member of `config` in the `META` section.
- Every plate number from 0 to `plates` − 1 is used by at least one sample.
- The samples of one plate form a single connected region: any two of them are joined by a path of neighbouring samples of the same plate. Neighbours are defined under `TRIS`.
- A plate boundary runs between two neighbouring samples whose plate numbers differ. No sample lies on a boundary.

In this library version `elevation` is a placeholder, not a simulated value.
For a sample at (x, y, z) it is 4 × (x² − y²) + 6 × y × z − 1.
It does not depend on `plate`.
A later version replaces it with simulated elevation and a wider range, under the same name, type, component count, and unit.

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
The default world has N = 942,059 and, with the attributes that library version 0.10.0-alpha writes, is 56,523,838 bytes.

## Example

The file `testdata/example.yaee` in the repository is 661 bytes.
It is the world generated by library version 0.10.0-alpha from this configuration, which gives N = 6 samples and T = 8 triangles:

```json
{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":250,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500}
```

Its `META` payload is these 209 bytes of text:

```json
{"library_version":"0.10.0-alpha","config":{"seed":42,"radius_km":6370,"sample_spacing_km":10000,"duration_my":250,"time_step_my":2,"plates":2,"plate_warp_amplitude_km":1500,"plate_warp_feature_size_km":2500}}
```

Every byte of the file is listed below.
The first column is the offset in hexadecimal, the second the bytes in hexadecimal, and the third the field they encode.
A field longer than 16 bytes continues on the following lines.

<!-- example:begin -->
```text
0000  59 41 45 45 44 41 54 41                          identifier "YAEEDATA"
0008  01 00 00 00                                      format version = 1

000c  4d 45 54 41                                      section kind "META"
0010  d1 00 00 00 00 00 00 00                          payload length = 209
0018  7b 22 6c 69 62 72 61 72 79 5f 76 65 72 73 69 6f  payload: the JSON text shown above
0028  6e 22 3a 22 30 2e 31 30 2e 30 2d 61 6c 70 68 61
0038  22 2c 22 63 6f 6e 66 69 67 22 3a 7b 22 73 65 65
0048  64 22 3a 34 32 2c 22 72 61 64 69 75 73 5f 6b 6d
0058  22 3a 36 33 37 30 2c 22 73 61 6d 70 6c 65 5f 73
0068  70 61 63 69 6e 67 5f 6b 6d 22 3a 31 30 30 30 30
0078  2c 22 64 75 72 61 74 69 6f 6e 5f 6d 79 22 3a 32
0088  35 30 2c 22 74 69 6d 65 5f 73 74 65 70 5f 6d 79
0098  22 3a 32 2c 22 70 6c 61 74 65 73 22 3a 32 2c 22
00a8  70 6c 61 74 65 5f 77 61 72 70 5f 61 6d 70 6c 69
00b8  74 75 64 65 5f 6b 6d 22 3a 31 35 30 30 2c 22 70
00c8  6c 61 74 65 5f 77 61 72 70 5f 66 65 61 74 75 72
00d8  65 5f 73 69 7a 65 5f 6b 6d 22 3a 32 35 30 30 7d
00e8  7d
00e9  c5 81 8f a6                                      CRC-32 of the 221 bytes from offset 000c = a68f81c5

00ed  53 4d 50 4c                                      section kind "SMPL"
00f1  94 00 00 00 00 00 00 00                          payload length = 148
00f9  06 00 00 00                                      N = 6 samples
00fd  cd f4 a8 62 4c b0 e1 3f                          sample 0 x = 0.5527707983925666
0105  00 00 00 00 00 00 00 00                          sample 0 y = 0
010d  ab aa aa aa aa aa ea 3f                          sample 0 z = 0.8333333333333334
0115  8d d1 d2 b3 3f 6f e4 bf                          sample 1 x = -0.6385801803758554
011d  17 00 ef a0 40 b8 e2 bf                          sample 1 y = -0.5849917548403053
0125  00 00 00 00 00 00 e0 3f                          sample 1 z = 0.5
012d  2a 45 a8 20 65 11 b6 3f                          sample 2 x = 0.08620292708680624
0135  29 36 2e 29 7e 6e ef 3f                          sample 2 y = 0.9822378925616181
013d  54 55 55 55 55 55 c5 3f                          sample 2 z = 0.16666666666666663
0145  be 62 fd e5 9d 32 e3 3f                          sample 3 x = 0.5999288074482008
014d  da ae 4b 50 3f 0a e9 bf                          sample 3 y = -0.7825008934282451
0155  58 55 55 55 55 55 c5 bf                          sample 3 z = -0.16666666666666674
015d  a3 33 61 bd 07 4a eb bf                          sample 4 x = -0.8527868937322761
0165  7c 4c 49 e9 eb 4e c3 3f                          sample 4 y = 0.15084599390920406
016d  00 00 00 00 00 00 e0 bf                          sample 4 z = -0.5
0175  66 a1 22 2d 8d d9 dd 3f                          sample 5 x = 0.46640328796140407
017d  21 af 85 f6 ed fc d2 3f                          sample 5 y = 0.29668759416994034
0185  aa aa aa aa aa aa ea bf                          sample 5 z = -0.8333333333333333
018d  c0 a8 d3 44                                      CRC-32 of the 160 bytes from offset 00ed = 44d3a8c0

0191  54 52 49 53                                      section kind "TRIS"
0195  64 00 00 00 00 00 00 00                          payload length = 100
019d  08 00 00 00                                      T = 8 triangles
01a1  00 00 00 00 05 00 00 00 02 00 00 00              triangle 0 = samples 0, 5, 2
01ad  00 00 00 00 03 00 00 00 05 00 00 00              triangle 1 = samples 0, 3, 5
01b9  05 00 00 00 03 00 00 00 04 00 00 00              triangle 2 = samples 5, 3, 4
01c5  02 00 00 00 04 00 00 00 01 00 00 00              triangle 3 = samples 2, 4, 1
01d1  04 00 00 00 03 00 00 00 01 00 00 00              triangle 4 = samples 4, 3, 1
01dd  03 00 00 00 00 00 00 00 01 00 00 00              triangle 5 = samples 3, 0, 1
01e9  05 00 00 00 04 00 00 00 02 00 00 00              triangle 6 = samples 5, 4, 2
01f5  01 00 00 00 00 00 00 00 02 00 00 00              triangle 7 = samples 1, 0, 2
0201  3f 76 78 72                                      CRC-32 of the 112 bytes from offset 0191 = 7278763f

0205  41 54 54 52                                      section kind "ATTR"
0209  21 00 00 00 00 00 00 00                          payload length = 33
0211  05                                               name length = 5
0212  70 6c 61 74 65                                   name "plate"
0217  00                                               unit length = 0
0218                                                   unit ""
0218  02                                               element type = 2 (int32)
0219  01                                               components = 1
021a  01 00 00 00                                      plate of sample 0 = 1
021e  01 00 00 00                                      plate of sample 1 = 1
0222  00 00 00 00                                      plate of sample 2 = 0
0226  01 00 00 00                                      plate of sample 3 = 1
022a  00 00 00 00                                      plate of sample 4 = 0
022e  00 00 00 00                                      plate of sample 5 = 0
0232  cb 98 75 80                                      CRC-32 of the 45 bytes from offset 0205 = 807598cb

0236  41 54 54 52                                      section kind "ATTR"
023a  3f 00 00 00 00 00 00 00                          payload length = 63
0242  09                                               name length = 9
0243  65 6c 65 76 61 74 69 6f 6e                       name "elevation"
024c  02                                               unit length = 2
024d  6b 6d                                            unit "km"
024f  04                                               element type = 4 (float64)
0250  01                                               components = 1
0251  18 c7 71 1c c7 71 cc 3f                          elevation of sample 0 = 0.2222222222222221
0259  13 87 47 b2 0b f1 03 c0                          elevation of sample 1 = -2.4926980903700353
0261  fa 16 c4 98 12 c7 0e c0                          elevation of sample 2 = -3.8472034392206043
0269  d8 1a b8 9f 15 a2 f3 bf                          elevation of sample 3 = -1.2270714034108803
0271  06 a3 0b d6 c8 d8 f5 3f                          elevation of sample 4 = 1.365425907244743
0279  97 93 c3 70 4b 72 ff bf                          elevation of sample 5 = -1.9654039769022569
0281  8a 84 8a 2f                                      CRC-32 of the 75 bytes from offset 0236 = 2f8a848a

0285  45 4e 44 20                                      section kind "END "
0289  00 00 00 00 00 00 00 00                          payload length = 0
0291  1f 11 8b 42                                      CRC-32 of the 12 bytes from offset 0285 = 428b111f
```
<!-- example:end -->

From the listing: sample 0 is at latitude asin(0.8333333333333334) = 0.9851 radians (56.44° north) and longitude atan2(0, 0.5527707983925666) = 0.
Its elevation is 0.2222 km, so its point on the surface is 6370.2222 × (0.5527707983925666, 0, 0.8333333333333334) km from the centre.
It belongs to plate 1, as do samples 1 and 3; samples 2, 4, and 5 belong to plate 0.
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
	fmt.Println(world.Attribute("elevation").Float64s[0])
}
```

Output:

```text
0.10.0-alpha 6 8
[0.5527707983925666 0 0.8333333333333334] [0 5 2]
[1 1 0 1 0 0]
0.2222222222222221
```
