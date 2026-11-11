// Copyright (c) 2026 Michael D Henderson.
// SPDX-License-Identifier: AGPL-3.0-or-later

// Command yaee is the development command of the yaee library: it
// generates worlds, writes them as datafiles, and renders images of
// datafiles. Run "yaee help" for its commands, flags, and exit statuses.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image"
	"io"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"time"

	"github.com/maloquacious/yaee"
	"github.com/maloquacious/yaee/internal/render"
)

// Exit statuses.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

const mainHelp = `yaee generates worlds by simulating plate tectonics on a sphere.

Usage:
  yaee <command> [flags]

Commands:
  generate   Generate a world and write it as a datafile.
  render     Read a datafile and write an image of it.
  version    Print the version of the yaee library.
  help       Print this text. "yaee help <command>" prints a command's help.

"yaee <command> -help" also prints the command's help: every flag, its
type, and its default. Flags may be written -flag value, -flag=value,
--flag value, or --flag=value. No command takes arguments other than
flags, and no command reads standard input or prompts.

The datafile format is specified in docs/datafile.md.

Output:
  Standard output carries only what a command is asked to print: help,
  the version, or a Config written to "-". Errors, and the timings of
  -v, go to standard error.

Exit status:
  0  Success.
  1  The command failed: an invalid Config, a file that cannot be read
     or written, a datafile that is not valid, or an interrupt.
  2  Usage error: an unknown command or flag, a flag value of the wrong
     type, a missing required flag, or an unexpected argument.
`

// A command is one sub-command: its flags and what it does.
type command struct {
	name    string
	summary string // printed above the flags in the command's help
	notes   string // printed below the flags
	flags   *flag.FlagSet
	run     func(ctx context.Context, stdout, stderr io.Writer) error
}

// usageError is a mistake in how a command was invoked. It exits with
// status 2; every other error exits with status 1.
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

// commands returns the sub-commands, freshly built so that a flag's
// value never survives from one run to the next.
func commands() []*command {
	return []*command{generateCommand(), renderCommand(), versionCommand()}
}

// run executes the command line and returns the exit status.
func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, "yaee: no command given\n\n", mainHelp)
		return exitUsage
	}
	find := func(name string) *command {
		for _, c := range commands() {
			if c.name == name {
				return c
			}
		}
		return nil
	}

	switch args[0] {
	case "-h", "-help", "--help":
		fmt.Fprint(stdout, mainHelp)
		return exitOK
	case "help":
		switch {
		case len(args) == 1:
			fmt.Fprint(stdout, mainHelp)
			return exitOK
		case len(args) == 2 && find(args[1]) != nil:
			find(args[1]).help(stdout)
			return exitOK
		case len(args) == 2 && args[1] == "help":
			fmt.Fprint(stdout, mainHelp)
			return exitOK
		case len(args) == 2:
			fmt.Fprintf(stderr, "yaee help: unknown command %q\nRun \"yaee help\" for the list of commands.\n", args[1])
			return exitUsage
		default:
			fmt.Fprintf(stderr, "yaee help: unexpected argument %q\n", args[2])
			return exitUsage
		}
	}

	c := find(args[0])
	if c == nil {
		fmt.Fprintf(stderr, "yaee: unknown command %q\nRun \"yaee help\" for the list of commands.\n", args[0])
		return exitUsage
	}
	usage := func(msg string) int {
		fmt.Fprintf(stderr, "yaee %s: %s\nRun \"yaee %s -help\" for its flags.\n", c.name, msg, c.name)
		return exitUsage
	}
	c.flags.SetOutput(io.Discard) // we print our own messages
	c.flags.Usage = func() {}
	if err := c.flags.Parse(args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			c.help(stdout)
			return exitOK
		}
		return usage(err.Error())
	}
	if c.flags.NArg() > 0 {
		return usage(fmt.Sprintf("unexpected argument %q", c.flags.Arg(0)))
	}
	if err := c.run(ctx, stdout, stderr); err != nil {
		if u, ok := errors.AsType[usageError](err); ok {
			return usage(u.msg)
		}
		fmt.Fprintf(stderr, "yaee %s: %v\n", c.name, err)
		return exitFailed
	}
	return exitOK
}

