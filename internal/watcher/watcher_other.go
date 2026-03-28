//go:build !linux

package watcher

import (
	"context"
	"fmt"
)

// Event represents a Discord IPC socket appearing or disappearing.
type Event struct {
	Path    string
	Created bool
}

// Watch is not supported on non-Linux platforms.
func Watch(_ context.Context) (<-chan Event, error) {
	return nil, fmt.Errorf("filesystem watching for Discord IPC sockets is only supported on Linux")
}
