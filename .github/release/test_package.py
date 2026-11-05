"""Release verification must stop before publication when the native version is wrong."""

import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

import package


class PackageTests(unittest.TestCase):
    def test_wrong_native_version_never_produces_a_manifest(self):
        real_output = subprocess.check_output
        real_run = subprocess.run

        def output(args, **kwargs):
            if args[:2] == ["go", "env"]:
                return "linux\namd64\n"
            if args[:2] == ["go", "version"]:
                settings = {"GOOS": "linux", "GOARCH": "amd64",
                            "CGO_ENABLED": "0", "-trimpath": "true"}
                return json.dumps({"Settings": [{"Key": key, "Value": value}
                                               for key, value in settings.items()]})
            return real_output(args, **kwargs)

        def build(args, **kwargs):
            if args[:2] != ["./x", "build"]:
                return real_run(args, **kwargs)
            binary = Path(args[args.index("-o") + 1])
            binary.write_text('#!/bin/sh\necho "drif v9.9.9"\n')
            binary.chmod(0o755)

        with tempfile.TemporaryDirectory() as temp:
            output_dir = Path(temp)
            with patch.object(package.subprocess, "check_output", side_effect=output), \
                    patch.object(package.subprocess, "run", side_effect=build):
                with self.assertRaisesRegex(ValueError, "unexpected version"):
                    package.package("v1.2.3", output_dir)
            self.assertFalse((output_dir / "checksums.txt").exists())
