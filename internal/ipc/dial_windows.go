//go:build windows

package ipc

import (
	"net"
	"strconv"
	"time"

	"gopkg.in/natefinch/npipe.v2"
)

// DialDiscord connects to Discord's IPC named pipe on Windows.
func DialDiscord() (net.Conn, error) {
	for i := 0; i < 10; i++ {
		conn, err := npipe.DialTimeout(`\\.\pipe\discord-ipc-`+strconv.Itoa(i), 1*time.Second)
		if err == nil {
			return conn, nil
		}
	}
	return nil, ErrDiscordNotFound
}

// DialDiscordPath is not used on Windows but provided for interface consistency.
func DialDiscordPath(path string) (net.Conn, error) {
	return npipe.DialTimeout(path, 1*time.Second)
}
