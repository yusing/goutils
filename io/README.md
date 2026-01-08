# io

IO utilities for copying data with context support and buffer pooling.

## Overview

The `io` package provides enhanced IO operations including context-aware copying.

## API Reference

```go
func CopyCloseWithContext(ctx context.Context, dst io.Writer, src io.Reader, sizeHint int) error
func CopyClose(dst io.Writer, src io.Reader, sizeHint int) error
```

## Usage

```go
// Basic copy
ioutils.CopyClose(resp.Body, out, 32*1024)

// Context-aware copy
ioutils.CopyCloseWithContext(ctx, w, upstream.Body, 8*1024)
```

## Features

- Uses 32KB default buffer
- Supports size hints for optimal allocation (optional)
- Uses `synk.GetSizedBytesPool` for buffer pooling to reduce GC pressure
- HTTP flusher support for streaming
- Automatic cleanup on context cancellation
