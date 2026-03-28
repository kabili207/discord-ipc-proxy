//go:build windows

package ipc

import (
	"fmt"
	"net"
	"strconv"

	"gopkg.in/natefinch/npipe.v2"
)

// ListenStandard creates a Discord IPC listener on a Windows named pipe.
func ListenStandard() (net.Listener, error) {
	for i := 0; i < 10; i++ {
		l, err := npipe.Listen(`\\.\pipe\discord-ipc-` + strconv.Itoa(i))
		if err == nil {
			return l, nil
		}
	}
	return nil, ErrDiscordNotFound
}

// ListenFlatpak is not applicable on Windows and always returns an error.
func ListenFlatpak() (net.Listener, error) {
	return nil, fmt.Errorf("flatpak mode is not supported on Windows")
}
