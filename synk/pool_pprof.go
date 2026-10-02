//go:build pprof

package synk

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
	"weak"

	"github.com/yusing/goutils/logging"
	strutils "github.com/yusing/goutils/strings"
)

type poolCounters struct {
	num  atomic.Uint64
	size atomic.Uint64
}

type bufferInUse struct {
	ptr  weak.Pointer[byte]
	size uint64
}

var (
	nonPooled    poolCounters
	dropped      poolCounters
	reused       poolCounters
	gced         poolCounters
	sizeInUse    atomic.Uint64
	buffersInUse sync.Map // map[uintptr]*bufferInUse
)

func addSizeInUse(b []byte) {
	ptr := unsafe.SliceData(b)
	if ptr == nil {
		return
	}

	buffer := &bufferInUse{ptr: weak.Make(ptr), size: uint64(cap(b))}
	if previous, loaded := buffersInUse.Swap(uintptr(unsafe.Pointer(ptr)), buffer); loaded {
		sizeInUse.Add(-previous.(*bufferInUse).size)
	}
	sizeInUse.Add(buffer.size)
}

func removeSizeInUse(b []byte) {
	ptr := unsafe.SliceData(b)
	if ptr == nil {
		return
	}

	if buffer, loaded := buffersInUse.LoadAndDelete(uintptr(unsafe.Pointer(ptr))); loaded {
		sizeInUse.Add(-buffer.(*bufferInUse).size)
	}
}

func pruneBuffersInUse() {
	buffersInUse.Range(func(key, value any) bool {
		buffer := value.(*bufferInUse)
		if buffer.ptr.Value() == nil && buffersInUse.CompareAndDelete(key, buffer) {
			sizeInUse.Add(-buffer.size)
		}
		return true
	})
}

func addNonPooled(size int) {
	nonPooled.num.Add(1)
	nonPooled.size.Add(uint64(size))
}

func addReused(size int) {
	reused.num.Add(1)
	reused.size.Add(uint64(size))
}

func addDropped(size int) {
	dropped.num.Add(1)
	dropped.size.Add(uint64(size))
}

func addGced(size int) {
	gced.num.Add(1)
	gced.size.Add(uint64(size))
}

func initPoolStats() {
	go func() {
		statsTicker := time.NewTicker(5 * time.Second)
		defer statsTicker.Stop()

		sig := make(chan os.Signal, 1)
		signal.Notify(sig, os.Interrupt)

		for {
			select {
			case <-sig:
				return
			case <-statsTicker.C:
				printPoolStats()
			}
		}
	}()
}

func printPoolStats() {
	pruneBuffersInUse()

	logging.Log(logging.Info, "bytes pool stats",
		logging.Field{Key: "sizeInUse", Value: strutils.FormatByteSize(sizeInUse.Load())},
		logging.Field{Key: "numReused", Value: reused.num.Load()},
		logging.Field{Key: "sizeReused", Value: strutils.FormatByteSize(reused.size.Load())},
		logging.Field{Key: "numDropped", Value: dropped.num.Load()},
		logging.Field{Key: "sizeDropped", Value: strutils.FormatByteSize(dropped.size.Load())},
		logging.Field{Key: "numNonPooled", Value: nonPooled.num.Load()},
		logging.Field{Key: "sizeNonPooled", Value: strutils.FormatByteSize(nonPooled.size.Load())},
		logging.Field{Key: "numGced", Value: gced.num.Load()},
		logging.Field{Key: "sizeGced", Value: strutils.FormatByteSize(gced.size.Load())},
	)
}
