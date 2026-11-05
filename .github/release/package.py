"""Build and verify release archives locally, without credentials or publication."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tarfile
import tempfile

ROOT = Path(__file__).resolve().parents[2]
TARGETS = (("linux", "amd64"), ("linux", "arm64"),
           ("darwin", "amd64"), ("darwin", "arm64"))


def package(tag, output):
    """Check each archive's contents and build settings, and run the native binary."""
    names = [f"drif_{tag}_{goos}_{goarch}.tar.gz" for goos, goarch in TARGETS]
    output.mkdir(parents=True, exist_ok=True)
    if any((output / name).exists() for name in (*names, "checksums.txt")):
        raise ValueError("release files already exist; remove them or use release-check")
    host = tuple(subprocess.check_output(
        ["go", "env", "GOHOSTOS", "GOHOSTARCH"], cwd=ROOT, text=True).split())
    checksums = []
    with tempfile.TemporaryDirectory(prefix="drif-package-") as temp:
        binary = Path(temp) / "drif"
        for (goos, goarch), name in zip(TARGETS, names):
            env = dict(os.environ, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0")
            subprocess.run(["./x", "build", "-trimpath", "-ldflags",
                            f"-s -w -X main.version={tag}", "-o", str(binary),
                            "./cmd/drif"], cwd=ROOT, env=env, check=True)
            info = json.loads(subprocess.check_output(
                ["go", "version", "-m", "-json", str(binary)], cwd=ROOT, text=True))
            settings = {entry["Key"]: entry["Value"] for entry in info["Settings"]}
            for key, expected in {"GOOS": goos, "GOARCH": goarch,
                                  "CGO_ENABLED": "0", "-trimpath": "true"}.items():
                if settings.get(key) != expected:
                    raise ValueError(f"{name}: {key} must be {expected}")
            archive = output / name
            with tarfile.open(archive, "w:gz") as bundle:
                bundle.add(binary, arcname="drif")
                bundle.add(ROOT / "LICENSE", arcname="LICENSE")
            with tarfile.open(archive) as bundle:
                if sorted(bundle.getnames()) != ["LICENSE", "drif"]:
                    raise ValueError(f"{name}: unexpected archive contents")
                for member, source in (("drif", binary), ("LICENSE", ROOT / "LICENSE")):
                    entry = bundle.getmember(member)
                    if not entry.isfile() or bundle.extractfile(entry).read() != source.read_bytes():
                        raise ValueError(f"{name}: {member} content mismatch")
                if not bundle.getmember("drif").mode & 0o111:
                    raise ValueError(f"{name}: drif must be executable")
                if (goos, goarch) == host:
                    # Execute bytes read back from the archive that will be published.
                    native = Path(temp) / "native-drif"
                    native.write_bytes(bundle.extractfile("drif").read())
                    native.chmod(0o700)
                    actual = subprocess.check_output([str(native), "--version"], text=True).strip()
                    if actual != f"drif {tag}":
                        raise ValueError(f"{name}: unexpected version {actual!r}")
                    subprocess.run([str(native), "--help"], stdout=subprocess.DEVNULL, check=True)
            checksums.append(f"{hashlib.sha256(archive.read_bytes()).hexdigest()}  {name}\n")
            print(f"Built and verified {name}", flush=True)
    manifest = output / "checksums.txt"
    manifest.write_text("".join(checksums))
    for line in manifest.read_text().splitlines():
        digest, name = line.split("  ", 1)
        if hashlib.sha256((output / name).read_bytes()).hexdigest() != digest:
            raise ValueError(f"{name}: checksum mismatch")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("check", "dist"))
    parser.add_argument("tag", nargs="?")
    args = parser.parse_args()
    if args.command == "dist" and not args.tag:
        parser.error("dist requires vX.Y.Z")
    tag = args.tag or "v0.0.0"
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", tag):
        parser.error("expected vX.Y.Z")
    if args.command == "check":
        with tempfile.TemporaryDirectory(prefix="drif-release-check-") as temp:
            package(tag, Path(temp))
        print("Four-target release check passed; temporary archives removed.")
    else:
        package(tag, ROOT / "dist")


if __name__ == "__main__":
    try:
        main()
    except ValueError as error:
        raise SystemExit(str(error)) from error
