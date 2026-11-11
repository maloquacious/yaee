// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

package yaee

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"unicode/utf8"

	"github.com/maloquacious/yaee/internal/cerrs"
)

// FormatVersion is the version of the datafile format that Write
// produces and Read accepts. The format is specified in docs/datafile.md.
const FormatVersion = 1

// Errors returned by Read and Write. Each is wrapped with detail; test
// for it with errors.Is.
const (
	// ErrNotDatafile means the input does not begin with the datafile
	// identifier.
	ErrNotDatafile = cerrs.Error("yaee: not a datafile")

	// ErrUnsupportedVersion means the input is a datafile of a format
	// version other than FormatVersion. The error text states the
	// version found.
	ErrUnsupportedVersion = cerrs.Error("yaee: unsupported datafile format version")

	// ErrTruncated means the input ended before the end of the datafile.
	ErrTruncated = cerrs.Error("yaee: truncated datafile")

	// ErrCorrupt means a checksum does not match or the contents break
	// a rule of the format.
	ErrCorrupt = cerrs.Error("yaee: corrupt datafile")

	// ErrInvalidWorld means the World given to Write is not consistent:
	// for example, an attribute's data has the wrong length.
	ErrInvalidWorld = cerrs.Error("yaee: invalid world")
)

// The datafile's fixed identifier and its section kinds.
const (
	fileMagic   = "YAEEDATA"
	kindMeta    = "META"
	kindSamples = "SMPL"
	kindTris    = "TRIS"
	kindAttr    = "ATTR"
	kindEnd     = "END "
)

// fileMeta is the JSON document in the META section.
type fileMeta struct {
	LibraryVersion *string `json:"library_version"`
	Config         *Config `json:"config"`
}

// Write encodes the world as a datafile, in the format specified in
// docs/datafile.md, version FormatVersion.
//
// The bytes written depend only on the world: equal worlds give equal
// bytes. Write does not buffer or compress; it makes one call to
// w.Write for the header and one for each section.
//
// It returns an error wrapping ErrInvalidWorld, having written nothing,
// if the world is not consistent. It returns any error from w.
func Write(w io.Writer, world *World) error {
	if err := world.check(); err != nil {
		return err
	}
	version := world.Version
	meta, err := json.Marshal(fileMeta{LibraryVersion: &version, Config: &world.Config})
	if err != nil {
		return fmt.Errorf("%w: Config cannot be encoded: %v", ErrInvalidWorld, err)
	}

	le := binary.LittleEndian
	header := le.AppendUint32([]byte(fileMagic), FormatVersion)
	if _, err := w.Write(header); err != nil {
		return err
	}

	// Each section is assembled whole, so that its length and checksum
	// are known before any of it is written.
	var buf []byte
	begin := func(kind string, payload int) {
		buf = make([]byte, 0, 12+payload+4)
		buf = append(buf, kind...)
		buf = le.AppendUint64(buf, uint64(payload))
	}
	end := func() error {
		buf = le.AppendUint32(buf, crc32.ChecksumIEEE(buf))
		_, err := w.Write(buf)
		buf = nil
		return err
	}

	begin(kindMeta, len(meta))
	buf = append(buf, meta...)
	if err := end(); err != nil {
		return err
	}

	begin(kindSamples, 4+24*len(world.Positions))
	buf = le.AppendUint32(buf, uint32(len(world.Positions)))
	for _, p := range world.Positions {
		for _, v := range p {
			buf = le.AppendUint64(buf, math.Float64bits(v))
		}
	}
	if err := end(); err != nil {
		return err
	}

	begin(kindTris, 4+12*len(world.Triangles))
	buf = le.AppendUint32(buf, uint32(len(world.Triangles)))
	for _, t := range world.Triangles {
		for _, v := range t {
			buf = le.AppendUint32(buf, uint32(v))
		}
	}
	if err := end(); err != nil {
		return err
	}

	for i := range world.Attributes {
		a := &world.Attributes[i]
		count := len(world.Positions) * a.Components
		begin(kindAttr, 1+len(a.Name)+1+len(a.Unit)+2+count*a.Type.Size())
		buf = append(buf, byte(len(a.Name)))
		buf = append(buf, a.Name...)
		buf = append(buf, byte(len(a.Unit)))
		buf = append(buf, a.Unit...)
		buf = append(buf, byte(a.Type), byte(a.Components))
		switch a.Type {
		case Uint8:
			buf = append(buf, a.Uint8s...)
		case Int32:
			for _, v := range a.Int32s {
				buf = le.AppendUint32(buf, uint32(v))
			}
		case Float32:
			for _, v := range a.Float32s {
				buf = le.AppendUint32(buf, math.Float32bits(v))
			}
		case Float64:
			for _, v := range a.Float64s {
				buf = le.AppendUint64(buf, math.Float64bits(v))
			}
		}
		if err := end(); err != nil {
			return err
		}
	}

	begin(kindEnd, 0)
	return end()
}

