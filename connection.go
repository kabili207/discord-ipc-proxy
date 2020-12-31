package proxy

import (
	"errors"
)

var (
	// ErrorDiscordNotFound - Discord connection not found
	ErrorDiscordNotFound = errors.New("could not find discord")
)
