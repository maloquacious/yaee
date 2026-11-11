# yaee

`yaee` is a Go library that generates worlds by simulating plate tectonics on a sphere.

It is a world generator, not a visualizer.
Its output is a datafile that other applications read to build their own world maps.

## Status

Early alpha. **It does not simulate tectonics yet.**
The sphere, the public API, the datafile format, and the command work at full scale.
A generated world has plates and crust, continents and oceans, and its plates drift for the configured time.
Nothing happens yet where plates meet or part: no ridge, trench, or mountain is formed.
[PLAN.md](PLAN.md) lists what is built and what comes next.
Until the first tagged release, the API and the datafile format may change without notice.

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

From the command line:

```sh
go run ./cmd/yaee generate -seed 42 -out tmp/world.yaee
go run ./cmd/yaee render -in tmp/world.yaee -out tmp/world.png
go run ./cmd/yaee render -in tmp/world.yaee -kind plates -out tmp/plates.png
go run ./cmd/yaee render -in tmp/world.yaee -kind crust -out tmp/crust.png
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
