# goutils/fs

Filesystem utilities for directory traversal and file listing.

## Overview

The `fs` package provides utilities for recursively listing files in directories.

## API Reference

```go
func ListFiles(dir string, maxDepth int, hideHidden ...bool) ([]string, error)
```

## Usage

```go
// List all files recursively
files, _ := fs.ListFiles("/path/to/dir", 10)

// List only top-level files
files, _ := fs.ListFiles("/path/to/dir", 1)

// Exclude hidden files
files, _ := fs.ListFiles("/path/to/dir", 10, true)
```
