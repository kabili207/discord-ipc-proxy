package proxy

import (
	"net"
	"strconv"
	"time"

	"gopkg.in/natefinch/npipe.v2"
)

// DialDiscord - connects to the discord ipc pipe
func DialDiscord() (net.Conn, error) {
	for i := 0; i < 10; i++ {
		con, err := npipe.DialTimeout(`\\.\pipe\discord-ipc-`+strconv.Itoa(i), 1*time.Second)
		if err == nil {
			return con, nil
		}
	}
	return nil, ErrorDiscordNotFound
}

// ListenDiscord - create a local discord ipc socket
func ListenDiscord() (net.Listener, error) {
	for i := 0; i < 10; i++ {
		l, err := npipe.Listen(`\\.\pipe\discord-ipc-` + strconv.Itoa(i))
		if err == nil {
			return l, nil
		}
	}
	return nil, ErrorDiscordNotFound
}