// help prints the command's help: its summary, every flag with its type
// and default, and its notes.
func (c *command) help(w io.Writer) {
	fmt.Fprintf(w, "%s\n\nUsage:\n  yaee %s [flags]\n\nFlags:\n", c.summary, c.name)
	c.flags.VisitAll(func(f *flag.Flag) {
		typ, usage := flag.UnquoteUsage(f)
		if typ == "" {
			typ = "(no value)"
		}
		def := f.DefValue
		if def == "" {
			def = "none"
		}
		fmt.Fprintf(w, "  -%s %s\n      %s\n      Default: %s\n", f.Name, typ, usage, def)
	})
	fmt.Fprintf(w, "\n%s\nRun \"yaee help\" for exit statuses and what goes to standard output.\n", c.notes)
}

// wasSet reports whether the named flag was given on the command line.
func wasSet(fs *flag.FlagSet, name string) bool {
	set := false
	fs.Visit(func(f *flag.Flag) { set = set || f.Name == name })
	return set
}

func generateCommand() *command {
	def := yaee.DefaultConfig()
	fs := flag.NewFlagSet("generate", flag.ContinueOnError)
	var (
		configPath  = fs.String("config", "", "Read the Config from this JSON `file`. Members the file omits keep their defaults; a member it does not recognise is an error.")
		seed        = fs.Uint64("seed", def.Seed, "Seed of all randomness: an `integer` from 0 to 18446744073709551615.")
		radius      = fs.Float64("radius", def.Radius, "Radius of the sphere at sea level, in `km`. Greater than 0.")
		spacing     = fs.Float64("spacing", def.SampleSpacing, "Distance between neighbouring samples, in `km`. Greater than 0.")
		duration    = fs.Float64("duration", def.Duration, "Span of time simulated, in `My` (millions of years). At least 0.")
		timeStep    = fs.Float64("time-step", def.TimeStep, "Length of one simulation step, in `My`. Greater than 0.")
		plates      = fs.Int("plates", def.Plates, fmt.Sprintf("Number of tectonic plates at the start: an `integer` from 1 to %d, and no more than the sample count.", yaee.MaxPlates))
		warpAmp     = fs.Float64("plate-warp-amplitude", def.PlateWarpAmplitude, "How far the initial plate boundaries are bent away from straight, in `km`. At least 0; 0 gives Voronoi cells.")
		warpSize    = fs.Float64("plate-warp-feature-size", def.PlateWarpFeatureSize, "Length of the largest bends in the initial plate boundaries, in `km`. Greater than 0.")
		land        = fs.Float64("land-coverage", def.LandCoverage, "Share of the sphere's area that is continental crust at the start: a `fraction` from 0 to 1.")
		ridge       = fs.Float64("ridge-elevation", def.RidgeElevation, "Highest elevation of oceanic crust, in `km`. Less than 0 and greater than -abyssal-elevation.")
		abyssal     = fs.Float64("abyssal-elevation", def.AbyssalElevation, "Elevation of the abyssal plains, in `km`. Less than -ridge-elevation and greater than -trench-elevation.")
		trench      = fs.Float64("trench-elevation", def.TrenchElevation, "Elevation of the deepest oceanic trench, in `km`. Less than -abyssal-elevation.")
		maxCont     = fs.Float64("max-continental-elevation", def.MaxContinentalElevation, "Highest elevation of continental crust, in `km`. Greater than 0.")
		maxSpeed    = fs.Float64("max-plate-speed", def.MaxPlateSpeed, "Greatest speed of any point of a plate, in `km/My` (equal to mm/y). At least 0; 0 gives plates that do not move.")
		outPath     = fs.String("out", "", "Write the datafile to this `file`, replacing it if it exists.")
		writeConfig = fs.String("write-config", "", "Write the effective Config as JSON to this `file`, or to standard output if the value is \"-\".")
		verbose     = fs.Bool("v", false, "Print the sample count, file size, and timings to standard error.")
	)
	return &command{
		name:    "generate",
		summary: "yaee generate builds a world from a Config and writes it as a datafile.",
		notes: fmt.Sprintf(`At least one of -out and -write-config is required. With -write-config
and no -out, the Config is checked and written and no world is generated.

The effective Config is the default Config, then the members of the
-config file, then each of the flags from -seed to
-max-plate-speed that is given. The defaults shown above are
the default Config: an Earth-like world of 942059 samples. An invalid
Config is reported with the name of each invalid field, and nothing is
written.

The Config file is one JSON object. This is the default Config:
%s
The same Config gives the same datafile, byte for byte, on the same CPU
architecture. The default world takes about 5 seconds to generate on
ten processors, and its datafile is about 126 MB. -duration 0 gives the
initial world, before any plate has moved.

Example:
  yaee generate -seed 42 -out tmp/world.yaee
`, defaultConfigJSON()),
		flags: fs,
		run: func(ctx context.Context, stdout, stderr io.Writer) error {
			if *outPath == "" && *writeConfig == "" {
				return usageError{"at least one of -out and -write-config is required"}
			}
			cfg := yaee.DefaultConfig()
			if *configPath != "" {
				data, err := os.ReadFile(*configPath)
				if err != nil {
					return err
				}
				dec := json.NewDecoder(bytes.NewReader(data))
				dec.DisallowUnknownFields()
				if err := dec.Decode(&cfg); err != nil {
					return fmt.Errorf("%s: %w", *configPath, err)
				}
				if dec.More() {
					return fmt.Errorf("%s: unexpected data after the JSON object", *configPath)
				}
			}
			if wasSet(fs, "seed") {
				cfg.Seed = *seed
			}
			if wasSet(fs, "radius") {
				cfg.Radius = *radius
			}
			if wasSet(fs, "spacing") {
				cfg.SampleSpacing = *spacing
			}
			if wasSet(fs, "duration") {
				cfg.Duration = *duration
			}
			if wasSet(fs, "time-step") {
				cfg.TimeStep = *timeStep
			}
			if wasSet(fs, "plates") {
				cfg.Plates = *plates
			}
			if wasSet(fs, "plate-warp-amplitude") {
				cfg.PlateWarpAmplitude = *warpAmp
			}
			if wasSet(fs, "plate-warp-feature-size") {
				cfg.PlateWarpFeatureSize = *warpSize
			}
			if wasSet(fs, "land-coverage") {
				cfg.LandCoverage = *land
			}
			if wasSet(fs, "ridge-elevation") {
				cfg.RidgeElevation = *ridge
			}
			if wasSet(fs, "abyssal-elevation") {
				cfg.AbyssalElevation = *abyssal
			}
			if wasSet(fs, "trench-elevation") {
				cfg.TrenchElevation = *trench
			}
			if wasSet(fs, "max-continental-elevation") {
				cfg.MaxContinentalElevation = *maxCont
			}
			if wasSet(fs, "max-plate-speed") {
				cfg.MaxPlateSpeed = *maxSpeed
			}
			if err := cfg.Validate(); err != nil {
				return err
			}

			if *writeConfig != "" {
				data, err := json.MarshalIndent(cfg, "", "  ")
				if err != nil {
					return err
				}
				data = append(data, '\n')
				if *writeConfig == "-" {
					if _, err := stdout.Write(data); err != nil {
						return err
					}
				} else if err := os.WriteFile(*writeConfig, data, 0o644); err != nil {
					return err
				}
			}
			if *outPath == "" {
				return nil
			}

			start := time.Now()
			world, err := yaee.Generate(ctx, cfg)
			if err != nil {
				return err
			}
			generated := time.Now()
			size, err := writeFile(*outPath, func(w io.Writer) error { return yaee.Write(w, world) })
			if err != nil {
				return err
			}
			if *verbose {
				fmt.Fprintf(stderr, "%s: %d samples, %d triangles, %d bytes; generated in %.2fs, written in %.2fs\n",
					*outPath, world.NumSamples(), world.NumTriangles(), size,
					generated.Sub(start).Seconds(), time.Since(generated).Seconds())
			}
			return nil
		},
	}
}

