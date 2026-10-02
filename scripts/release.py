#!/usr/bin/env python3
"""Validate and publish the repository's Go modules as one stable release."""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
from urllib.error import HTTPError
from urllib.parse import quote
from urllib.request import urlopen

ROOT = Path(__file__).resolve().parents[1]


def run(*args, cwd=ROOT, capture=False, env=None):
    return subprocess.run(args, cwd=cwd, check=True, text=True,
                          stdout=subprocess.PIPE if capture else None, env=env).stdout


def modules():
    tracked = run("git", "ls-files", "-z", capture=True).split("\0")
    result = []
    for filename in sorted(tracked):
        if filename == "go.mod" or filename.endswith("/go.mod"):
            directory = str(Path(filename).parent)
            metadata = json.loads(run("go", "mod", "edit", "-json", cwd=ROOT / directory, capture=True))
            result.append((directory, metadata))
    return result


def version(value):
    if not re.fullmatch(r"v(?:0|1)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)", value):
        raise ValueError("use a stable v0.x.y or v1.x.y version, for example v0.1.0")
    return value


def tags(release_version, entries):
    return [release_version if directory == "." else f"{directory}/{release_version}"
            for directory, _ in entries]


def require_unpublished(release_version, entries):
    remote = run("git", "ls-remote", "--tags", "origin", capture=True)
    refs = {line.split()[1] for line in remote.splitlines()}
    for tag in tags(release_version, entries):
        if f"refs/tags/{tag}" in refs:
            raise ValueError(f"{tag} is already published; retry failed downstream jobs or choose a new version")
    for _, metadata in entries:
        module_path = metadata["Module"]["Path"]
        lookup = f"https://sum.golang.org/lookup/{quote(module_path, safe='/')}@{release_version}"
        try:
            with urlopen(lookup, timeout=20):
                raise ValueError(f"{module_path}@{release_version} is permanently recorded in Go's checksum database; choose a new version")
        except HTTPError as error:
            if error.code != 404:
                raise RuntimeError(f"could not check Go publication history for {module_path}: HTTP {error.code}") from error


def prepare(release_version):
    entries = modules()
    require_unpublished(release_version, entries)
    local_paths = {metadata["Module"]["Path"] for _, metadata in entries}
    for directory, metadata in entries:
        print(f"Preparing {metadata['Module']['Path']} for {release_version}", flush=True)
        for requirement in metadata.get("Require") or []:
            if requirement["Path"] in local_paths:
                run("go", "mod", "edit", f"-require={requirement['Path']}@{release_version}", cwd=ROOT / directory)
        run("go", "mod", "tidy", cwd=ROOT / directory)


def test(directory=None):
    entries = modules()
    if directory is not None:
        entries = [entry for entry in entries if entry[0] == directory]
        if not entries:
            raise ValueError(f"unknown module directory: {directory}")
    for module_dir, metadata in entries:
        print(f"::group::Test {metadata['Module']['Path']}", flush=True)
        try:
            run("go", "test", "-ldflags=-checklinkname=0", "./...", cwd=ROOT / module_dir)
            if module_dir == ".":
                run("go", "test", "-tags=debug,pprof", "./...", cwd=ROOT)
            run("go", "mod", "tidy", "-diff", cwd=ROOT / module_dir)
        finally:
            print("::endgroup::", flush=True)


def publish(release_version):
    entries = modules()
    require_unpublished(release_version, entries)
    local_paths = {metadata["Module"]["Path"] for _, metadata in entries}
    for _, metadata in entries:
        for requirement in metadata.get("Require") or []:
            if requirement["Path"] in local_paths and requirement["Version"] != release_version:
                raise ValueError(f"{metadata['Module']['Path']} is not prepared for {release_version}")
    paths = [str(Path(directory) / name) for directory, _ in entries
             for name in ("go.mod", "go.sum") if (ROOT / directory / name).exists()]
    run("git", "add", "--", *paths)
    changed = subprocess.run(["git", "diff", "--cached", "--quiet", "--", *paths], cwd=ROOT).returncode
    if changed == 1:
        run("git", "commit", "--only", "-m", f"chore(release): prepare {release_version} modules", "--", *paths)
    elif changed != 0:
        raise RuntimeError("could not inspect staged module changes")
    release_tags = tags(release_version, entries)
    for tag in release_tags:
        run("git", "tag", "--annotate", tag, "--message", f"Release {tag}")
    run("git", "push", "--atomic", "origin", "HEAD:refs/heads/main",
        *(f"refs/tags/{tag}" for tag in release_tags))
    print(f"Published {len(release_tags)} module tags for {release_version}", flush=True)


def verify(release_version):
    entries = modules()
    with tempfile.TemporaryDirectory(prefix="goutils-release-") as scratch:
        directory = Path(scratch)
        env = dict(os.environ, GOWORK="off", GOPROXY="direct")
        run("go", "mod", "init", "goutils-release-check", cwd=directory, env=env)
        run("go", "mod", "edit", "-go=1.27", cwd=directory, env=env)
        for _, metadata in entries:
            run("go", "mod", "edit", f"-require={metadata['Module']['Path']}@{release_version}", cwd=directory, env=env)
        run("go", "test", "-mod=mod", "-run=^$", "-ldflags=-checklinkname=0",
            *(f"{metadata['Module']['Path']}/..." for _, metadata in entries), cwd=directory, env=env)
        for _, metadata in entries:
            published = json.loads(run("go", "list", "-m", "-json", metadata["Module"]["Path"], cwd=directory, capture=True, env=env))
            if published.get("Replace") or published["Version"] != release_version:
                raise RuntimeError(f"unexpected published module: {published['Path']}")
        print(f"Verified {len(entries)} published modules without local replacements", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    commands.add_parser("modules")
    test_parser = commands.add_parser("test")
    test_parser.add_argument("--module")
    for command in ("prepare", "publish", "verify"):
        commands.add_parser(command).add_argument("version", type=version)
    args = parser.parse_args()
    if args.command == "modules":
        print(json.dumps([directory for directory, _ in modules()]))
    elif args.command == "test":
        test(args.module)
    else:
        globals()[args.command](args.version)


if __name__ == "__main__":
    main()
