#!/usr/bin/env python3
"""Build, verify, and pack the release artifacts of one platform.

One platform produces two executables, shipped together in a single archive:

    maactl-<version>-<platform>.zip
      maactl(.exe)       self-contained: carries MaaFramework inside
      maactl-lite(.exe)  loads MaaFramework from ./maafw/bin or --lib-dir

The self-contained build embeds the payload produced by tools/packmaafw from the
runtime unpacked below maafw/bin, so this script runs after fetch_maafw.py and
the packer, on the platform it builds for.

Usage:
    python build_release.py --platform win-x86_64 --version 0.1.2
"""

from __future__ import annotations

import argparse
import subprocess
import sys
import zipfile
from pathlib import Path

# Executable base names, without the platform's suffix.
BASES = ("maactl", "maactl-lite")

# Windows appends .exe to executables; every other platform does not.
WINDOWS = "win-"


def executable(base: str, platform: str) -> str:
    return base + ".exe" if platform.startswith(WINDOWS) else base


def run(command: list[str]) -> str:
    print("$ " + " ".join(command))
    result = subprocess.run(command, capture_output=True, text=True, encoding="utf-8", errors="replace")
    if result.stdout.strip():
        print(result.stdout.strip())
    if result.returncode != 0:
        if result.stderr.strip():
            print(result.stderr.strip(), file=sys.stderr)
        raise SystemExit(f"error: {' '.join(command)} failed with exit code {result.returncode}")
    return result.stdout.strip()


def build(platform: str, version: str, directory: Path) -> list[Path]:
    """Compile the self-contained and the lite executable."""
    directory.mkdir(parents=True, exist_ok=True)
    flags = f"-s -w -X main.version={version}"
    targets = []
    for base in BASES:
        output = directory / executable(base, platform)
        command = ["go", "build", "-trimpath"]
        if base == "maactl":
            # Only the self-contained build embeds the MaaFramework payload.
            command += ["-tags", "bundled"]
        command += ["-ldflags", flags, "-o", str(output), "./cmd/maactl"]
        run(command)
        targets.append(output)
    return targets


def verify(executables: list[Path], version: str) -> None:
    """Check that both executables run, report the version, and load MaaFramework.

    `--version` proves what a build carries—a build with -tags bundled but
    without a payload would otherwise silently ship an executable that cannot
    load MaaFramework. `selfcheck` then loads the runtime for real: the
    self-contained executable from its embedded payload, the lite one from
    ./maafw/bin, which is where the release download was unpacked.
    """
    for path in executables:
        banner = run([str(path), "--version"])
        if version not in banner:
            raise SystemExit(f"error: {path} reports {banner!r}, expected version {version}")
        expected = "bundled" if path.stem == "maactl" else "local"
        if path.stem == "maactl":
            if "MaaFramework v" not in banner:
                raise SystemExit(f"error: {path} does not carry MaaFramework: {banner!r}")
        elif "MaaFramework v" in banner:
            raise SystemExit(f"error: {path} unexpectedly carries MaaFramework: {banner!r}")

        runtime = run([str(path), "selfcheck"])
        if expected not in runtime:
            raise SystemExit(
                f"error: {path} reports {runtime!r}, expected the {expected} MaaFramework runtime; "
                "run '.github/scripts/fetch_maafw.py --platform <platform>' and 'go run ./tools/packmaafw' first"
            )


def pack(executables: list[Path], platform: str, version: str, directory: Path) -> Path:
    archive = directory / f"maactl-{version}-{platform}.zip"
    with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as zipped:
        for path in executables:
            zipped.write(path, arcname=path.name)
    return archive


def main() -> None:
    # The executables echo localized (non-ASCII) text, and a Windows console
    # defaults to a code page that cannot encode it; printing must not be what
    # fails the release.
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    sys.stderr.reconfigure(encoding="utf-8", errors="replace")

    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--platform", required=True, help="MaaFramework platform id, e.g. win-x86_64")
    parser.add_argument("--version", required=True, help="maactl version, e.g. 0.1.2")
    parser.add_argument("--dir", default="dist", help="directory receiving the executables and the archive (default: dist)")
    args = parser.parse_args()

    # An empty version would silently build a nameless executable and an
    # archive called "maactl--<platform>.zip", so reject it here instead.
    if not args.version.strip():
        raise SystemExit("error: --version is empty; pass the release version, e.g. 0.1.2")

    directory = Path(args.dir)
    built = build(args.platform, args.version, directory)
    verify(built, args.version)
    archive = pack(built, args.platform, args.version, directory)
    print(f"packed {', '.join(path.name for path in built)} into {archive} ({archive.stat().st_size / (1024 * 1024):.1f} MiB)")


if __name__ == "__main__":
    main()
