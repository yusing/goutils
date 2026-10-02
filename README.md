# goutils

Go 1.27 utility packages for strings, buffers, IO, errors, task lifetimes,
object pools, events, and HTTP handling. The root module depends only on xsync
and x/text. Logging is application-owned, and testing helpers use the standard
library.

## Use a Package

```sh
go get github.com/yusing/goutils@v0.8.0
```

Import the package you need, such as `github.com/yusing/goutils/strings` or
`github.com/yusing/goutils/task`. There is no package at the module root.
See each package's README for its API and behavior.

HTTP and other dependency-heavy packages are separate modules with unchanged
package import paths. Add their module when needed:

```sh
go get github.com/yusing/goutils/http@v0.8.0
```

| Module suffix | Purpose |
| --- | --- |
| Root | General utilities and core event history |
| `http` | HTTP request/response utilities and headers |
| `server` | HTTP server support |
| `cache` | Cache utilities |
| `http/reverseproxy` | Reverse proxy |
| `http/websocket` | WebSocket support |
| `events/acl` | ACL blocked-event reporting |
| `events/http` | HTTP blocked-event reporting |
| `http/reverseproxy/integrationtest` | Proxy integration tests, not an application dependency |

Diagnostics require an installed [application logger](logging/README.md).
Nested modules may retain their own framework dependencies. HTTP logging helpers
accept a message and optional fields instead of returning a zerolog event.

## Development and CI

```sh
go test ./...
python3 scripts/release.py test
python3 -m unittest discover -s scripts -p 'test_*.py'
```

The first command tests the dependency-light root. The second tests every module,
including root debug/profiling builds and module tidy checks. It supplies the
linker option required by existing HTTP internals. Local module replacements
allow development without publishing intermediate versions.

CI runs on main-branch pushes and pull requests, with a separate result for each
module. The full task race suite currently has a known cleanup/root-pointer race;
CI does not run that suite. Use targeted race checks for changed concurrent code
until that issue is resolved.

## CI-Managed Releases

Maintainers publish all modules at one stable version through the Release
workflow. Supported versions are `v0.x.y` and `v1.x.y`.

```sh
gh workflow run release.yml --repo yusing/goutils --ref main -f version=v0.8.0
```

The workflow updates internal module dependency versions, validates the prepared
source, and commits the module metadata to main. It atomically pushes that commit
and all module tags, then creates a GitHub release and verifies public consumption
without local replacements. Root tags are `v0.8.0`; nested tags include the module
directory, such as `http/v0.8.0`. Actions needs permission to write repository
contents; no additional release secret is required.

Run releases from main. A published version cannot be reused or overwritten, even
if its Git tag was deleted. The workflow checks both existing tags and Go's
permanent checksum database before preparing a version. Public-consumption checks
retry up to thirty times, one minute apart, to allow Go services to observe new tags.
If validation or the atomic push fails before publication, rerun the workflow.
If release creation or public-consumption verification fails after tags are
published, rerun only the failed jobs of that run. Do not dispatch the same
version again. Release validation runs explicitly because pushes made with the
workflow's token do not start another push-triggered CI run.
