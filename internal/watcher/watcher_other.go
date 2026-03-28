//go:build !linux

package watcher

import (
	"context"
	"fmt"
)

// Watch is not supported on non-Linux platforms.
func Watch(_ context.Context, _ func(path string)) error {
	return fmt.Errorf("filesystem watching for Discord IPC sockets is only supported on Linux")
}
