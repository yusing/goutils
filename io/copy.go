package ioutils

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"

	"github.com/yusing/goutils/synk"
)

var bytesPool = synk.GetSizedBytesPool()

var noContext context.Context

func CopyCloseWithContext(ctx context.Context, dst io.Writer, src io.Reader, sizeHint int) (err error) {
	return copyClose(ctx, dst, src, sizeHint)
}

func CopyClose(dst io.Writer, src io.Reader, sizeHint int) (err error) {
	return copyClose(noContext, dst, src, sizeHint)
}

const minBufferSize = 256

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// This is a copy of io.Copy with context and HTTP flusher handling
// Author: yusing <yusing@6uo.me>.
func copyClose(ctx context.Context, dst io.Writer, src io.Reader, sizeHint int) (err error) {
	size := 32 * 1024
	if l, ok := src.(*io.LimitedReader); ok {
		if int64(size) > l.N {
			if l.N < 1 {
				size = 1
			} else {
				size = int(l.N)
			}
		}
	} else if sizeHint > 0 {
		size = sizeHint
	}

	var buf []byte
	if size > minBufferSize {
		buf = bytesPool.GetSized(min(size, 32*1024)) // limit the buffer size to 32KB
		defer bytesPool.Put(buf)
	} else {
		var array [minBufferSize]byte
		buf = array[:size]
	}

	if ctx != nil {
		// close both as soon as one of them is done
		wCloser, wCanClose := dst.(io.Closer)
		rCloser, rCanClose := src.(io.Closer)
		if wCanClose || rCanClose {
			close := func() {
				if rCanClose {
					rCloser.Close()
				}
				if wCanClose {
					wCloser.Close()
				}
			}
			context.AfterFunc(ctx, close)
		}
	}

	flusher, shouldFlush := getHTTPFlusher(dst)
	for {
		nr, er := src.Read(buf)
		if nr > 0 {
			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			if ew != nil {
				err = ew
				return
			}
			if nr != nw {
				err = io.ErrShortWrite
				return
			}
			if shouldFlush {
				err = flusher.FlushError()
				if err != nil {
					return err
				}
			}
		}
		if er != nil {
			if er != io.EOF {
				err = er
			}
			return
		}
	}
}

type flushErrorInterface interface {
	FlushError() error
}

type flusherWrapper struct {
	rw http.Flusher
}

type rwUnwrapper interface {
	Unwrap() http.ResponseWriter
}

func (f *flusherWrapper) FlushError() error {
	f.rw.Flush()
	return nil
}

func getHTTPFlusher(dst io.Writer) (flusher flushErrorInterface, shouldFlush bool) {
	// pre-unwrap the flusher to prevent unwrap and check in every loop
	if rw, ok := dst.(http.ResponseWriter); ok {
		for {
			switch t := rw.(type) {
			case flushErrorInterface:
				return t, shouldFlushHTTPWriter(rw)
			case http.Flusher:
				return &flusherWrapper{rw: t}, shouldFlushHTTPWriter(rw)
			case rwUnwrapper:
				rw = t.Unwrap()
			default:
				return nil, false
			}
		}
	}
	return nil, false
}

func shouldFlushHTTPWriter(rw http.ResponseWriter) bool {
	header := rw.Header()

	contentType := header.Get("Content-Type")
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil && strings.EqualFold(mediaType, "text/event-stream") {
			return true
		}
	}

	if header.Get("Content-Length") != "" {
		return false
	}

	for _, value := range header.Values("Transfer-Encoding") {
		for token := range strings.SplitSeq(value, ",") {
			if strings.EqualFold(strings.TrimSpace(token), "chunked") {
				return true
			}
		}
	}

	return false
}
