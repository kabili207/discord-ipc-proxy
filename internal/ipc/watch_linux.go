//go:build linux

package ipc

import "fmt"

// WatchDirectories returns directories to monitor for new Discord IPC sockets.
func WatchDirectories() []string {
	tmp := GetTempPath()
	return []string{
		fmt.Sprintf("%s/", tmp),
		fmt.Sprintf("%s/snap.discord-canary/", tmp),
		fmt.Sprintf("%s/snap.discord/", tmp),
	}
}
