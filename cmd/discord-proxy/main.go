package main

import (
	"flag"
	"fmt"
	"net"
	"os"

	proxy "github.com/kabili207/discord-ipc-proxy"
)

var (
	version = "0.0.0-src"
	matchid = uint64(0)
	connid  = uint64(0)
	logger  proxy.ColorLogger

	serverAddr  = flag.String("a", ":9999", "server address")
	isClient    = flag.Bool("c", false, "run as a client instead of a server")
	verbose     = flag.Bool("v", false, "display server actions")
	veryverbose = flag.Bool("vv", false, "display server actions and all tcp data")
	nagles      = flag.Bool("n", false, "disable nagles algorithm")
	hex         = flag.Bool("h", false, "output hex")
	colors      = flag.Bool("ac", false, "output ansi colors")
)

func main() {
	flag.Parse()

	logger := proxy.ColorLogger{
		Verbose: *verbose,
		Color:   *colors,
	}

	if *isClient {
		logger.Info("discord-proxy (%s) connecting to %v", version, *serverAddr)
	} else {
		logger.Info("discord-proxy (%s) listening on %v", version, *serverAddr)
	}
	addr, err := net.ResolveTCPAddr("tcp", *serverAddr)
	if err != nil {
		logger.Warn("Failed to resolve server address: %s", err)
		os.Exit(1)
	}

	var listener net.Listener

	if *isClient {
		listener, err = proxy.ListenDiscord()
		if err != nil {
			logger.Warn("Failed to create local discord ipc pipe: %s", err)
			os.Exit(1)
		}
	} else {
		listener, err = net.ListenTCP("tcp", addr)
		if err != nil {
			logger.Warn("Failed to open local port to listen: %s", err)
			os.Exit(1)
		}
	}

	if *veryverbose {
		*verbose = true
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Warn("Failed to accept connection '%s'", err)
			continue
		}
		connid++

		var p *proxy.Proxy
		p = proxy.New(conn, addr, !*isClient)

		p.Nagles = *nagles
		p.OutputHex = *hex
		p.Log = proxy.ColorLogger{
			Verbose:     *verbose,
			VeryVerbose: *veryverbose,
			Prefix:      fmt.Sprintf("Connection #%03d ", connid),
			Color:       *colors,
		}

		go p.Start()
	}
}