func renderCommand() *command {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	var (
		inPath  = fs.String("in", "", "Read the world from the datafile in this `file`. Required.")
		outPath = fs.String("out", "", "Write the PNG image to this `file`, replacing it if it exists. Required.")
		kind    = fs.String("kind", "elevation", "What to draw: `name` is one of "+kindNames()+".")
		width   = fs.Int("width", 2048, fmt.Sprintf("Width of the image in `pixels`: an even number from %d to %d. The height is half the width.", render.MinWidth, render.MaxWidth))
		verbose = fs.Bool("v", false, "Print the image size and timings to standard error.")
	)
	return &command{
		name:    "render",
		summary: "yaee render reads a datafile and writes an image of one kind of its data.",
		notes: fmt.Sprintf(`The image is drawn from the datafile alone. It is an equirectangular
map: north is up, longitude 0 is the vertical line through the middle,
east is to the right, and the left and right edges are longitude 180.
Each pixel has the colour of the sample nearest to its centre.

-kind elevation colours by the "elevation" attribute, with sea level at
0 km:
  below 0   blue, from (150, 200, 235) at sea level to (8, 24, 80) at
            %g km and below
  0 and up  from green (200, 220, 150) at sea level to brown
            (90, 50, 20) at %g km and above
  not a number   magenta (255, 0, 255)

-kind plates colours by the "plate" attribute. Each plate has one
colour; plates close in number have very different hues. A pixel is
black where the pixel to its right or below it is on a different plate,
so black lines trace the plate boundaries.

-kind crust colours by the "crust_type" attribute: oceanic crust is
blue (40, 90, 160) and continental crust is tan (190, 170, 110).

A datafile without the attribute a kind needs is an error.

The same datafile and width give the same PNG, byte for byte.

Example:
  yaee render -in tmp/world.yaee -out tmp/world.png
`, render.DeepestOcean, render.HighestLand),
		flags: fs,
		run: func(ctx context.Context, stdout, stderr io.Writer) error {
			if *inPath == "" || *outPath == "" {
				return usageError{"-in and -out are both required"}
			}
			start := time.Now()
			f, err := os.Open(*inPath)
			if err != nil {
				return err
			}
			world, err := yaee.Read(f)
			f.Close()
			if err != nil {
				return fmt.Errorf("%s: %w", *inPath, err)
			}
			read := time.Now()
			draw, ok := kinds[*kind]
			if !ok {
				return usageError{fmt.Sprintf("unknown -kind %q; it must be one of %s", *kind, kindNames())}
			}
			img, err := draw(world, *width)
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			drawn := time.Now()
			size, err := writeFile(*outPath, func(w io.Writer) error { return render.WritePNG(w, img) })
			if err != nil {
				return err
			}
			if *verbose {
				fmt.Fprintf(stderr, "%s: %dx%d pixels, %d bytes; datafile read in %.2fs, drawn in %.2fs, written in %.2fs\n",
					*outPath, *width, *width/2, size,
					read.Sub(start).Seconds(), drawn.Sub(read).Seconds(), time.Since(drawn).Seconds())
			}
			return nil
		},
	}
}

