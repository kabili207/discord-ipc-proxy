//go:build linux || darwin

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// listenUnix attempts to listen on a Unix socket path, removing stale sockets.
func listenUnix(path string) (net.Listener, error) {
	l, err := net.Listen("unix", path)
	if err == nil {
		return newCleanupListener(l, path), nil
	}

	// If the socket file exists, check if it's stale
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, err // file doesn't exist, real error
	}

	// Try connecting — if it fails, the socket is stale
	conn, dialErr := net.Dial("unix", path)
	if dialErr == nil {
		conn.Close()
		return nil, err // someone is actually listening, real conflict
	}

	// Stale socket — remove and retry
	os.Remove(path)
	l, err = net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	return newCleanupListener(l, path), nil
}

// cleanupListener wraps a net.Listener to remove the socket file on close.
type cleanupListener struct {
	net.Listener
	path string
}

func newCleanupListener(l net.Listener, path string) *cleanupListener {
	return &cleanupListener{Listener: l, path: path}
}

func (l *cleanupListener) Close() error {
	err := l.Listener.Close()
	os.Remove(l.path)
	return err
}

// ListenStandard creates a Discord IPC listener at the standard socket path.
func ListenStandard() (net.Listener, error) {
	tmp := GetTempPath()
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("%s/discord-ipc-%d", tmp, i)
		l, err := listenUnix(path)
		if err == nil {
			return l, nil
		}
	}
	return nil, ErrDiscordNotFound
}

// ListenFlatpak creates a Discord IPC listener at the Flatpak socket path.
func ListenFlatpak() (net.Listener, error) {
	tmp := GetTempPath()
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("%s/app/com.discordapp.Discord/discord-ipc-%d", tmp, i)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return nil, fmt.Errorf("creating flatpak ipc directory: %w", err)
		}
		l, err := listenUnix(path)
		if err == nil {
			return l, nil
		}
	}
	return nil, ErrDiscordNotFound
}
