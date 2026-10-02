"""Offline behavioral tests for the coordinated Go module release helper."""
import importlib.util
import io
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import unittest
from unittest import mock


SPEC = importlib.util.spec_from_file_location("release", Path(__file__).with_name("release.py"))
release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(release)


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory(prefix="release-tests-")
        self.addCleanup(self.scratch.cleanup)
        self.repo = Path(self.scratch.name) / "repo"
        self.remote = Path(self.scratch.name) / "remote.git"
        self.repo.mkdir()
        # Ignore user/global signing and Git hooks without changing user settings.
        self.env = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull,
                        GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_COUNT="0")
        for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
                    "GIT_CONFIG_PARAMETERS", "GIT_CONFIG", "GIT_TEMPLATE_DIR"):
            self.env.pop(key, None)
        self.start_patch(mock.patch.dict(os.environ, self.env, clear=True))
        self.git("init", "--initial-branch=main")
        self.git("config", "user.name", "Release Test")
        self.git("config", "user.email", "release-test@example.invalid")
        self.git("config", "commit.gpgsign", "false")
        self.git("config", "tag.gpgsign", "false")
        self.git("config", "core.hooksPath", str(Path(self.scratch.name) / "no-hooks"))
        self.write("go.mod", "module example.test/root\n\ngo 1.27.0\n\nrequire (\n"
                   "\texample.test/root/nested/deep v0.0.1\n"
                   "\texample.test/external v1.2.3\n)\n")
        self.write("nested/deep/go.mod", "module example.test/root/nested/deep\n\ngo 1.27.0\n")
        self.write("unused/go.mod", "module example.test/root/unused\n\ngo 1.27.0\n")
        self.write("go.sum", "existing checksum fixture\n")
        self.write("README.md", "original\n")
        self.git("add", ".")
        self.git("commit", "-m", "fixture")
        subprocess.run(["git", "init", "--bare", str(self.remote)], env=self.env,
                       check=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.git("remote", "add", "origin", str(self.remote))
        self.git("push", "origin", "main")
        self.original_run = release.run
        self.calls = []
        self.list_override = {}
        self.start_patch(mock.patch.object(release, "ROOT", self.repo))
        self.start_patch(mock.patch.object(release, "run", side_effect=self.mock_run))
        self.start_patch(mock.patch("sys.stdout", new=io.StringIO()))

    def start_patch(self, patcher):
        self.addCleanup(patcher.stop)
        return patcher.start()

    def write(self, filename, content):
        path = self.repo / filename
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)

    def git(self, *args, cwd=None):
        return subprocess.run(["git", *args], cwd=cwd or self.repo, env=self.env,
                              check=True, text=True, stdout=subprocess.PIPE,
                              stderr=subprocess.PIPE).stdout.strip()

    def metadata(self, directory):
        text = (directory / "go.mod").read_text()
        return {"Module": {"Path": re.search(r"^module (\S+)", text, re.M)[1]},
                "Require": [{"Path": path, "Version": version}
                            for path, version in re.findall(
                                r"^\s*(?:require )?(\S+) (v\S+)", text, re.M)]}

    def mock_run(self, *args, cwd=None, capture=False, env=None):
        directory = Path(cwd) if cwd is not None else self.repo
        self.calls.append((args, directory, env))
        if args[0] != "go":
            return self.original_run(*args, cwd=directory, capture=capture, env=env)
        if args[1:3] == ("mod", "init"):
            (directory / "go.mod").write_text(f"module {args[3]}\n")
        elif args[1:3] == ("mod", "edit"):
            if args[3] == "-json":
                return json.dumps(self.metadata(directory))
            if args[3].startswith("-require="):
                path, version = args[3].removeprefix("-require=").split("@")
                manifest = directory / "go.mod"
                text = manifest.read_text()
                pattern = rf"(?m)^(\s*(?:require )?{re.escape(path)} )v\S+"
                text, count = re.subn(pattern, lambda match: match[1] + version, text)
                if not count:
                    text += f"require {path} {version}\n"
                manifest.write_text(text)
        elif args[1:4] == ("list", "-m", "-json"):
            path = args[4]
            requirement = next(item for item in self.metadata(directory)["Require"]
                               if item["Path"] == path)
            return json.dumps(self.list_override.get(path, requirement))
        elif args[1:3] != ("mod", "tidy") and args[1] != "test":
            raise AssertionError(f"unexpected Go command: {args}")
        return "" if capture else None

    def prepare(self):
        release.prepare("v0.1.0")

    def remote_refs(self):
        return self.git("for-each-ref", "--format=%(refname) %(objectname)", cwd=self.remote)

    def test_discovery_ignores_untracked_and_ignored_modules(self):
        self.write("untracked/go.mod", "module example.test/untracked\n")
        self.write("ignored/go.mod", "module example.test/ignored\n")
        self.write(".gitignore", "ignored/\n")
        self.assertEqual([directory for directory, _ in release.modules()],
                         [".", "nested/deep", "unused"])
        inspected = [directory for args, directory, _ in self.calls if args[:3] == ("go", "mod", "edit")]
        self.assertEqual(set(inspected), {self.repo, self.repo / "nested/deep", self.repo / "unused"})

    def test_stable_versions_and_tag_names(self):
        for value in ("v0.0.0", "v0.1.0", "v1.23.456"):
            with self.subTest(value=value):
                self.assertEqual(release.version(value), value)
        for value in ("0.1.0", "v2.0.0", "v01.1.0", "v1.01.0", "v1.0.01",
                      "v0.1.0-rc.1", "v0.1.0+build", "v0.1", "v0.1.0\n", ""):
            with self.subTest(value=value), self.assertRaises(ValueError):
                release.version(value)
        self.assertEqual(release.tags("v0.1.0", release.modules()),
                         ["v0.1.0", "nested/deep/v0.1.0", "unused/v0.1.0"])

    def test_prepare_updates_only_existing_internal_requirements(self):
        before = {directory: metadata for directory, metadata in release.modules()}
        self.prepare()
        after = {directory: metadata for directory, metadata in release.modules()}
        self.assertEqual(after["."]["Require"], [
            {"Path": "example.test/root/nested/deep", "Version": "v0.1.0"},
            {"Path": "example.test/external", "Version": "v1.2.3"}])
        self.assertEqual(after["nested/deep"], before["nested/deep"])
        self.assertEqual(after["unused"], before["unused"])
        edits = [args[3] for args, _, _ in self.calls
                 if args[:3] == ("go", "mod", "edit") and args[3] != "-json"]
        self.assertEqual(edits, ["-require=example.test/root/nested/deep@v0.1.0"])

    def test_published_tag_collision_blocks_prepare_and_publish_without_changes(self):
        for tag in ("v0.1.0", "nested/deep/v0.1.0"):
            with self.subTest(tag=tag):
                self.git("tag", "--annotate", tag, "--message", "published")
                self.git("push", "origin", f"refs/tags/{tag}")
                before = self.remote_refs()
                for action in (release.prepare, release.publish):
                    with self.assertRaisesRegex(ValueError, "already published"):
                        action("v0.1.0")
                self.assertEqual(self.git("status", "--porcelain"), "")
                self.assertEqual(self.remote_refs(), before)
                self.git("push", "origin", f":refs/tags/{tag}")
                self.git("tag", "--delete", tag)

    def test_publish_rejects_mismatched_dependency_before_git_mutations(self):
        before = self.remote_refs()
        with self.assertRaisesRegex(ValueError, "not prepared"):
            release.publish("v0.1.0")
        self.assertEqual(self.remote_refs(), before)
        self.assertEqual(self.git("tag", "--list"), "")
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_publish_commits_manifests_and_sums_only_and_pushes_main_and_tags(self):
        self.prepare()
        self.write("go.sum", "updated checksum fixture\n")
        self.write("README.md", "unrelated local edit\n")
        release.publish("v0.1.0")
        self.assertEqual(set(self.git("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").splitlines()),
                         {"go.mod", "go.sum"})
        self.assertEqual(self.git("status", "--porcelain"), "M README.md")
        head = self.git("rev-parse", "HEAD")
        self.assertEqual(self.git("rev-parse", "main", cwd=self.remote), head)
        for tag in ("v0.1.0", "nested/deep/v0.1.0", "unused/v0.1.0"):
            self.assertEqual(self.git("rev-parse", f"{tag}^{{}}", cwd=self.remote), head)
            self.assertEqual(self.git("cat-file", "-t", tag, cwd=self.remote), "tag")
        pushes = [args for args, _, _ in self.calls if args[:2] == ("git", "push")]
        self.assertEqual(pushes, [("git", "push", "--atomic", "origin", "HEAD:refs/heads/main",
                                  "refs/tags/v0.1.0", "refs/tags/nested/deep/v0.1.0",
                                  "refs/tags/unused/v0.1.0")])

    def test_publish_does_not_commit_preexisting_unrelated_staged_edits(self):
        self.prepare()
        self.write("README.md", "unrelated staged edit\n")
        self.git("add", "README.md")
        before = self.remote_refs()
        try:
            release.publish("v0.1.0")
        except (ValueError, RuntimeError, subprocess.CalledProcessError):
            self.assertEqual(self.remote_refs(), before)
        else:
            committed = self.git("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").splitlines()
            self.assertNotIn("README.md", committed)
            self.assertIn("README.md", self.git("diff", "--cached", "--name-only").splitlines())

    def test_rejected_tag_causes_atomic_push_to_leave_remote_unchanged(self):
        self.prepare()
        hook = self.remote / "hooks" / "update"
        hook.write_text('#!/bin/sh\ncase "$1" in refs/tags/nested/deep/*) exit 1;; esac\nexit 0\n')
        hook.chmod(0o755)
        before = self.remote_refs()
        with self.assertRaises(subprocess.CalledProcessError):
            release.publish("v0.1.0")
        self.assertEqual(self.remote_refs(), before)

    def test_verify_uses_isolated_consumer_and_exact_requested_versions(self):
        original = (self.repo / "go.mod").read_text()
        release.verify("v0.1.0")
        consumer_calls = [(args, directory, env) for args, directory, env in self.calls
                          if directory != self.repo and directory not in (self.repo / "nested/deep", self.repo / "unused")]
        directories = {directory for _, directory, _ in consumer_calls}
        self.assertEqual(len(directories), 1)
        directory = directories.pop()
        self.assertFalse(directory.is_relative_to(self.repo))
        self.assertFalse(directory.exists(), "temporary consumer must be removed")
        for _, _, env in consumer_calls:
            self.assertEqual(env["GOWORK"], "off")
            self.assertEqual(env["GOPROXY"], "direct")
        commands = [args for args, _, _ in consumer_calls]
        self.assertIn(("go", "mod", "init", "goutils-release-check"), commands)
        for path in ("example.test/root", "example.test/root/nested/deep", "example.test/root/unused"):
            self.assertIn(("go", "mod", "edit", f"-require={path}@v0.1.0"), commands)
            self.assertIn(("go", "list", "-m", "-json", path), commands)
        self.assertFalse(any("replace" in argument for args in commands for argument in args))
        self.assertIn(("go", "test", "-mod=mod", "-run=^$", "-ldflags=-checklinkname=0",
                       "example.test/root/...", "example.test/root/nested/deep/...",
                       "example.test/root/unused/..."), commands)
        self.assertEqual((self.repo / "go.mod").read_text(), original)
        self.assertEqual(self.git("status", "--porcelain"), "")

    def test_verify_rejects_replacement_or_wrong_resolved_version_and_cleans_up(self):
        for resolved in ({"Path": "example.test/root", "Version": "v0.0.1"},
                         {"Path": "example.test/root", "Version": "v0.1.0",
                          "Replace": {"Path": str(self.repo)}}):
            with self.subTest(resolved=resolved):
                self.calls.clear()
                self.list_override = {"example.test/root": resolved}
                with self.assertRaisesRegex(RuntimeError, "unexpected published module"):
                    release.verify("v0.1.0")
                consumers = [directory for args, directory, _ in self.calls
                             if args[:3] == ("go", "mod", "init")]
                self.assertEqual(len(consumers), 1)
                self.assertFalse(consumers[0].exists())

    def test_verify_propagates_consumer_compile_failure_and_cleans_up(self):
        def failing_run(*args, **kwargs):
            if args[:2] == ("go", "test"):
                raise subprocess.CalledProcessError(1, args)
            return self.mock_run(*args, **kwargs)

        with mock.patch.object(release, "run", side_effect=failing_run):
            with self.assertRaises(subprocess.CalledProcessError):
                release.verify("v0.1.0")
        consumers = [directory for args, directory, _ in self.calls
                     if args[:3] == ("go", "mod", "init")]
        self.assertEqual(len(consumers), 1)
        self.assertFalse(consumers[0].exists())


if __name__ == "__main__":
    unittest.main()
