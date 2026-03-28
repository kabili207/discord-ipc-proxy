//go:build linux || darwin

package ipc

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
)

// ListenStandard creates a Discord IPC listener at the standard socket path.
func ListenStandard() (net.Listener, error) {
	tmp := GetTempPath()
	for i := 0; i < 10; i++ {
		path := fmt.Sprintf("%s/discord-ipc-%d", tmp, i)
		l, err := net.Listen("unix", path)
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
		l, err := net.Listen("unix", path)
		if err == nil {
			return l, nil
		}
	}
	return nil, ErrDiscordNotFound
}