// check reports whether the world can be written.
func (w *World) check() error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidWorld, fmt.Sprintf(format, args...))
	}
	if w == nil {
		return invalid("nil World")
	}
	n := len(w.Positions)
	if n > MaxSamples {
		return invalid("%d samples; the limit is %d", n, MaxSamples)
	}
	if len(w.Triangles) > math.MaxUint32 {
		return invalid("%d triangles; the limit is %d", len(w.Triangles), uint32(math.MaxUint32))
	}
	if !utf8.ValidString(w.Version) {
		return invalid("Version is not valid UTF-8")
	}
	for t, tri := range w.Triangles {
		for _, v := range tri {
			if v < 0 || int(v) >= n {
				return invalid("triangle %d has corner %d; there are %d samples", t, v, n)
			}
		}
	}
	seen := make(map[string]bool, len(w.Attributes))
	for i := range w.Attributes {
		a := &w.Attributes[i]
		if len(a.Name) < 1 || len(a.Name) > 255 || !utf8.ValidString(a.Name) {
			return invalid("attribute %d: name %q must be 1 to 255 bytes of UTF-8", i, a.Name)
		}
		if seen[a.Name] {
			return invalid("attribute %q appears more than once", a.Name)
		}
		seen[a.Name] = true
		if len(a.Unit) > 255 || !utf8.ValidString(a.Unit) {
			return invalid("attribute %q: unit %q must be at most 255 bytes of UTF-8", a.Name, a.Unit)
		}
		if a.Type.Size() == 0 {
			return invalid("attribute %q: unknown element type %d", a.Name, uint8(a.Type))
		}
		if a.Components < 1 || a.Components > 255 {
			return invalid("attribute %q: %d components; must be 1 to 255", a.Name, a.Components)
		}
		for _, d := range []struct {
			typ ElementType
			len int
		}{
			{Uint8, len(a.Uint8s)}, {Int32, len(a.Int32s)}, {Float32, len(a.Float32s)}, {Float64, len(a.Float64s)},
		} {
			switch {
			case d.typ == a.Type && d.len != n*a.Components:
				return invalid("attribute %q: %d %s elements; %d samples of %d components need %d",
					a.Name, d.len, d.typ, n, a.Components, n*a.Components)
			case d.typ != a.Type && d.len != 0:
				return invalid("attribute %q: has %s data but its type is %s", a.Name, d.typ, a.Type)
			}
		}
	}
	return nil
}

