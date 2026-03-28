//go:build linux || darwin

package ipc

import (
	"fmt"
	"net"
)

var dialPaths = []string{
	"%s/discord-ipc-%d",
	"%s/app/com.discordapp.Discord/discord-ipc-%d",
	"%s/snap.discord-canary/discord-ipc-%d",
	"%s/snap.discord/discord-ipc-%d",
}

// DialDiscord connects to Discord's IPC socket by trying known paths.
func DialDiscord() (net.Conn, error) {
	tmp := GetTempPath()
	for _, pattern := range dialPaths {
		for i := 0; i < 10; i++ {
			conn, err := net.Dial("unix", fmt.Sprintf(pattern, tmp, i))
			if err == nil {
				return conn, nil
			}
		}
	}
	return nil, ErrDiscordNotFound
}

// DialDiscordPath connects to a specific Discord IPC socket path.
func DialDiscordPath(path string) (net.Conn, error) {
	return net.Dial("unix", path)
}
