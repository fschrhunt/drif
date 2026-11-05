#!/bin/sh
# Repository checks shared by contributors and CI; check never rewrites sources.
set -eu
cd "$(dirname "$0")"

# Print supported targets and their argument contracts.
usage() {
    cat <<'EOF'
usage: ./x [command] [args...]
  check              Default; Go formatting, vet, build and race tests
  fmt [--check]      Format Go sources, or check without writing
  lint               Run go vet
  test [args...]     Forward arguments to go test (default: ./...)
  build [args...]    Forward arguments to go build (default: ./...)
  review [args...]   Build this checkout and review its own changes with it
  demo [--edges]     Build the demo repository and review it
  release-check [vX.Y.Z] Build and verify four temporary release archives
  dist vX.Y.Z       Build and verify release archives in dist/; never publish
  help | --help | -h Show this help
EOF
}

# Reject invalid arguments before running any command that could write files.
invalid() { usage >&2; exit 2; }

command=${1-check}
if [ "$#" -gt 0 ]; then shift; fi

case "$command" in
    check|lint) [ "$#" -eq 0 ] || invalid ;;
    fmt)
        [ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && [ "$1" = "--check" ]; } || invalid
        ;;
    demo)
        [ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && [ "$1" = "--edges" ]; } || invalid
        ;;
    review) ;;
    release-check) [ "$#" -le 1 ] || invalid ;;
    dist) [ "$#" -eq 1 ] || invalid ;;
    help|--help|-h) [ "$#" -eq 0 ] || invalid; usage; exit 0 ;;
esac

# build_local puts the binary under dist/ so review and demo always run the
# checkout in front of you, not whatever is on PATH.
build_local() {
    mkdir -p dist
    ./x build -o dist/drif ./cmd/drif
}

case "$command" in
    check)
        ./x fmt --check
        ./x lint
        ./x build
        ./x test -race ./...
        ;;
    fmt)
        if [ "${1:-}" = "--check" ]; then
            unformatted=$(gofmt -l .)
            if [ -n "$unformatted" ]; then
                printf 'format these with: gofmt -w %s\n' "$unformatted" >&2
                exit 1
            fi
        else
            gofmt -l -w .
        fi
        ;;
    lint) go vet ./... ;;
    test)
        if [ "$#" -eq 0 ]; then set -- ./...; fi
        go test "$@"
        ;;
    build)
        if [ "$#" -eq 0 ]; then set -- ./...; fi
        go build "$@"
        ;;
    review)
        # The point is to read the change with the changed program: build this
        # tree, then run it on this tree.
        build_local
        exec ./dist/drif "$@"
        ;;
    demo)
        root=$(pwd)
        build_local
        ./scripts/demo.sh "$@" >/dev/null
        cd "${TMPDIR:-/tmp}/drif-demo"
        exec "$root/dist/drif"
        ;;
    release-check)
        python3 -m unittest discover -s .github/release -p 'test_*.py'
        exec python3 .github/release/package.py check "$@"
        ;;
    dist) exec python3 .github/release/package.py dist "$1" ;;
    *) invalid ;;
esac
