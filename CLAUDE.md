# CLAUDE.md

`yaee` is a Go library that generates worlds by simulating plate tectonics on a sphere.
Process rules (checks, versioning, commits, where scratch files go) are in @WORKFLOW.md.
This file says what we are building; WORKFLOW.md says how we work.
The order in which it gets built, and what "done" means for each step, are in [PLAN.md](PLAN.md).
If the two disagree about what the product is, this file wins.

## What we are building

A world **generator**, not a visualizer.

- **The product is a datafile.** Other applications read it to build their own world maps.
  Its format is a public contract, as much as the Go API is.
  It holds the generator's native samples; it is not resampled to any client's grid.
- **The users are coding agents.** They call the library, run the command, and write datafile readers for programs not written in Go.
- **The library is called directly by other applications**, so the root package has a stable public API.
- **The command line tool is for us**: it exists to develop and test the generator.
- **Images are for the human to check the "look" of a world.**
  They are a verification aid, always derived from the world data, and never an input to anything.
  Do not add rendering features beyond what is needed to judge a generated world.

When a choice trades data quality against picture quality, data wins.

## The model

The basis is *Procedural Tectonic Planets* (Cortial, Peytavie, Galin, Guérin; Computer Graphics Forum 38(2), 2019; hal-02136820).
A copy belongs at `docs/research/2019-Procedural-Tectonic-Planets.pdf`.
`*.pdf` is git-ignored, so a fresh clone lacks it: `docs/research/papers.md` lists every reference paper with its citation and download link.
If you need to retrieve a paper, save it into `docs/research/` under the file name at the end of its link; do not rename it.
PDFs already in that directory may have been renamed by the user, so check for a paper by its year and title before downloading it again.

Read the relevant section of the paper before implementing or changing an algorithm, and cite the section in the code comment.
Where we depart from the paper, say so in the comment and say why.

| Paper | Topic |
|-------|-------|
| §3, Table 1 | Crust parameters per sample: type, thickness, elevation, oceanic age and ridge direction, orogeny age, type and fold direction |
| §4.1 | Subduction, uplift transfer functions, slab pull |
| §4.2 | Continental collision, terranes |
| §4.3 | Oceanic crust generation at ridges |
| §4.4 | Plate rifting |
| §4.5 | Continental erosion, oceanic dampening, sediment accretion |
| §5 | Amplification of the coarse crust into detailed relief |
| §6 | Fibonacci sampling, spherical Delaunay triangulation, periodic resampling, boundary tracking |
| Appendix A, Table 3 | Constants |

The paper's constants (Table 3) are the defaults.
Its units are mixed, so use kilometres and millions of years (My) internally: 1 mm/y is exactly 1 km/My, which makes the paper's rates usable as written.

The paper describes an interactive authoring tool.
We want the unattended path through it: seed and configuration in, world out.

## Where the generator is going

Tectonics is the first stage, not the whole generator.
Erosion comes next, and eventually biomes.
`docs/research/papers.md` also lists two erosion papers (Cordonnier et al. 2016; Tzathas et al. 2024) as references for that work.

**Relief: the generator stops at the coarse crust. §5 is on hold.**
The tectonic simulation yields a coarse crust; the paper's §5 amplifies it into detailed relief.
Decided 2026-11-11: do not implement §5. It adds detail finer than the samples and needs exemplar terrain data; a later erosion stage, or a client, supplies detail.
On hold is not ruled out: do not implement §5 without being asked, and do not remove what it would need.

So keep both paths open:

- Build the generator as stages that each take the world data and add to it, so erosion and biomes can follow tectonics without reworking it.
- Keep everything the simulation computes per sample (Table 1), not just elevation. Later stages and amplification both need crust type, ages, and fold and ridge directions.
- Give the datafile a format version and room for new per-sample data, so a later stage adds to the file instead of breaking its readers.

## Target scale

Clients are expected to build **Earth-like worlds at about 25 km per data point**.
That is the case to design, default, and tune for.

- Earth radius is 6370 km. At 25 km spacing that is roughly **one million sample points**.
  The paper's largest reported run is 500,000 points at 35 km, so we are past what it measured.
