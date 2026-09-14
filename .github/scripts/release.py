#!/usr/bin/env python3
"""SemVer release metadata and changelog generation for MaaCtl.

The release workflow calls this script to validate the pushed tag against
Semantic Versioning 2.0.0, decide whether the release is stable or a
pre-release, and build the changelog between the pushed version and the
previous stable (non pre-release) tag.

Usage:
    python release.py metadata --tag v1.2.0-beta.1
    python release.py notes --tag v1.2.0-beta.1 --previous v1.1.0 --repo MaaXYZ/MaaCtl
"""

from __future__ import annotations

import argparse
import re
import subprocess
import sys

# Semantic Versioning 2.0.0 with an optional leading "v", as used by git tags.
SEMVER = re.compile(
    r"^v(?P<major>0|[1-9]\d*)\.(?P<minor>0|[1-9]\d*)\.(?P<patch>0|[1-9]\d*)"
    r"(?:-(?P<prerelease>(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)"
    r"(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?"
    r"(?:\+(?P<build>[0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$"
)

GROUP_FEATURES = "✨ 新功能"
GROUP_FIXES = "🐛 修复"
GROUP_PERFORMANCE = "⚡ 性能优化"
GROUP_REFACTOR = "🔨 重构"
GROUP_DOCS = "📝 文档"
GROUP_OTHER = "📦 其他变更"

GROUP_ORDER = [
    GROUP_FEATURES,
    GROUP_FIXES,
    GROUP_PERFORMANCE,
    GROUP_REFACTOR,
    GROUP_DOCS,
    GROUP_OTHER,
]

TYPE_GROUPS = {
    "feat": GROUP_FEATURES,
    "fix": GROUP_FIXES,
    "perf": GROUP_PERFORMANCE,
    "refactor": GROUP_REFACTOR,
    "docs": GROUP_DOCS,
    "test": GROUP_OTHER,
    "chore": GROUP_OTHER,
    "build": GROUP_OTHER,
    "ci": GROUP_OTHER,
    "style": GROUP_OTHER,
}

CONVENTIONAL = re.compile(
    r"^(?P<type>[a-zA-Z]+)(?:\((?P<scope>[^)]*)\))?(?P<breaking>!)?:\s*(?P<description>.+)$"
)


def run_git(*args: str) -> str:
    result = subprocess.run(
        ["git", *args],
        check=True,
        capture_output=True,
        text=True,
        encoding="utf-8",
        errors="replace",
    )
    return result.stdout


def parse_tag(tag: str) -> re.Match:
    match = SEMVER.match(tag)
    if not match:
        sys.exit(
            f"error: tag {tag!r} is not a valid Semantic Versioning tag "
            "(expected vMAJOR.MINOR.PATCH[-PRERELEASE][+BUILD])"
        )
    return match


def prerelease_channel(prerelease: str | None) -> str:
    """Return the prerelease channel: alpha, beta, rc, or prerelease."""
    if not prerelease:
        return "stable"
    first = prerelease.split(".")[0].lower()
    if first.startswith("alpha") or first == "a":
        return "alpha"
    if first.startswith("beta") or first == "b":
        return "beta"
    if first.startswith("rc"):
        return "rc"
    return "prerelease"


def version_key(tag: str) -> tuple[int, int, int] | None:
    match = SEMVER.match(tag)
    if not match:
        return None
    return (int(match["major"]), int(match["minor"]), int(match["patch"]))


def all_tags() -> list[str]:
    output = run_git("tag", "--list", "v*")
    return [line.strip() for line in output.splitlines() if line.strip()]


def previous_stable_tag(tag: str, tags: list[str]) -> str | None:
    """Largest stable tag whose core version is lower than the current one."""
    current = version_key(tag)
    best_tag = None
    best_key = None
    for candidate in tags:
        match = SEMVER.match(candidate)
        if not match or match["prerelease"]:
            continue  # only stable releases participate
        key = version_key(candidate)
        if key is None or key >= current:
            continue
        if best_key is None or key > best_key:
            best_tag, best_key = candidate, key
    return best_tag


