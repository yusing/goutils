package websocket

import (
	"errors"
	"io"
	"net"
	"time"
)

type Reader struct {
	manager *Manager
	pending []byte
}

func (m *Manager) NewReader() io.Reader {
	return &Reader{
		manager: m,
	}
}

func (r *Reader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for len(r.pending) == 0 {
		data, err := r.manager.ReadBinary(10 * time.Second)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return 0, io.EOF
			}
			return 0, err
		}
		r.pending = data
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}
