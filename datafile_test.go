// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/maloquacious/yaee"
)

var update = flag.Bool("update", false, "rewrite testdata/example.yaee and the example in docs/datafile.md")

func encode(t testing.TB, w *yaee.World) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := yaee.Write(&buf, w); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return buf.Bytes()
}

// section encodes one section as docs/datafile.md specifies it,
// independently of the writer.
func section(kind string, payload []byte) []byte {
	b := []byte(kind)
	b = binary.LittleEndian.AppendUint64(b, uint64(len(payload)))
	b = append(b, payload...)
	return binary.LittleEndian.AppendUint32(b, crc32.ChecksumIEEE(b))
}

// attrPayload encodes the payload of an ATTR section.
func attrPayload(name, unit string, typ, components byte, data []byte) []byte {
	b := append([]byte{byte(len(name))}, name...)
	b = append(append(b, byte(len(unit))), unit...)
	return append(append(b, typ, components), data...)
}

// handWorld is a world with every element type, several components, and
// values that are easy to corrupt in a round trip.
func handWorld() *yaee.World {
	return &yaee.World{
		Config:    yaee.Config{Seed: math.MaxUint64, Radius: 1, SampleSpacing: 0.1 + 0.2, Duration: 0, TimeStep: math.Pi},
		Version:   "9.8.7-test",
		Positions: [][3]float64{{0, 0, 1}, {1, 0, 0}, {0, 1, 0}, {math.Copysign(0, -1), 0, -1}},
		Triangles: [][3]int32{{0, 1, 2}, {3, 2, 1}},
		Attributes: []yaee.Attribute{
			{Name: "kind", Unit: "", Type: yaee.Uint8, Components: 1, Uint8s: []uint8{0, 1, 254, 255}},
			{Name: "plate", Unit: "", Type: yaee.Int32, Components: 1, Int32s: []int32{math.MinInt32, -1, 0, math.MaxInt32}},
			{Name: "température", Unit: "°C", Type: yaee.Float32, Components: 2,
				Float32s: []float32{0, float32(math.Copysign(0, -1)), math.SmallestNonzeroFloat32, math.MaxFloat32, float32(math.Inf(-1)), 1.5, -2.5, 3}},
			{Name: "fold", Unit: "km", Type: yaee.Float64, Components: 3,
				Float64s: []float64{0, math.Copysign(0, -1), math.SmallestNonzeroFloat64, math.MaxFloat64, math.Inf(1), math.Inf(-1), 0.1, 0.2, 0.3, -1, -2, -3}},
		},
	}
}

func TestRoundTripGenerated(t *testing.T) {
	w := mustGenerate(t, smallConfig(2000))
	data := encode(t, w)
	got, err := yaee.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got, w) {
		t.Fatal("the world read back differs from the world written")
	}
	// DeepEqual treats 0 and -0 as equal; the bytes do not.
	if !bytes.Equal(encode(t, got), data) {
		t.Fatal("writing the world read back gives different bytes")
	}
	// The size the specification gives, with J the length of the META
	// JSON.
	want := 12 + (16 + 4 + 24*2000) + (16 + 4 + 12*3996) + 16
	for _, a := range w.Attributes {
		want += 16 + 4 + len(a.Name) + len(a.Unit) + a.Type.Size()*a.Components*2000
	}
	if j := len(data) - want - 16; j < 100 || j > 1000 {
		t.Fatalf("file is %d bytes; want %d plus a META section, which leaves %d bytes of JSON", len(data), want, j)
	}
}

