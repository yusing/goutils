package httputils

import (
	"io"
	"net/http"

	"github.com/yusing/goutils/synk"
)

var bytesPool = synk.GetBytesPool()

// ReadAllBody reads the body of the response into a buffer and returns it and a function to release the buffer.
func ReadAllBody(resp *http.Response) (buf []byte, release func(), err error) {
	if contentLength := resp.ContentLength; contentLength > 0 {
		buf = bytesPool.GetSized(int(contentLength))
		_, err = io.ReadFull(resp.Body, buf)
		if err != nil {
			bytesPool.Put(buf)
			return nil, nil, err
		}
		return buf, func() { bytesPool.Put(buf) }, nil
	}
	buf, err = io.ReadAll(resp.Body)
	if err != nil {
		bytesPool.Put(buf)
		return nil, nil, err
	}
	return buf, func() { bytesPool.Put(buf) }, nil
}
