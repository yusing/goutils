//go:build !debug

package pool

func (*Pool[T]) checkExists(string) {
	// no-op in production
}