// Read decodes a datafile written by Write, in the format specified in
// docs/datafile.md.
//
// It reads from r up to and including the end of the datafile and no
// further. It makes a few large reads, so r need not be buffered.
//
// Everything in the file is checked: the identifier, the format version,
// each section's checksum, and the consistency of the contents. A world
// that Read returns can be given to Write, and Write then produces the
// bytes that were read, provided no attribute was skipped.
//
// An attribute whose element type this version of the library does not
// know is skipped: the world is returned without it. A section of a kind
// this version does not know is skipped likewise.
//
// Read does not call Config.Validate on the recorded Config.
//
// Errors wrap ErrNotDatafile, ErrUnsupportedVersion, ErrTruncated, or
// ErrCorrupt, or are the error that r returned.
func Read(r io.Reader) (*World, error) {
	le := binary.LittleEndian

	header := make([]byte, 12)
	if err := readFull(r, header, "the header"); err != nil {
		return nil, err
	}
	if string(header[:8]) != fileMagic {
		return nil, fmt.Errorf("%w: the first 8 bytes are %q, want %q", ErrNotDatafile, header[:8], fileMagic)
	}
	if v := le.Uint32(header[8:]); v != FormatVersion {
		return nil, fmt.Errorf("%w: found version %d, can read version %d", ErrUnsupportedVersion, v, FormatVersion)
	}

	corrupt := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrCorrupt, fmt.Sprintf(format, args...))
	}

	world := &World{}
	var haveMeta, haveSamples, haveTris bool
	names := map[string]bool{}
	for index := 0; ; index++ {
		what := fmt.Sprintf("section %d", index)
		head := make([]byte, 12)
		if err := readFull(r, head, what); err != nil {
			return nil, err
		}
		kind := string(head[:4])
		what = fmt.Sprintf("section %d (%q)", index, kind)
		payload, err := readPayload(r, le.Uint64(head[4:]), what)
		if err != nil {
			return nil, err
		}
		sum := make([]byte, 4)
		if err := readFull(r, sum, what); err != nil {
			return nil, err
		}
		if got, want := crc32.Update(crc32.ChecksumIEEE(head), crc32.IEEETable, payload), le.Uint32(sum); got != want {
			return nil, corrupt("%s: checksum is %08x, the file records %08x", what, got, want)
		}

		switch kind {
		case kindMeta:
			if haveMeta {
				return nil, corrupt("%s: more than one META section", what)
			}
			haveMeta = true
			var meta fileMeta
			if err := json.Unmarshal(payload, &meta); err != nil {
				return nil, corrupt("%s: %v", what, err)
			}
			if meta.LibraryVersion == nil || meta.Config == nil {
				return nil, corrupt("%s: library_version or config is missing", what)
			}
			world.Version, world.Config = *meta.LibraryVersion, *meta.Config

		case kindSamples:
			if haveSamples {
				return nil, corrupt("%s: more than one SMPL section", what)
			}
			haveSamples = true
			if len(payload) < 4 {
				return nil, corrupt("%s: payload is %d bytes, too short for a count", what, len(payload))
			}
			n := uint64(le.Uint32(payload))
			if n > MaxSamples || uint64(len(payload)) != 4+24*n {
				return nil, corrupt("%s: payload is %d bytes for %d samples", what, len(payload), n)
			}
			world.Positions = make([][3]float64, n)
			for i, data := 0, payload[4:]; i < len(world.Positions); i, data = i+1, data[24:] {
				world.Positions[i] = [3]float64{
					math.Float64frombits(le.Uint64(data)),
					math.Float64frombits(le.Uint64(data[8:])),
					math.Float64frombits(le.Uint64(data[16:])),
				}
			}

		case kindTris:
			if haveTris {
				return nil, corrupt("%s: more than one TRIS section", what)
			}
			if !haveSamples {
				return nil, corrupt("%s: TRIS comes before SMPL", what)
			}
			haveTris = true
			if len(payload) < 4 {
				return nil, corrupt("%s: payload is %d bytes, too short for a count", what, len(payload))
			}
			n := uint64(le.Uint32(payload))
			if uint64(len(payload)) != 4+12*n {
				return nil, corrupt("%s: payload is %d bytes for %d triangles", what, len(payload), n)
			}
			world.Triangles = make([][3]int32, n)
			for t, data := 0, payload[4:]; t < len(world.Triangles); t, data = t+1, data[12:] {
				for c := range 3 {
					v := le.Uint32(data[4*c:])
					if uint64(v) >= uint64(len(world.Positions)) {
						return nil, corrupt("%s: triangle %d has corner %d; there are %d samples", what, t, v, len(world.Positions))
					}
					world.Triangles[t][c] = int32(v)
				}
			}

		case kindAttr:
			if !haveSamples {
				return nil, corrupt("%s: ATTR comes before SMPL", what)
			}
			a, known, err := decodeAttribute(payload, len(world.Positions))
			if err != nil {
				return nil, corrupt("%s: %v", what, err)
			}
			if names[a.Name] {
				return nil, corrupt("%s: attribute %q appears more than once", what, a.Name)
			}
			names[a.Name] = true
			if known {
				world.Attributes = append(world.Attributes, a)
			}

		case kindEnd:
			if len(payload) != 0 {
				return nil, corrupt("%s: payload is %d bytes, want 0", what, len(payload))
			}
			if !haveMeta || !haveSamples || !haveTris {
				return nil, corrupt("the file ends without a META, SMPL, or TRIS section")
			}
			return world, nil

		default:
			// A section added by a later version of the library. Its
			// checksum has been verified; ignore its contents.
		}
	}
}

