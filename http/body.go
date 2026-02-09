package httputils

import (
	"io"
	"net/http"

	"github.com/yusing/goutils/synk"
)

var (
	bytesPool   = synk.GetSizedBytesPool()
	unsizedPool = synk.GetUnsizedBytesPool()
)

func noopRelease([]byte) {}

// ReadAllBody reads the body of the response into a buffer and returns it and a function to release the buffer.
// If the response has a content length, it will be read into a sized buffer.
// Otherwise, it will be read into an unsized buffer.
// If error is not nil, the buffer will be released and release will be nil.
func ReadAllBody(resp *http.Response) (b []byte, release func([]byte), err error) {
	if unwritten, ok := resp.Body.(*UnwrittenBody); ok {
		return unwritten.Bytes(), noopRelease, nil
	}

	return readAll(int(resp.ContentLength), resp.Body)
}

// ReadAllRequestBody reads the body of the request into a buffer and returns it and a function to release the buffer.
// If the response has a content length, it will be read into a sized buffer.
// Otherwise, it will be read into an unsized buffer.
// If error is not nil, the buffer will be released and release will be nil.
func ReadAllRequestBody(req *http.Request) (b []byte, release func([]byte), err error) {
	return readAll(int(req.ContentLength), req.Body)
}

func readAll(size int, r io.Reader) (b []byte, release func([]byte), err error) {
	if size > 0 {
		b = bytesPool.GetSized(size)
		_, err = io.ReadFull(r, b)
		if err != nil {
			bytesPool.Put(b)
			return nil, nil, err
		}
		return b, bytesPool.Put, nil
	}

	b = unsizedPool.Get()
	release = unsizedPool.Put
	var totalRead int
	// copied from io.ReadAll
	// Copyright 2009 The Go Authors. All rights reserved.
	// Use of this source code is governed by a BSD-style
	// license that can be found in the LICENSE file.
	for {
		n, err := r.Read(b[len(b):cap(b)])
		b = b[:len(b)+n]
		if err != nil {
			if err == io.EOF {
				return b, release, nil
			}
			release(b)
			return nil, nil, err
		}
		totalRead += n

		if len(b) >= cap(b) {
			b = append(b, 0)[:len(b)]
		}
	}
}
