#!/usr/bin/env python3
"""Download and unpack a MaaFramework release into the maafw/ directory.

The packer (tools/packmaafw) never touches the network: it packs whatever
runtime is unpacked below maafw/bin. CI therefore downloads the release of the
platform it is building for here, before packing.

Usage:
    python fetch_maafw.py --platform win-x86_64
    python fetch_maafw.py --platform linux-aarch64 --version v5.13.0 --dest maafw
"""

from __future__ import annotations

import argparse
import io
import json
import os
import shutil
import sys
import time
import urllib.error
import urllib.request
import zipfile
from pathlib import Path

DEFAULT_REPO = "MaaXYZ/MaaFramework"
DEFAULT_DEST = "maafw"
DEFAULT_VERSION_FILE = "maafw.version"
USER_AGENT = "maactl-ci"
ATTEMPTS = 3

# One entry per platform maactl is built for: the id in the release asset name,
# and the libraries the unpacked bin/ directory must contain.
PLATFORMS = {
    "win-x86_64": ("MaaFramework.dll", "MaaToolkit.dll"),
    "win-aarch64": ("MaaFramework.dll", "MaaToolkit.dll"),
    "linux-x86_64": ("libMaaFramework.so", "libMaaToolkit.so"),
    "linux-aarch64": ("libMaaFramework.so", "libMaaToolkit.so"),
    "macos-x86_64": ("libMaaFramework.dylib", "libMaaToolkit.dylib"),
    "macos-aarch64": ("libMaaFramework.dylib", "libMaaToolkit.dylib"),
}


def token() -> str:
    for name in ("GH_TOKEN", "GITHUB_TOKEN"):
        value = os.environ.get(name, "").strip()
        if value:
            return value
    return ""


def request(url: str, accept: str, authenticated: bool = True) -> urllib.request.Request:
    headers = {"User-Agent": USER_AGENT, "Accept": accept}
    if authenticated:
        secret = token()
        if secret:
            headers["Authorization"] = f"Bearer {secret}"
        headers["X-GitHub-Api-Version"] = "2022-11-28"
    return urllib.request.Request(url, headers=headers)


def fetch(url: str, accept: str = "application/octet-stream", authenticated: bool = True) -> bytes:
    """Fetch a URL, retrying transient failures.

    Network errors and server errors are usually transient; a client error is
    not, so it fails immediately and lets the caller try another source.
    """
    last: Exception | None = None
    for attempt in range(1, ATTEMPTS + 1):
        try:
            with urllib.request.urlopen(request(url, accept, authenticated), timeout=600) as response:
                return response.read()
        except urllib.error.HTTPError as error:
            last = error
            if error.code < 500 and error.code != 429:
                break
        except (urllib.error.URLError, TimeoutError, OSError) as error:
            last = error
        if attempt < ATTEMPTS:
            print(f"  {last}; retrying ({attempt + 1}/{ATTEMPTS})", file=sys.stderr)
            time.sleep(attempt)
    raise SystemExit(f"error: download {url} failed: {last}")


def asset_name(platform: str, version: str) -> str:
    return f"MAA-{platform}-{version}.zip"


def resolve_version(explicit: str, version_file: Path) -> str:
    version = explicit.strip()
    if not version:
        if not version_file.exists():
            raise SystemExit(f"error: no --version and {version_file} does not exist")
        version = version_file.read_text(encoding="utf-8").strip()
    if not version:
        raise SystemExit(f"error: {version_file} is empty; write the MaaFramework release tag into it, e.g. v5.13.0")
    return version if version.startswith("v") else f"v{version}"


