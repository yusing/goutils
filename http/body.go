package httputils

import (
	"io"
	"net/http"

	"github.com/yusing/goutils/synk"
)

var bytesPool = synk.GetSizedBytesPool()
var unsizedPool = synk.GetUnsizedBytesPool()

func noopRelease([]byte) {}

// ReadAllBody reads the body of the response into a buffer and returns it and a function to release the buffer.
// If the response has a content length, it will be read into a sized buffer.
// Otherwise, it will be read into an unsized buffer.
// If error is not nil, the buffer will be released and release will be nil.
func ReadAllBody(resp *http.Response) (b []byte, release func([]byte), err error) {
	if unwritten, ok := resp.Body.(*UnwrittenBody); ok {
		return unwritten.Bytes(), noopRelease, nil
	}

	if resp.ContentLength > 0 {
		b = bytesPool.GetSized(int(resp.ContentLength))
		_, err = io.ReadFull(resp.Body, b)
		if err != nil {
			bytesPool.Put(b)
			return nil, nil, err
		}
		return b, bytesPool.Put, nil
	}

	b = unsizedPool.Get()
	release = unsizedPool.Put
	var totalRead int64
	// copied from io.ReadAll
	// Copyright 2009 The Go Authors. All rights reserved.
	// Use of this source code is governed by a BSD-style
	// license that can be found in the LICENSE file.
	for {
		n, err := resp.Body.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				if resp.ContentLength > 0 && totalRead < resp.ContentLength {
					release(b)
					return nil, nil, io.ErrUnexpectedEOF
				}
				err = nil
				return b, release, nil
			}
			release(b)
			return nil, nil, err
		}
		totalRead += int64(n)

		if len(b) >= cap(b) {
			b = append(b, 0)[:len(b)]
		}
	}
}
