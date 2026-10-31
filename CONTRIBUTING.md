# Contributing

Smallness is the point. Prefer the fewest moving parts that solve the problem.
Keep changes focused and write one test for each behavior worth protecting.

Use Go 1.26 or newer and Python 3.9 or newer (standard library only, for the
release tooling):

```sh
./x check                   # fmt, vet, build and race tests
./x test ./internal/patch   # focused package tests
./x build -o drif ./cmd/drif
```

`./x` defaults to `check`, which checks formatting without rewriting sources. CI
and release checks use the same command, including `go test -race ./...`.
`./x --help` lists targets; `./x fmt --check`, `./x lint`, `./x test` and
`./x build` run individual checks. Test and build arguments are forwarded to Go.

Tests are hermetic and offline. `internal/git/real_test.go` builds throwaway
repositories in `t.TempDir()` and parses the output of the real git binary —
that seam is where the bugs are, so keep it exercised. `scripts/demo.sh` builds
a repository of awkward changes for looking at by hand:

```sh
scripts/demo.sh
cd /tmp/drif-demo && drif
```

Add a CHANGELOG entry for user-visible changes, under `## Unreleased`. Keep
`README.md` and `docs/plan.md` true to the code; a stale comment is worse than
no comment.

Report issues with `drif --version`, `drif config` and what you expected. Remove
sensitive values before sharing output.

By contributing, you agree that your contributions are licensed under this
project's MIT license.

## Releases

See [docs/releasing.md](docs/releasing.md) for the one-time environment and
token setup.

`scripts/release.sh vX.Y.Z` is a two-step release: it first opens a PR naming
the Unreleased changelog section, then, once merged, asks Actions to tag and
publish. The release workflow builds the archives, attests them, writes the
notes from the changelog and opens a Homebrew formula PR. Automation never
merges the formula PR.

## Formula updates

After publication, the release workflow proposes the latest release's formula
in a release PR against main. Review and merge after required checks pass. An
unchanged formula creates no PR. Retries reuse only an open PR from the same
author containing one formula-only automation commit with identical content and
file mode; human edits and closed PRs require manual review and are never
overwritten.
