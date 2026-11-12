# Project Workflow

Guidance for agents when working in this repository.

This repository is `yaee`: a Go package, with a small command around it, that simulates plate tectonics on a sphere.
What we are building, and its scope, are in [CLAUDE.md](CLAUDE.md). Read it before writing code.

## Priorities

Fix bugs before adding features.
When proposing what to work on next, first check for open bugs (`gh issue list --label bug --state open`).
If any are open, propose working on them instead of new features or documentation.
An explicit request from the user takes precedence; do the requested work.

## Build and test

Prefer `go run` to `go build`: `go run ./cmd/yaee ...`.
Put any binary you do build, and every other scratch artifact, under `tmp/`.
That includes generated worlds and debug PNGs.
Never write build artifacts or output files to the repository root.
`tmp/` is git-ignored, and `go.mod` has an `ignore` directive that keeps `./...` out of it.

The checks, from the repository root:

```sh
go vet ./...
go test ./...
```

Before committing, both must pass if the commit changes Go code, `go.mod`, or `go.sum`.
Documentation-only changes need no checks.

## Verification

The model is verified in Go, by tests.

- Every algorithm lands with the tests that define what correct means for it: an analytic case, an invariant, or a conservation budget.
- A test that needs a grid uses the smallest detail that shows the behavior, so `go test ./...` stays fast.
  Long runs belong behind `testing.Short`.
- A run must be repeatable: the same seed and flags give the same file, byte for byte, on the same CPU architecture.
  Any change that alters the output for a given seed says so in its commit message.

## Versioning

- Semantic versioning.
  The current version lives in `version.go` as a `semver.Version` literal (`Major`, `Minor`, `Patch`, `PreRelease`), from `github.com/maloquacious/semver`.
- **Every commit that changes Go code (including `go.mod` and `go.sum`) must bump the version in `version.go`.**
  Documentation-only commits are encouraged to bump it but are not required to.
- Choose the bump by the change: patch for fixes, minor for new features, major for any breaking change.
  A change to the root package's exported names, to the output file's format, or to the output for a given seed is a breaking change.
  Adding to the datafile what `docs/datafile.md` says a later version may add without changing the format version (an attribute, a section, a `META` member) is a minor change, provided every value already written for a given seed stays the same.
  Adding an exported name or a `Config` field is a minor change.
  Go requires a new module path (`github.com/maloquacious/yaee/v2`) for major version 2 and above, so ask the user before making a breaking change.
- Go takes a module's version from its git tags and never reads `version.go`, so the two must be kept in step by hand.

### Tags

The library left alpha at v1.0.0. `version.go` has no `PreRelease`.

- Tag every commit that bumps the version.
  The tag is `v` followed by the version in `version.go` at that commit, such as `v1.0.0`; check that they match before pushing the tag.
- Never move or delete a pushed tag. The Go proxy keeps the first copy it sees, so a bad tag is fixed with a new version.

## Git workflow

- Commit directly to `main`.
  No feature branches or pull requests; history is the recovery mechanism.
- Agents are authorized to commit and push each task to `origin` as it is completed, provided all checks pass and Go code changes include a version bump.
- Agents are authorized to create GitHub labels as needed.

## CI

Do not create GitHub Actions workflows or other GitHub CI actions.
Run the checks above locally instead.
