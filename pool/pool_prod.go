//go:build !debug

package pool

func (*Pool[T]) logExisting(string) {
	// no-op in production
}