def command_metadata(args: argparse.Namespace) -> None:
    parse_tag(args.tag)
    tags = all_tags()
    if args.tag not in tags:
        sys.exit(f"error: tag {args.tag!r} was not found; push the tag before running this workflow")
    match = SEMVER.match(args.tag)
    channel = prerelease_channel(match["prerelease"])
    outputs = {
        "tag": args.tag,
        "version": args.tag[1:],
        "channel": channel,
        "prerelease": "true" if match["prerelease"] else "false",
        "previous_tag": previous_stable_tag(args.tag, tags) or "",
    }
    for key, value in outputs.items():
        print(f"{key}={value}")


def classify(subject: str) -> tuple[str, str]:
    """Map a commit subject to a changelog group and display text."""
    match = CONVENTIONAL.match(subject)
    if not match:
        return GROUP_OTHER, subject
    group = TYPE_GROUPS.get(match["type"].lower(), GROUP_OTHER)
    text = match["description"].strip()
    if match["scope"]:
        text = f"{text} ({match['scope']})"
    if match["breaking"]:
        text = f"**BREAKING**: {text}"
    return group, text


def command_notes(args: argparse.Namespace) -> None:
    match = parse_tag(args.tag)
    if args.previous:
        # Fail early when the previous tag is missing from the checkout.
        try:
            run_git("rev-parse", "--verify", "--quiet", f"{args.previous}^{{commit}}")
        except subprocess.CalledProcessError:
            sys.exit(f"error: previous tag {args.previous!r} was not found")

    revision = f"{args.previous}..{args.tag}" if args.previous else args.tag
    log = run_git("log", "--no-merges", "--pretty=format:%h\t%s", revision)

    groups: dict[str, list[tuple[str, str]]] = {name: [] for name in GROUP_ORDER}
    for line in log.splitlines():
        if not line.strip():
            continue
        short, _, subject = line.partition("\t")
        group, text = classify(subject)
        groups[group].append((short, text))

    lines: list[str] = []
    channel = prerelease_channel(match["prerelease"])
    if channel != "stable":
        lines.append(f"> ⚠️ 这是 {channel} 预发布版本，可能不稳定，请谨慎使用。")
        lines.append("")

    if not any(groups.values()):
        lines.append("本次发布没有收集到代码变更。")
        lines.append("")
    for group in GROUP_ORDER:
        entries = groups[group]
        if not entries:
            continue
        lines.append(f"### {group}")
        lines.append("")
        for short, text in entries:
            lines.append(f"- {text} ([{short}](https://github.com/{args.repo}/commit/{short}))")
        lines.append("")

    if args.previous:
        lines.append(
            f"**完整变更日志**: https://github.com/{args.repo}/compare/{args.previous}...{args.tag}"
        )
    else:
        lines.append(f"**完整变更日志**: https://github.com/{args.repo}/commits/{args.tag}")

    print("\n".join(lines))


def main() -> None:
    # Keep changelog output stable regardless of the host console encoding.
    sys.stdout.reconfigure(encoding="utf-8", errors="replace")
    parser = argparse.ArgumentParser(description=__doc__)
    subparsers = parser.add_subparsers(dest="command", required=True)

    metadata = subparsers.add_parser("metadata", help="validate the tag and print version metadata")
    metadata.add_argument("--tag", required=True, help="tag name, e.g. v1.2.0-beta.1")
    metadata.set_defaults(func=command_metadata)

    notes = subparsers.add_parser("notes", help="print the release notes between previous stable and tag")
    notes.add_argument("--tag", required=True, help="current tag name")
    notes.add_argument("--previous", default="", help="previous stable tag name (may be empty)")
    notes.add_argument("--repo", required=True, help="GitHub repository, e.g. MaaXYZ/MaaCtl")
    notes.set_defaults(func=command_notes)

    args = parser.parse_args()
    args.func(args)


if __name__ == "__main__":
    main()
