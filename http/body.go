package httputils

import (
	"io"
	"net/http"

	"github.com/yusing/goutils/synk"
)

var bytesPool = synk.GetBytesPoolWithUniqueMemory()

func noopRelease([]byte) {}

// ReadAllBody reads the body of the response into a buffer and returns it and a function to release the buffer.
func ReadAllBody(resp *http.Response) (b []byte, release func([]byte), err error) {
	if resp.ContentLength > 0 && resp.ContentLength < synk.UnsizedAvg {
		b = make([]byte, resp.ContentLength)
		_, err = io.ReadFull(resp.Body, b)
		return b, noopRelease, err
	} else {
		b = bytesPool.Get()
		release = bytesPool.Put
	}

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
