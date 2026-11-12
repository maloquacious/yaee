# yaee

`yaee` is a Go library that generates worlds by simulating plate tectonics on a sphere.

It is a world generator, not a visualizer.
Its output is a datafile that other applications read to build their own world maps.

## Status

The tectonic simulation is complete, and the public API, the datafile format, and the command work at full scale.
The version is in [version.go](version.go); while it is below 1.0, a minor version may change the API, the datafile format, or the world a seed gives.

A generated world has:

- plates and crust, continents and oceans, after the plates have drifted for the configured time (250 My by default);
- ocean floor that formed at the ridges where plates part, and sinks as it ages;
- trenches where one plate subducts beneath another, and mountains on the overriding plate;
- mountains where two continents have collided and joined;
- continents kept apart by plates that break into smaller plates from time to time;
- land that erodes, and low-frequency noise of up to 0.5 km added to the elevation after the last step.

With the defaults about a quarter of the sphere is land, half of the continental crust is below about 0.7 km, and half of the ocean floor is below about 4.4 km.

What it does not have:

- Relief finer than the sample spacing. The world is the coarse crust; a client adds detail.
- Erosion by water, rivers, climate, or biomes.
- Clean coasts. Continents contain channels of oceanic crust 1 to 6 samples wide, which the generator leaves in place; [docs/datafile.md](docs/datafile.md) gives their extent under `crust_type`.
- The same bytes on every CPU architecture. A configuration gives the same datafile on machines of one architecture; ARM and AMD may differ.

[PLAN.md](PLAN.md) lists what is built and what comes next.

## What it does, and will do

- Generates an Earth-like world from a seed and a configuration. The same configuration always produces the same datafile on the same CPU architecture.
- Targets a scale of about 25 km per data point: 942,059 samples on an Earth-sized sphere.
- Offers a Go API so other applications can call the generator directly.
- Writes a datafile specified in [docs/datafile.md](docs/datafile.md), completely enough to write a reader in another language.
- Provides a command, `cmd/yaee`, for developing and testing the generator, which also renders images so a person can check the look of a world.

Tectonics is the first stage. Erosion is planned next, and biomes after that.

## The model

The tectonic model follows:

> Yann Cortial, Adrien Peytavie, Eric Galin, Eric Guérin.
> *Procedural Tectonic Planets.*
> Computer Graphics Forum, 2019, 38 (2). [doi:10.1111/cgf.13614](https://doi.org/10.1111/cgf.13614). [hal-02136820](https://hal.science/hal-02136820v1)

[docs/research/papers.md](docs/research/papers.md) lists this and the other reference papers, with download links.
The PDFs are not kept in the repository.

## Use

From Go:

```go
cfg := yaee.DefaultConfig() // Earth-like, about 25 km per data point
cfg.Seed = 42
world, err := yaee.Generate(ctx, cfg)
if err != nil {
	return err
}
err = yaee.Write(file, world) // yaee.Read(file) reads it back
```

To sample a world at points of your own, such as the cells of a map grid:

```go
loc, err := yaee.NewLocator(world)
if err != nil {
	return err
}
// (x, y, z) is any direction from the centre of the world; its length is ignored.
triangle, weights := loc.Locate(x, y, z) // the triangle of samples containing it
nearest := loc.Nearest(x, y, z)          // the index of the sample nearest to it
t := world.Triangles[triangle]
e := world.Attribute(yaee.AttrElevation).Float64s // km above sea level, one per sample
elevation := weights[0]*e[t[0]] + weights[1]*e[t[1]] + weights[2]*e[t[2]]
```

From the command line:

```sh
go run ./cmd/yaee generate -seed 42 -out tmp/world.yaee
go run ./cmd/yaee render -in tmp/world.yaee -out tmp/world.png
go run ./cmd/yaee render -in tmp/world.yaee -kind plates -out tmp/plates.png
go run ./cmd/yaee render -in tmp/world.yaee -kind crust -out tmp/crust.png
go run ./cmd/yaee render -in tmp/world.yaee -kind age -out tmp/age.png
go run ./cmd/yaee render -in tmp/world.yaee -kind orogeny -out tmp/orogeny.png
go run ./cmd/yaee stats -in tmp/world.yaee
go run ./cmd/yaee help
```

`yaee help` lists the commands and exit statuses; `yaee help <command>` lists a command's flags and defaults.

## Development

Requires Go 1.27.4 or later.

```sh
go vet ./...
go test ./...
```

[CLAUDE.md](CLAUDE.md) describes what is being built and the design rules.
[WORKFLOW.md](WORKFLOW.md) describes how work is done in this repository: checks, versioning, and commits.

## License

Copyright (c) 2026 Michael D Henderson.

Licensed under the GNU Affero General Public License, version 3 or later.
See [LICENSE](LICENSE).
