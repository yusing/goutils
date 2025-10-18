package ioutils

import (
	"context"
	"errors"
	"io"
	"net/http"

	"github.com/yusing/goutils/synk"
)

var bytesPool = synk.GetSizedBytesPool()

// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// This is a copy of io.Copy with context and HTTP flusher handling
// Author: yusing <yusing@6uo.me>.
func CopyClose(dst *ContextWriter, src *ContextReader, sizeHint int) (err error) {
	size := 16384
	if l, ok := src.Reader.(*io.LimitedReader); ok {
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

	buf := bytesPool.GetSized(size)
	defer bytesPool.Put(buf)
	// close both as soon as one of them is done
	wCloser, wCanClose := dst.Writer.(io.Closer)
	rCloser, rCanClose := src.Reader.(io.Closer)
	if wCanClose || rCanClose {
		go func() {
			select {
			case <-src.ctx.Done():
			case <-dst.ctx.Done():
			}
			if rCanClose {
				defer rCloser.Close()
			}
			if wCanClose {
				defer wCloser.Close()
			}
		}()
	}
	flusher := getHTTPFlusher(dst.Writer)
	for {
		nr, er := src.Reader.Read(buf)
		if nr > 0 {
			nw, ew := dst.Writer.Write(buf[0:nr])
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
			if flusher != nil {
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

func CopyCloseWithContext(ctx context.Context, dst io.Writer, src io.Reader, sizeHint int) (err error) {
	return CopyClose(NewContextWriter(ctx, dst), NewContextReader(ctx, src), sizeHint)
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

func getHTTPFlusher(dst io.Writer) flushErrorInterface {
	// pre-unwrap the flusher to prevent unwrap and check in every loop
	if rw, ok := dst.(http.ResponseWriter); ok {
		for {
			switch t := rw.(type) {
			case flushErrorInterface:
				return t
			case http.Flusher:
				return &flusherWrapper{rw: t}
			case rwUnwrapper:
				rw = t.Unwrap()
			default:
				return nil
			}
		}
	}
	return nil
}
