# goutils/fs

Recursive file listing with a depth limit.

## Install

```sh
go get github.com/yusing/goutils@v0.8.0
```

```go
import "github.com/yusing/goutils/fs"
```

The package name is `fs`, which is also the name of the standard library's `io/fs`;
alias one of them if you import both. It uses only the standard library and needs
Go 1.27 or newer.

## Quick start

```go
package main

import (
	"fmt"
	"os"

	"github.com/yusing/goutils/fs"
)

func main() {
	dir, err := os.MkdirTemp("", "fs-example")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	if err := os.Chdir(dir); err != nil {
		panic(err)
	}

	os.MkdirAll("a/b", 0o755)
	for _, name := range []string{"top.txt", ".hidden", "a/mid.txt", "a/b/deep.txt"} {
		os.WriteFile(name, nil, 0o644)
	}

	files, _ := fs.ListFiles(".", 0)
	fmt.Println(files)
	files, _ = fs.ListFiles(".", 1)
	fmt.Println(files)
	files, _ = fs.ListFiles(".", 5, true)
	fmt.Println(files)
}
```

Output:

```text
[.hidden top.txt]
[.hidden a/mid.txt top.txt]
[a/b/deep.txt a/mid.txt top.txt]
```

## `ListFiles(dir, maxDepth, hideHidden...)`

```go
func ListFiles(dir string, maxDepth int, hideHidden ...bool) ([]string, error)
```

- **Depth.** `maxDepth` is how many directory levels to descend below `dir`. `0` lists only
  the files directly in `dir`, `1` adds the files of its immediate subdirectories, and so
  on. A negative value behaves like `0`.
- **Result.** The slice holds every non-directory entry, each as `path.Join(dir, name)`.
  That is `dir` followed by the path below it, not a path relative to `dir`: with
  `dir = "/data"` you get `/data/a/mid.txt`. It is never `nil` on success. Directories
  themselves are not listed, so an empty directory contributes nothing.
- **Order.** Entries come in `os.ReadDir` order (sorted by name) within each directory,
  with subdirectories expanded where they appear.
- **Symlinks.** Links are listed as files, even when they point to a directory, and are
  not followed.
- **Hidden entries.** `ListFiles(dir, n, true)` skips names that start with `.` in `dir`
  itself. The flag is not passed down, so hidden files and directories inside
  subdirectories are still listed.
- **Errors.** If `dir`, or any subdirectory that the depth limit lets it enter, cannot be
  read, the call returns `nil` and `error listing directory <path>: <cause>`. The cause is
  wrapped, so `errors.Is(err, os.ErrNotExist)` works. Entries found before the failure are
  discarded.
- **Separators.** Paths are joined with `path.Join`, so they use `/` on every platform.
