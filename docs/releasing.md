# Releasing

Releases are cut from `main` by tag. `scripts/release.sh` drives the two steps;
everything after the tag happens in Actions.

## One-time setup

The release workflow needs an environment named `release` and a token of its
own. The default `GITHUB_TOKEN` cannot be used to push the tag, because a tag
pushed with it does not start another workflow.

Create the environment (already done for this repository):

```sh
gh api --method PUT repos/fschrhunt/drif/environments/release
```

Create a **fine-grained personal access token** scoped to `fschrhunt/drif` with:

| Permission | Access | Why |
| --- | --- | --- |
| Contents | read and write | push the tag, push the formula branch |
| Pull requests | read and write | open and update the formula PR |
| Metadata | read | mandatory for every fine-grained token |

Store it as an environment secret. The command prompts, so the value never
enters your shell history:

```sh
gh secret set RELEASE_TOKEN --env release -R fschrhunt/drif
```

Check both exist:

```sh
gh secret list --env release -R fschrhunt/drif
gh api repos/fschrhunt/drif/environments --jq '.environments[].name'
```

## Cutting a release

```sh
scripts/release.sh v0.1.0
```

The first run opens a PR that renames the `## Unreleased` changelog section to
`## v0.1.0 · YYYY-MM-DD`. Merge it, pull `main`, then run the same command
again. The second run checks that CI passed on that exact commit and asks
Actions to create the tag.

The tag starts `.github/workflows/release.yml`:

| Job | What it does |
| --- | --- |
| `check` | the tag is on `main`, the changelog has its section, `./x check` passes |
| `release` | builds linux/darwin × amd64/arm64 with `CGO_ENABLED=0`, writes `checksums.txt`, attests the archives, publishes the release with notes from the changelog |
| `formula` | regenerates `drif.rb` from the release checksums and opens a review PR; automation never merges it |
| `install` | runs `install.sh` against the published release on Linux and macOS and checks `drif --version` |

If a release fails after the tag exists, delete the tag, fix the problem, and
run the command again. Nothing is rewritten in place.

## Branch protection

`.github/rulesets/main.json` is the source of truth. Apply it with:

```sh
gh api --method POST repos/fschrhunt/drif/rulesets \
  --input .github/rulesets/main.json
```

It requires a status check named `Required`, which `.github/workflows/ci.yml`
aggregates from the Linux and macOS jobs. If that job is ever renamed, change the
workflow and the ruleset together.

## Who can release

A release needs **write access to this repository**. Today that is one account;
there is no separate releaser role.

- Pushing a `v*` tag needs write access and starts the release workflow.
- `scripts/release.sh` dispatches the same workflow, which also needs write
  access.
- Nobody can release code that is not on `main`: the tag job checks that the
  tagged commit is an ancestor of `origin/main`, and that `CHANGELOG.md` names
  the version.
- Nobody can release untested code: the same job runs `./x check` on the tag.

What limits the damage if an account or a token is compromised:

| Guard | What it stops |
| --- | --- |
| Branch protection on `main` | unreviewed commits, force-pushes, deleting the branch |
| The `Required` check | merging without Linux and macOS passing |
| Protected release tags | deleting or moving a published version |
| The `release` environment policy | reading `RELEASE_TOKEN` from a workflow on any ref other than `main` or `v*` |
| A fine-grained token | reaching other repositories, or repository settings, with the release token |
| Build provenance | publishing an artifact that does not match the tag |

What is not guarded, stated plainly:

- **A compromised maintainer account is a compromised release.** The ruleset
  lets a repository admin bypass through a pull request, and an admin can edit
  the workflows. Protect the account, not only the repository.
- **There is no second pair of eyes.** `required_approving_review_count` is 0,
  so "reviewed" means the author merged their own PR after CI.
- **Anyone with write access can run any workflow**, including one that asks for
  the `release` environment. Today that is one account. If collaborators are
  added, put required reviewers on the environment and add a `creation` rule to
  the tag ruleset.
