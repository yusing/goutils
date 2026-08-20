package websocket

import (
	"bufio"
	"errors"
	"io"
	"sync/atomic"
	"time"

	"github.com/yusing/goutils/synk"
)

const maxBufferedTextBytes int64 = 64 << 20

// ErrTextBufferBudgetExceeded means that active text streams exhausted their shared buffer budget.
var ErrTextBufferBudgetExceeded = errors.New("websocket text buffer budget exceeded")

var (
	bufferedTextBytes atomic.Int64
	textBufferPool    = synk.GetUnsizedBytesPool()
)

func reserveBufferedTextBytes(size int64) bool {
	for {
		used := bufferedTextBytes.Load()
		if size > maxBufferedTextBytes-used {
			return false
		}
		if bufferedTextBytes.CompareAndSwap(used, used+size) {
			return true
		}
	}
}

// CopyTextLines copies each newline-delimited record from r to a separate WebSocket text message.
// Active records larger than the read buffer share a 64 MiB buffered-payload budget.
func (cm *Manager) CopyTextLines(r io.Reader) error {
	reader := bufio.NewReader(r)
	for {
		fragment, readErr := reader.ReadSlice('\n')
		if len(fragment) == 0 && readErr != nil {
			if errors.Is(readErr, io.EOF) {
				return nil
			}
			return readErr
		}
		switch {
		case readErr == nil:
			if err := cm.WriteData(TextMessage, fragment, 10*time.Second); err != nil {
				return err
			}
		case errors.Is(readErr, io.EOF):
			return cm.WriteData(TextMessage, fragment, 10*time.Second)
		case errors.Is(readErr, bufio.ErrBufferFull):
			reachedEOF, err := cm.copyLongTextLine(reader, fragment)
			if err != nil {
				return err
			}
			if reachedEOF {
				return nil
			}
		default:
			return readErr
		}
	}
}

func (cm *Manager) copyLongTextLine(reader *bufio.Reader, firstFragment []byte) (bool, error) {
	record := textBufferPool.GetBuffer()
	defer textBufferPool.PutBuffer(record)
	defer func() {
		bufferedTextBytes.Add(-int64(record.Len()))
	}()

	appendFragment := func(fragment []byte) error {
		fragmentSize := int64(len(fragment))
		if !reserveBufferedTextBytes(fragmentSize) {
			return ErrTextBufferBudgetExceeded
		}
		record.Write(fragment)
		return nil
	}

	if err := appendFragment(firstFragment); err != nil {
		return false, err
	}
	for {
		fragment, readErr := reader.ReadSlice('\n')
		if len(fragment) > 0 {
			if err := appendFragment(fragment); err != nil {
				return false, err
			}
		}

		switch {
		case readErr == nil:
			return false, cm.WriteData(TextMessage, record.Bytes(), 10*time.Second)
		case errors.Is(readErr, io.EOF):
			return true, cm.WriteData(TextMessage, record.Bytes(), 10*time.Second)
		case errors.Is(readErr, bufio.ErrBufferFull):
			continue
		default:
			return false, readErr
		}
	}
}
