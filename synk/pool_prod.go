//go:build !pprof

package synk

func addNonPooled(size int)       {}
func addDropped(size int)         {}
func addReused(size int)          {}
func addReusedRemaining(b []byte) {}
func initPoolStats()              {}
func printPoolStats()             {}
