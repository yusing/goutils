//go:build pprof

package synk

import (
	"os"
	"os/signal"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"
	strutils "github.com/yusing/goutils/strings"
)

type poolCounters struct {
	num  atomic.Uint64
	size atomic.Uint64
}

var (
	nonPooled       poolCounters
	dropped         poolCounters
	reused          poolCounters
	reusedRemaining poolCounters
	gced            poolCounters
)

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

func addReusedRemaining(size int) {
	reusedRemaining.num.Add(1)
	reusedRemaining.size.Add(uint64(size))
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
	log.Info().
		Uint64("numReused", reused.num.Load()).
		Str("sizeReused", strutils.FormatByteSize(reused.size.Load())).
		Uint64("numDropped", dropped.num.Load()).
		Str("sizeDropped", strutils.FormatByteSize(dropped.size.Load())).
		Uint64("numNonPooled", nonPooled.num.Load()).
		Str("sizeNonPooled", strutils.FormatByteSize(nonPooled.size.Load())).
		Uint64("numReusedRemaining", reusedRemaining.num.Load()).
		Str("sizeReusedRemaining", strutils.FormatByteSize(reusedRemaining.size.Load())).
		Uint64("numGced", gced.num.Load()).
		Str("sizeGced", strutils.FormatByteSize(gced.size.Load())).
		Msg("bytes pool stats")
}