func TestRoundTripEveryType(t *testing.T) {
	w := handWorld()
	data := encode(t, w)
	got, err := yaee.Read(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if !reflect.DeepEqual(got, w) {
		t.Fatalf("round trip changed the world:\n got %+v\nwant %+v", got, w)
	}
	if !bytes.Equal(encode(t, got), data) {
		t.Fatal("writing the world read back gives different bytes")
	}
}

// NaN is not equal to itself, so compare its bits.
func TestRoundTripNaNBits(t *testing.T) {
	w := handWorld()
	nan64 := math.Float64frombits(0x7ff8_dead_beef_0001)
	nan32 := math.Float32frombits(0x7fc1_2345)
	w.Attributes[3].Float64s[0] = nan64
	w.Attributes[2].Float32s[0] = nan32
	got, err := yaee.Read(bytes.NewReader(encode(t, w)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if b := math.Float64bits(got.Attributes[3].Float64s[0]); b != math.Float64bits(nan64) {
		t.Errorf("float64 NaN came back as %016x", b)
	}
	if b := math.Float32bits(got.Attributes[2].Float32s[0]); b != math.Float32bits(nan32) {
		t.Errorf("float32 NaN came back as %08x", b)
	}
}

func TestWriteTwiceIdentical(t *testing.T) {
	w := mustGenerate(t, smallConfig(1000))
	if !bytes.Equal(encode(t, w), encode(t, w)) {
		t.Fatal("writing the same world twice gave different bytes")
	}
	if !bytes.Equal(encode(t, w), encode(t, mustGenerate(t, smallConfig(1000)))) {
		t.Fatal("two worlds from equal Configs gave different bytes")
	}
}

func TestReadStopsAtEnd(t *testing.T) {
	data := encode(t, handWorld())
	r := bytes.NewReader(append(append([]byte{}, data...), "trailing"...))
	if _, err := yaee.Read(r); err != nil {
		t.Fatalf("Read: %v", err)
	}
	rest, _ := io.ReadAll(r)
	if string(rest) != "trailing" {
		t.Fatalf("Read left %q unread, want %q", rest, "trailing")
	}
}

func TestReadNotDatafile(t *testing.T) {
	for _, in := range []string{"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", "yaeedata\x01\x00\x00\x00", `{"seed":0}  `} {
		if _, err := yaee.Read(strings.NewReader(in)); !errors.Is(err, yaee.ErrNotDatafile) {
			t.Errorf("%q: got %v, want ErrNotDatafile", in, err)
		}
	}
}

func TestReadUnknownVersion(t *testing.T) {
	for _, v := range []uint32{0, 2, 7, 0x01000000} {
		data := encode(t, handWorld())
		binary.LittleEndian.PutUint32(data[8:], v)
		_, err := yaee.Read(bytes.NewReader(data))
		if !errors.Is(err, yaee.ErrUnsupportedVersion) {
			t.Errorf("version %d: got %v, want ErrUnsupportedVersion", v, err)
			continue
		}
		if !strings.Contains(err.Error(), fmt.Sprintf("found version %d,", v)) {
			t.Errorf("version %d: the error does not state the version found: %v", v, err)
		}
	}
}

// Every proper prefix of a file is a truncated file. That covers every
// section boundary and the inside of every section.
func TestReadTruncated(t *testing.T) {
	for _, w := range []*yaee.World{handWorld(), mustGenerate(t, smallConfig(12))} {
		data := encode(t, w)
		for n := range len(data) {
			got, err := yaee.Read(bytes.NewReader(data[:n]))
			if !errors.Is(err, yaee.ErrTruncated) {
				t.Fatalf("first %d of %d bytes: got %v, want ErrTruncated", n, len(data), err)
			}
			if got != nil {
				t.Fatalf("first %d of %d bytes: Read returned a world with the error", n, len(data))
			}
		}
	}
}

// Changing any one byte must be detected, whatever it is changed to.
func TestReadOneByteChanged(t *testing.T) {
	data := encode(t, handWorld())
	for i := range data {
		for _, flip := range []byte{0x01, 0x80, 0xff} {
			bad := bytes.Clone(data)
			bad[i] ^= flip
			if _, err := yaee.Read(bytes.NewReader(bad)); err == nil {
				t.Fatalf("byte %d changed from %02x to %02x: Read returned no error", i, data[i], bad[i])
			}
		}
	}
}

// A file whose length field claims far more data than exists must be
// rejected without allocating what it claims.
func TestReadHugeLength(t *testing.T) {
	data := append([]byte("YAEEDATA\x01\x00\x00\x00META"), 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f)
	data = append(data, "{}"...)
	if _, err := yaee.Read(bytes.NewReader(data)); !errors.Is(err, yaee.ErrTruncated) {
		t.Fatalf("got %v, want ErrTruncated", err)
	}
}

// parts returns the header and the sections of a valid file for a world
// of 4 samples, built by hand from the specification.
func parts() (header, meta, samples, tris, attr, end []byte) {
	le := binary.LittleEndian
	header = []byte("YAEEDATA\x01\x00\x00\x00")
	meta = section("META", []byte(`{"library_version":"1.2.3","config":{"seed":5,"radius_km":1,"sample_spacing_km":2,"duration_my":3,"time_step_my":4},"added_later":[1,2]}`))
	p := le.AppendUint32(nil, 4)
	for _, v := range []float64{0, 0, 1, 1, 0, 0, 0, 1, 0, 0, 0, -1} {
		p = le.AppendUint64(p, math.Float64bits(v))
	}
	samples = section("SMPL", p)
	p = le.AppendUint32(nil, 2)
	for _, v := range []uint32{0, 1, 2, 3, 2, 1} {
		p = le.AppendUint32(p, v)
	}
	tris = section("TRIS", p)
	attr = section("ATTR", attrPayload("kind", "", 1, 1, []byte{9, 8, 7, 6}))
	end = section("END ", nil)
	return
}

func join(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

func TestReadHandBuiltFile(t *testing.T) {
	header, meta, samples, tris, attr, end := parts()
	got, err := yaee.Read(bytes.NewReader(join(header, meta, samples, tris, attr, end)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := &yaee.World{
		Config:     yaee.Config{Seed: 5, Radius: 1, SampleSpacing: 2, Duration: 3, TimeStep: 4},
		Version:    "1.2.3",
		Positions:  [][3]float64{{0, 0, 1}, {1, 0, 0}, {0, 1, 0}, {0, 0, -1}},
		Triangles:  [][3]int32{{0, 1, 2}, {3, 2, 1}},
		Attributes: []yaee.Attribute{{Name: "kind", Type: yaee.Uint8, Components: 1, Uint8s: []uint8{9, 8, 7, 6}}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

// A reader skips what a later version of the library added: an attribute
// with an element type it does not know, and a section of a kind it does
// not know. It still reads everything else.
func TestReadSkipsUnknown(t *testing.T) {
	header, meta, samples, tris, attr, end := parts()
	future := section("ATTR", attrPayload("future", "furlong", 200, 3, []byte("any number of bytes, since the type is unknown")))
	other := section("PLAT", []byte("plate table"))
	second := section("ATTR", attrPayload("age", "My", 4, 1, make([]byte, 32)))

	got, err := yaee.Read(bytes.NewReader(join(header, meta, other, samples, tris, future, attr, other, second, end)))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got.Attributes) != 2 || got.Attributes[0].Name != "kind" || got.Attributes[1].Name != "age" {
		t.Fatalf("attributes read: %+v; want kind and age", got.Attributes)
	}
	if got.Attribute("future") != nil {
		t.Fatal("the attribute of unknown type was not skipped")
	}
	if got.NumSamples() != 4 || got.NumTriangles() != 2 || got.Version != "1.2.3" {
		t.Fatalf("the rest of the file was not read: %+v", got)
	}

	// The checksum of a skipped section is still verified.
	bad := bytes.Clone(other)
	bad[14] ^= 1
	if _, err := yaee.Read(bytes.NewReader(join(header, meta, bad, samples, tris, attr, end))); !errors.Is(err, yaee.ErrCorrupt) {
		t.Fatalf("corrupt unknown section: got %v, want ErrCorrupt", err)
	}
}

// Files whose checksums are right but whose contents break the format.
func TestReadCorrupt(t *testing.T) {
	header, meta, samples, tris, attr, end := parts()
	le := binary.LittleEndian
	tests := []struct {
		name string
		file []byte
	}{
		{"no META", join(header, samples, tris, attr, end)},
		{"no SMPL", join(header, meta, end)},
		{"no TRIS", join(header, meta, samples, attr, end)},
		{"two META", join(header, meta, meta, samples, tris, end)},
		{"two SMPL", join(header, meta, samples, samples, tris, end)},
		{"two TRIS", join(header, meta, samples, tris, tris, end)},
		{"ATTR before SMPL", join(header, meta, attr, samples, tris, end)},
		{"TRIS before SMPL", join(header, meta, tris, samples, end)},
		{"duplicate attribute", join(header, meta, samples, tris, attr, attr, end)},
		{"END with payload", join(header, meta, samples, tris, section("END ", []byte{0}))},
		{"META not JSON", join(header, section("META", []byte("not json")), samples, tris, end)},
		{"META without config", join(header, section("META", []byte(`{"library_version":"1"}`)), samples, tris, end)},
		{"META without version", join(header, section("META", []byte(`{"config":{}}`)), samples, tris, end)},
		{"SMPL too short", join(header, meta, section("SMPL", []byte{1, 0}), tris, end)},
		{"SMPL wrong length", join(header, meta, section("SMPL", le.AppendUint32(nil, 4)), tris, end)},
		{"TRIS wrong length", join(header, meta, samples, section("TRIS", le.AppendUint32(nil, 2)), end)},
		{"TRIS corner out of range", join(header, meta, samples, section("TRIS", []byte{1, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 4, 0, 0, 0}), end)},
		{"ATTR empty", join(header, meta, samples, tris, section("ATTR", nil), end)},
		{"ATTR empty name", join(header, meta, samples, tris, section("ATTR", attrPayload("", "", 1, 1, make([]byte, 4))), end)},
		{"ATTR name runs past the end", join(header, meta, samples, tris, section("ATTR", []byte{200, 'a'}), end)},
		{"ATTR without type", join(header, meta, samples, tris, section("ATTR", []byte{1, 'a', 0}), end)},
		{"ATTR name not UTF-8", join(header, meta, samples, tris, section("ATTR", attrPayload("\xff", "", 1, 1, make([]byte, 4))), end)},
		{"ATTR unit not UTF-8", join(header, meta, samples, tris, section("ATTR", attrPayload("a", "\xff", 1, 1, make([]byte, 4))), end)},
		{"ATTR zero components", join(header, meta, samples, tris, section("ATTR", attrPayload("a", "", 1, 0, nil)), end)},
		{"ATTR wrong data length", join(header, meta, samples, tris, section("ATTR", attrPayload("a", "", 4, 1, make([]byte, 31))), end)},
	}
	for _, tt := range tests {
		if _, err := yaee.Read(bytes.NewReader(tt.file)); !errors.Is(err, yaee.ErrCorrupt) {
			t.Errorf("%s: got %v, want ErrCorrupt", tt.name, err)
		}
	}
}

func TestWriteInvalidWorld(t *testing.T) {
	tests := []struct {
		name   string
		change func(*yaee.World)
	}{
		{"corner out of range", func(w *yaee.World) { w.Triangles[0][1] = 4 }},
		{"negative corner", func(w *yaee.World) { w.Triangles[1][2] = -1 }},
		{"empty name", func(w *yaee.World) { w.Attributes[0].Name = "" }},
		{"long name", func(w *yaee.World) { w.Attributes[0].Name = strings.Repeat("n", 256) }},
		{"name not UTF-8", func(w *yaee.World) { w.Attributes[0].Name = "\xff" }},
		{"duplicate name", func(w *yaee.World) { w.Attributes[1].Name = w.Attributes[0].Name }},
		{"long unit", func(w *yaee.World) { w.Attributes[0].Unit = strings.Repeat("u", 256) }},
		{"unknown type", func(w *yaee.World) { w.Attributes[0].Type = 0 }},
		{"zero components", func(w *yaee.World) { w.Attributes[0].Components = 0 }},
		{"too many components", func(w *yaee.World) { w.Attributes[0].Components = 256 }},
		{"short data", func(w *yaee.World) { w.Attributes[3].Float64s = w.Attributes[3].Float64s[:11] }},
		{"data of another type", func(w *yaee.World) { w.Attributes[0].Int32s = []int32{1} }},
		{"wrong type for data", func(w *yaee.World) { w.Attributes[1].Type = yaee.Float32 }},
		{"Config not encodable", func(w *yaee.World) { w.Config.Radius = math.NaN() }},
	}
	for _, tt := range tests {
		w := handWorld()
		tt.change(w)
		var buf bytes.Buffer
		if err := yaee.Write(&buf, w); !errors.Is(err, yaee.ErrInvalidWorld) {
			t.Errorf("%s: got %v, want ErrInvalidWorld", tt.name, err)
		}
		if buf.Len() != 0 {
			t.Errorf("%s: Write wrote %d bytes of an invalid world", tt.name, buf.Len())
		}
	}
	if err := yaee.Write(io.Discard, nil); !errors.Is(err, yaee.ErrInvalidWorld) {
		t.Errorf("nil world: got %v, want ErrInvalidWorld", err)
	}
}

type failAfter struct{ calls int }

func (f *failAfter) Write(p []byte) (int, error) {
	if f.calls == 0 {
		return 0, errDiskFull
	}
	f.calls--
	return len(p), nil
}

var errDiskFull = errors.New("disk full")

func TestWriteReturnsWriterError(t *testing.T) {
	// The header and each of the eight sections is one call.
	for calls := range 9 {
		if err := yaee.Write(&failAfter{calls: calls}, handWorld()); !errors.Is(err, errDiskFull) {
			t.Errorf("writer failing on call %d: got %v, want its error", calls+1, err)
		}
	}
	if err := yaee.Write(&failAfter{calls: 9}, handWorld()); err != nil {
		t.Errorf("writer that does not fail: %v", err)
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errDiskFull }

func TestReadReturnsReaderError(t *testing.T) {
	if _, err := yaee.Read(failingReader{}); !errors.Is(err, errDiskFull) {
		t.Fatalf("got %v, want the reader's error", err)
	}
}

// exampleWorld is the world of the example in docs/datafile.md. Its
// recorded library version is fixed, so that the example does not change
// with every release.
func exampleWorld(t testing.TB) *yaee.World {
	cfg := yaee.DefaultConfig()
	cfg.Seed = 42
	cfg.SampleSpacing = 10000
	cfg.Plates = 2
	cfg.Duration = 0 // the initial world, which later versions change least
	w := mustGenerate(t, cfg)
	w.Version = "1.1.0"
	return w
}

const (
	examplePath  = "testdata/example.yaee"
	specPath     = "docs/datafile.md"
	exampleBegin = "<!-- example:begin -->\n"
	exampleEnd   = "<!-- example:end -->\n"
)

// The committed example file is what the writer produces, and the
// specification's annotated listing is a listing of that file.
//
// After a deliberate change to the output, run
//
//	go test . -run TestExampleFile -update
//
// and state the change in the commit message.
func TestExampleFile(t *testing.T) {
	data := encode(t, exampleWorld(t))
	listing := exampleBegin + "```text\n" + annotate(t, data) + "```\n" + exampleEnd

	spec, err := os.ReadFile(specPath)
	if err != nil {
		t.Fatal(err)
	}
	i, j := bytes.Index(spec, []byte(exampleBegin)), bytes.Index(spec, []byte(exampleEnd))
	if i < 0 || j < i {
		t.Fatalf("%s has no example markers", specPath)
	}
	j += len(exampleEnd)

	if *update {
		if err := os.WriteFile(examplePath, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(specPath, join(spec[:i], []byte(listing), spec[j:]), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}

	committed, err := os.ReadFile(examplePath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(committed, data) {
		t.Errorf("%s is not what the writer produces", examplePath)
	}
	if string(spec[i:j]) != listing {
		t.Errorf("the example in %s is not a listing of %s", specPath, examplePath)
	}
	if !bytes.Contains(spec, fmt.Appendf(nil, "is %d bytes", len(data))) {
		t.Errorf("%s does not state the example's size, %d bytes", specPath, len(data))
	}

	got, err := yaee.Read(bytes.NewReader(committed))
	if err != nil {
		t.Fatalf("Read(%s): %v", examplePath, err)
	}
	if !reflect.DeepEqual(got, exampleWorld(t)) {
		t.Errorf("%s does not read back as the example world", examplePath)
	}
}

// annotate lists a datafile field by field: offset, bytes, meaning. It
// walks the bytes by the rules of the specification, independently of
// Read.
func annotate(t testing.TB, data []byte) string {
	t.Helper()
	le := binary.LittleEndian
	var out strings.Builder
	off := 0
	field := func(n int, format string, args ...any) []byte {
		t.Helper()
		if off+n > len(data) {
			t.Fatalf("annotate: field of %d bytes at offset %d runs past the end", n, off)
		}
		b := data[off : off+n]
		text := fmt.Sprintf(format, args...)
		for i := 0; i < n || i == 0; i += 16 {
			row := b[i:min(n, i+16)]
			fmt.Fprintf(&out, "%04x  %-47s  %s\n", off+i, fmt.Sprintf("% x", row), text)
			text = ""
		}
		off += n
		return b
	}
	lazy := func(n int, describe func(b []byte) string) []byte {
		return field(n, "%s", describe(data[off:min(len(data), off+n)]))
	}
	float := func(v float64) string { return strconv.FormatFloat(v, 'g', -1, 64) }
	trim := func() string {
		lines := strings.Split(out.String(), "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " ")
		}
		return strings.Join(lines, "\n")
	}

	field(8, "identifier \"YAEEDATA\"")
	lazy(4, func(b []byte) string { return fmt.Sprintf("format version = %d", le.Uint32(b)) })

	n := 0
	for {
		out.WriteString("\n")
		kind := string(field(4, "section kind %q", data[off:off+4]))
		length := int(le.Uint64(lazy(8, func(b []byte) string { return fmt.Sprintf("payload length = %d", le.Uint64(b)) })))
		end := off + length
		switch kind {
		case "META":
			field(length, "payload: the JSON text shown above")
		case "SMPL":
			n = int(le.Uint32(lazy(4, func(b []byte) string { return fmt.Sprintf("N = %d samples", le.Uint32(b)) })))
			for i := range n {
				for _, axis := range "xyz" {
					lazy(8, func(b []byte) string {
						return fmt.Sprintf("sample %d %c = %s", i, axis, float(math.Float64frombits(le.Uint64(b))))
					})
				}
			}
		case "TRIS":
			count := int(le.Uint32(lazy(4, func(b []byte) string { return fmt.Sprintf("T = %d triangles", le.Uint32(b)) })))
			for i := range count {
				lazy(12, func(b []byte) string {
					return fmt.Sprintf("triangle %d = samples %d, %d, %d", i, le.Uint32(b), le.Uint32(b[4:]), le.Uint32(b[8:]))
				})
			}
		case "ATTR":
			l := int(lazy(1, func(b []byte) string { return fmt.Sprintf("name length = %d", b[0]) })[0])
			name := string(field(l, "name %q", data[off:off+l]))
			l = int(lazy(1, func(b []byte) string { return fmt.Sprintf("unit length = %d", b[0]) })[0])
			field(l, "unit %q", data[off:off+l])
			typ := yaee.ElementType(lazy(1, func(b []byte) string { return fmt.Sprintf("element type = %d (%s)", b[0], yaee.ElementType(b[0])) })[0])
			comps := int(lazy(1, func(b []byte) string { return fmt.Sprintf("components = %d", b[0]) })[0])
			for i := range n {
				for c := range comps {
					what := fmt.Sprintf("%s of sample %d", name, i)
					if comps == 3 {
						what = fmt.Sprintf("%s %c of sample %d", name, "xyz"[c], i)
					} else if comps != 1 {
						what = fmt.Sprintf("%s component %d of sample %d", name, c, i)
					}
					lazy(typ.Size(), func(b []byte) string {
						switch typ {
						case yaee.Uint8:
							return fmt.Sprintf("%s = %d", what, b[0])
						case yaee.Int32:
							return fmt.Sprintf("%s = %d", what, int32(le.Uint32(b)))
						case yaee.Float32:
							return fmt.Sprintf("%s = %s", what, strconv.FormatFloat(float64(math.Float32frombits(le.Uint32(b))), 'g', -1, 32))
						case yaee.Float64:
							return fmt.Sprintf("%s = %s", what, float(math.Float64frombits(le.Uint64(b))))
						}
						t.Fatalf("annotate: attribute %q has unknown element type %d", name, typ)
						return ""
					})
				}
			}
		case "END ":
		default:
			t.Fatalf("annotate: unknown section %q", kind)
		}
		if off != end {
			t.Fatalf("annotate: section %q ends at offset %d, its length says %d", kind, off, end)
		}
		crc := crc32.ChecksumIEEE(data[end-length-12 : end])
		if got := le.Uint32(field(4, "CRC-32 of the %d bytes from offset %04x = %08x", length+12, end-length-12, crc)); got != crc {
			t.Fatalf("annotate: section %q records checksum %08x, computed %08x", kind, got, crc)
		}
		if kind == "END " {
			break
		}
	}
	if off != len(data) {
		t.Fatalf("annotate: %d bytes follow the END section", len(data)-off)
	}
	return trim()
}