- Anything worse than O(n log n) per time step in the number of samples is unusable at this scale.
  The paper's cost is dominated by resampling for oceanic crust generation and by rifting (Table 2).
- The paper's time step is 2 My and its maximum plate speed is 100 km/My, so a plate can move 200 km, eight samples, in one step.
  Do not assume a sample's neighbours are the same from one step to the next.
- Tests do not run at this scale.
  They use the smallest grid that shows the behavior, as WORKFLOW.md requires; full-scale runs are done by hand through the command.

Other radii and resolutions must work, but Earth-like at 25 km is what the defaults produce.

## Public API

The API takes a **config struct**, not functional options.

```go
cfg := yaee.DefaultConfig() // Earth-like, about 25 km per data point
cfg.Seed = 42
world, err := yaee.Generate(ctx, cfg)
```

A struct was chosen because the configuration is data: it has many numeric parameters, the command must load and save it, and the datafile must record exactly what produced it.

Rules for `Config`:

- It is plain data: exported fields, no functions, channels, or interfaces. It must round-trip through its file encoding.
- `DefaultConfig()` returns the Earth-like 25 km configuration. Callers start from it and change fields; they do not build a `Config` literal from nothing.
- The zero value is not a valid configuration. `Validate` rejects bad values with an error naming the field; nothing silently replaces a zero with a default, because zero is a legitimate value for some parameters.
- Every field's doc comment states its unit and its default.
- The datafile embeds the `Config` and the library version that produced it.
- The same `Config`, including its seed, produces the same datafile byte for byte on the same CPU architecture.
  ARM and AMD may produce different output, because they do not round floating point arithmetic identically; that is accepted, so do not write code or tests to make them agree. All randomness comes from the seed; never from the clock, map iteration order, or goroutine scheduling.
  That rules out the package-level functions of `math/rand/v2`, which cannot be seeded: draw from a `*rand.Rand` built from the configured seed.

The public surface is the root package `yaee` only.
Keep it small: everything else goes under `internal/`, and a name is exported from the root only when a client needs it.
Changing an exported name, the datafile format, or the output for a given seed is a breaking change; WORKFLOW.md says how to version one.

The library does not print, read the environment, read flags, or exit.
It returns errors and accepts a `context.Context` so a caller can cancel a long run.
Those other things belong to the command.

## Documentation

Documentation is written for coding agents, not for people.
That covers doc comments, the command's help output, the datafile specification, and everything under `docs/`.

- Assume the reader has only the document, and will act on it without asking a question.
- Be exact: every unit, type, range, default, byte layout, and error. An agent writes a datafile reader in another language from the specification alone.
- Give examples that run and whose output is shown.
- Leave out motivation, history, and tutorial narrative.

The command is used by agents too: it never prompts, it reports errors on standard error with a non-zero exit status, and its help output lists every flag and default.

## Layout

| Path | Purpose |
|------|---------|
| `/` (package `yaee`) | The public API: `Config`, `Generate`, the world data, reading and writing the datafile |
| `cmd/yaee` | The development command: generate a world, write the datafile, render check images |
| `internal/` | Everything else. `internal/cerrs` gives constant sentinel errors; `internal/dotenv` loads `.env` files for the command |
| `docs/datafile.md` | The datafile specification. `testdata/example.yaee` is the example it lists byte by byte; a test keeps the two and the writer in step |
| `docs/research/` | Reference papers. `papers.md` has the citations and links; the PDFs are not committed |
| `tmp/` | All scratch output: binaries, generated worlds, images. Git-ignored, and excluded from `./...` by the `ignore` directive in `go.mod` |

## Conventions

- Go files begin with the copyright line and `SPDX-License-Identifier: AGPL-3.0-or-later`, as the existing files do.
- Use `math/rand/v2`. Never import `math/rand`.
- Declare sentinel errors as `cerrs.Error` constants.
- Keep dependencies few. Clients inherit every dependency of the root package, so image encoding and command-line libraries must not be imported by it.
