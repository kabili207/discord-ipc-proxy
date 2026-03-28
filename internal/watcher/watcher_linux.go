//go:build linux

package watcher

import (
	"context"
	"log/slog"
	"regexp"

	"github.com/fsnotify/fsnotify"
	"github.com/kabili207/discord-proxy/internal/ipc"
)

var ipcSocketPattern = regexp.MustCompile(`discord-ipc-\d+$`)

// Event represents a Discord IPC socket appearing or disappearing.
type Event struct {
	Path    string
	Created bool // true = created, false = removed
}

// Watch monitors directories for Discord IPC socket creation and removal.
// Events are sent on the returned channel. Blocks until the context is cancelled.
func Watch(ctx context.Context) (<-chan Event, error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	for _, dir := range ipc.WatchDirectories() {
		if err := w.Add(dir); err != nil {
			slog.Info("Cannot watch directory", "path", dir, "error", err)
		}
	}

	ch := make(chan Event, 4)

	go func() {
		defer w.Close()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-w.Events:
				if !ok {
					return
				}
				if !ipcSocketPattern.MatchString(event.Name) {
					continue
				}
				if event.Has(fsnotify.Create) {
					slog.Info("Discord IPC socket created", "path", event.Name)
					ch <- Event{Path: event.Name, Created: true}
				} else if event.Has(fsnotify.Remove) {
					slog.Info("Discord IPC socket removed", "path", event.Name)
					ch <- Event{Path: event.Name, Created: false}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				slog.Warn("Watcher error", "error", err)
			}
		}
	}()

	return ch, nil
}