// decodeAttribute decodes the payload of an ATTR section for a world of
// n samples. known is false, and the attribute has a name but no data,
// when the element type is not one this version knows.
func decodeAttribute(payload []byte, n int) (a Attribute, known bool, err error) {
	short := errors.New("payload is too short for the attribute's header")
	text := func(what string) (string, error) {
		if len(payload) < 1 || len(payload) < 1+int(payload[0]) {
			return "", short
		}
		s := string(payload[1 : 1+payload[0]])
		payload = payload[1+payload[0]:]
		if !utf8.ValidString(s) {
			return "", fmt.Errorf("the %s is not valid UTF-8", what)
		}
		return s, nil
	}
	if a.Name, err = text("name"); err != nil {
		return a, false, err
	}
	if a.Name == "" {
		return a, false, errors.New("the name is empty")
	}
	if a.Unit, err = text("unit"); err != nil {
		return a, false, err
	}
	if len(payload) < 2 {
		return a, false, short
	}
	a.Type, a.Components = ElementType(payload[0]), int(payload[1])
	payload = payload[2:]
	if a.Components == 0 {
		return a, false, fmt.Errorf("attribute %q has 0 components", a.Name)
	}
	if a.Type.Size() == 0 {
		return a, false, nil
	}
	count := n * a.Components
	if len(payload) != count*a.Type.Size() {
		return a, false, fmt.Errorf("attribute %q has %d bytes of data; %d samples of %d %s components need %d",
			a.Name, len(payload), n, a.Components, a.Type, count*a.Type.Size())
	}
	le := binary.LittleEndian
	switch a.Type {
	case Uint8:
		a.Uint8s = append(make([]uint8, 0, count), payload...)
	case Int32:
		a.Int32s = make([]int32, count)
		for i := range a.Int32s {
			a.Int32s[i] = int32(le.Uint32(payload[4*i:]))
		}
	case Float32:
		a.Float32s = make([]float32, count)
		for i := range a.Float32s {
			a.Float32s[i] = math.Float32frombits(le.Uint32(payload[4*i:]))
		}
	case Float64:
		a.Float64s = make([]float64, count)
		for i := range a.Float64s {
			a.Float64s[i] = math.Float64frombits(le.Uint64(payload[8*i:]))
		}
	}
	return a, true, nil
}

// readFull fills buf from r. Running out of input is ErrTruncated.
func readFull(r io.Reader, buf []byte, what string) error {
	if _, err := io.ReadFull(r, buf); err != nil {
		if err == io.EOF || err == io.ErrUnexpectedEOF {
			return fmt.Errorf("%w: the input ends inside %s", ErrTruncated, what)
		}
		return err
	}
	return nil
}

// readPayload reads a payload of the given length. The length comes from
// the file and may be wrong, so memory is allocated only as fast as data
// actually arrives.
func readPayload(r io.Reader, length uint64, what string) ([]byte, error) {
	const step = 1 << 20
	buf := make([]byte, 0, min(length, step))
	for uint64(len(buf)) < length {
		grow := int(min(length-uint64(len(buf)), uint64(max(len(buf), step))))
		at := len(buf)
		buf = append(buf, make([]byte, grow)...)
		if err := readFull(r, buf[at:], what); err != nil {
			return nil, err
		}
	}
	return buf, nil
}
