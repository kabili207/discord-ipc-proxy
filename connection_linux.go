package proxy

import (
	"net"
	"os"
	"strconv"
)

// GetTempPath - gets the temporary path
func GetTempPath() string {
	temp := os.Getenv("XDG_RUNTIME_DIR")
	if temp != "" {
		return temp
	}

	temp = os.Getenv("TMPDIR")
	if temp != "" {
		return temp
	}

	temp = os.Getenv("TMP")
	if temp != "" {
		return temp
	}

	temp = os.Getenv("TEMP")
	if temp != "" {
		return temp
	}

	return "/tmp"
}

// DialDiscord - connects to the discord ipc socket
func DialDiscord() (net.Conn, error) {
	path := GetTempPath()
	for i := 0; i < 10; i++ {
		con, err := net.Dial("unix", path+"/discord-ipc-"+strconv.Itoa(i))
		if err == nil {
			return con, nil
		}
	}
	return nil, ErrorDiscordNotFound
}

// ListenDiscord - create a local discord ipc socket
func ListenDiscord() (net.Listener, error) {
	path := GetTempPath()
	for i := 0; i < 10; i++ {
		l, err := net.Listen("unix", path+"/discord-ipc-"+strconv.Itoa(i))
		if err == nil {
			return l, nil
		}
	}
	return nil, ErrorDiscordNotFound
}