def download_archive(repo: str, platform: str, version: str, name: str) -> tuple[bytes, str]:
    """Download the release archive of one platform.

    The API endpoint is tried first: it resolves the exact asset name and serves
    the bytes itself instead of redirecting to github.com, so a token is never
    forwarded to another host and networks that block github.com still work. The
    conventional release URL is the fallback.
    """
    failures: list[str] = []
    try:
        release = json.loads(
            fetch(f"https://api.github.com/repos/{repo}/releases/tags/{version}", accept="application/vnd.github+json")
        )
        for asset in release.get("assets", []):
            if asset.get("name") == name:
                endpoint = f"https://api.github.com/repos/{repo}/releases/assets/{asset['id']}"
                return fetch(endpoint), "api.github.com"
        failures.append(f"release {version} has no asset {name}")
    except SystemExit as error:
        failures.append(str(error))

    url = f"https://github.com/{repo}/releases/download/{version}/{name}"
    try:
        # No token on the fallback: the request redirects to a different host.
        return fetch(url, authenticated=False), "github.com"
    except SystemExit as error:
        failures.append(str(error))
    raise SystemExit("error: " + "; ".join(failures))


def extract(archive: bytes, dest: Path) -> int:
    """Unpack the archive into dest, which is replaced as a whole.

    Replacing the directory is deliberate: the packer refuses a maafw/ that mixes
    runtimes of several platforms, and a stale download should never be the
    reason a build fails.
    """
    if dest.exists():
        shutil.rmtree(dest)
    dest.mkdir(parents=True)
    written = 0
    try:
        with zipfile.ZipFile(io.BytesIO(archive)) as zipped:
            for entry in zipped.infolist():
                target = safe_target(dest, entry.filename)
                if target is None:
                    continue
                if entry.is_dir():
                    target.mkdir(parents=True, exist_ok=True)
                    continue
                target.parent.mkdir(parents=True, exist_ok=True)
                with zipped.open(entry) as source, open(target, "wb") as sink:
                    shutil.copyfileobj(source, sink)
                written += 1
    except zipfile.BadZipFile as error:
        raise SystemExit(f"error: the downloaded archive is not a zip file: {error}")
    if not (dest / "bin").is_dir():
        raise SystemExit("error: the archive holds no bin/ directory")
    return written


def safe_target(root: Path, name: str) -> Path | None:
    """Resolve an archive entry below root, rejecting absolute and escaping names."""
    relative = Path(name.replace("\\", "/").lstrip("/"))
    if relative == Path("."):
        return None
    target = (root / relative).resolve()
    if not target.is_relative_to(root.resolve()):
        print(f"warning: skipping unsafe archive entry {name}", file=sys.stderr)
        return None
    return target


def verify(dest: Path, platform: str) -> None:
    libraries = PLATFORMS[platform]
    missing = [library for library in libraries if not (dest / "bin" / library).is_file()]
    if missing:
        raise SystemExit(f"error: {dest / 'bin'} is missing {', '.join(missing)} for {platform}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--platform", required=True, choices=sorted(PLATFORMS), help="MaaFramework platform id, e.g. win-x86_64")
    parser.add_argument("--version", default="", help=f"MaaFramework tag, e.g. v5.13.0 (default: read from {DEFAULT_VERSION_FILE})")
    parser.add_argument("--dest", default=DEFAULT_DEST, help=f"directory receiving the release (default: {DEFAULT_DEST})")
    parser.add_argument("--repo", default=DEFAULT_REPO, help=f"repository publishing the release (default: {DEFAULT_REPO})")
    parser.add_argument("--version-file", default=DEFAULT_VERSION_FILE, help=f"file pinning the version (default: {DEFAULT_VERSION_FILE})")
    args = parser.parse_args()

    version = resolve_version(args.version, Path(args.version_file))
    dest = Path(args.dest)
    name = asset_name(args.platform, version)
    print(f"downloading MaaFramework {version} for {args.platform}: {name}")
    archive, source = download_archive(args.repo, args.platform, version, name)
    print(f"  archive: {len(archive) / (1024 * 1024):.1f} MiB from {source}")

    written = extract(archive, dest)
    verify(dest, args.platform)
    print(f"  unpacked {written} files into {dest}")
    print(f"  runtime: {dest / 'bin'}")


if __name__ == "__main__":
    main()
