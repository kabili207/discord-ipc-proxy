//go:build !linux

package ipc

// WatchDirectories is not supported on non-Linux platforms.
func WatchDirectories() []string {
	return nil
}
