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

// Watch monitors directories for new Discord IPC sockets and calls onFound for each.
// It blocks until the context is cancelled.
func Watch(ctx context.Context, onFound func(path string)) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	for _, dir := range ipc.WatchDirectories() {
		if err := w.Add(dir); err != nil {
			slog.Info("Cannot watch directory", "path", dir, "error", err)
		}
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event, ok := <-w.Events:
			if !ok {
				return nil
			}
			if event.Has(fsnotify.Create) && ipcSocketPattern.MatchString(event.Name) {
				slog.Info("Discovered Discord IPC socket", "path", event.Name)
				onFound(event.Name)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return nil
			}
			slog.Warn("Watcher error", "error", err)
		}
	}
}
