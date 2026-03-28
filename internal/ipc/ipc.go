package ipc

import (
	"errors"
	"os"
)

var ErrDiscordNotFound = errors.New("could not find discord ipc socket")

// GetTempPath returns the runtime/temp directory used for IPC sockets.
func GetTempPath() string {
	for _, env := range []string{"XDG_RUNTIME_DIR", "TMPDIR", "TMP", "TEMP"} {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return "/tmp"
}