// defaultConfigJSON returns the default Config as indented JSON, each
// line indented by two more spaces and ended by a newline.
func defaultConfigJSON() string {
	data, err := json.MarshalIndent(yaee.DefaultConfig(), "  ", "  ")
	if err != nil {
		panic(err) // a Config is plain data
	}
	return "  " + string(data) + "\n"
}

// kinds maps each value of render's -kind flag to what draws it.
var kinds = map[string]func(*yaee.World, int) (*image.NRGBA, error){
	"crust":     render.CrustType,
	"elevation": render.Elevation,
	"plates":    render.Plates,
}

// kindNames lists the values of -kind, in alphabetical order.
func kindNames() string {
	return strings.Join(slices.Sorted(maps.Keys(kinds)), ", ")
}

func versionCommand() *command {
	return &command{
		name:    "version",
		summary: "yaee version prints the version of the yaee library on one line, such as 0.11.0-alpha.",
		notes: `The version is what a datafile records as library_version.
`,
		flags: flag.NewFlagSet("version", flag.ContinueOnError),
		run: func(ctx context.Context, stdout, stderr io.Writer) error {
			_, err := fmt.Fprintln(stdout, yaee.Version().String())
			return err
		},
	}
}

// writeFile creates the file, fills it with write, and returns its
// size. If anything fails it removes the file, so that a failed command
// never leaves a partial one.
func writeFile(path string, write func(io.Writer) error) (size int64, err error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err != nil {
			os.Remove(path)
		}
	}()
	counter := &countingWriter{w: f}
	if err := write(counter); err != nil {
		f.Close()
		return 0, fmt.Errorf("%s: %w", path, err)
	}
	if err := f.Close(); err != nil {
		return 0, err
	}
	return counter.n, nil
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
