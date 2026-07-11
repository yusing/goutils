//go:build !pprof

package synk

func addSizeInUse(b []byte)       {}
func removeSizeInUse(b []byte)    {}
func addNonPooled(size int)       {}
func addDropped(size int)         {}
func addReused(size int)          {}
func addReusedRemaining(size int) {}
func addGced(size int)            {}
func initPoolStats()              {}
func printPoolStats()             {}
