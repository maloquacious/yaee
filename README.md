# yaee

`yaee` is a Go library that generates worlds by simulating plate tectonics on a sphere.

It is a world generator, not a visualizer.
Its output is a datafile that other applications read to build their own world maps.

## Status

Early alpha. **It does not generate a world yet.**
The repository holds the project scaffolding and the design; the generator, the command, and the datafile format are still to be written.
Until the first tagged release, the API and the datafile format may change without notice.

## What it will do

- Generate an Earth-like world from a seed and a configuration, with the same seed and configuration always producing the same datafile.
- Target a scale of about 25 km per data point, which is roughly one million sample points on an Earth-sized sphere.
- Offer a stable Go API so other applications can call the generator directly.
- Provide a command, `cmd/yaee`, for developing and testing the generator, which also renders images so a person can check the look of a world.

Tectonics is the first stage. Erosion is planned next, and biomes after that.

## The model

The tectonic model follows:

> Yann Cortial, Adrien Peytavie, Eric Galin, Eric Guérin.
> *Procedural Tectonic Planets.*
> Computer Graphics Forum, 2019, 38 (2). [doi:10.1111/cgf.13614](https://doi.org/10.1111/cgf.13614). [hal-02136820](https://hal.science/hal-02136820v1)

[docs/research/papers.md](docs/research/papers.md) lists this and the other reference papers, with download links.
The PDFs are not kept in the repository.

## Intended use

This is the planned shape of the API. None of it is implemented yet.

```go
cfg := yaee.DefaultConfig() // Earth-like, about 25 km per data point
cfg.Seed = 42
world, err := yaee.Generate(ctx, cfg)
```

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
