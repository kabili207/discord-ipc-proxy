package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	sloghelper "github.com/kabili207/slog-helper"

	"github.com/kabili207/discord-proxy/internal/ipc"
	"github.com/kabili207/discord-proxy/internal/proxy"
	"github.com/kabili207/discord-proxy/internal/watcher"
)

var version = "0.0.0-src"

func main() {
	sloghelper.InitFromEnv()

	if err := run(); err != nil {
		slog.Error("Fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	serverAddr := flag.String("a", ":9999", "TCP server address")
	flatpak := flag.Bool("flatpak", false, "enable Flatpak IPC proxy (Linux only)")
	disableNagle := flag.Bool("n", false, "disable Nagle's algorithm")
	outputHex := flag.Bool("hex", false, "log data as hex dump")
	logLevel := flag.String("log-level", "", "log level (debug, info, warn, error)")
	logFormat := flag.String("log-format", "", "log format (color, json, journal)")
	flag.Parse()

	sloghelper.InitFromConfig(*logLevel, *logFormat, "")

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: discord-proxy <command> [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  server    Listen on TCP, forward to Discord IPC\n")
		fmt.Fprintf(os.Stderr, "  client    Listen on Discord IPC, forward to TCP\n")
		fmt.Fprintf(os.Stderr, "  flatpak   Local IPC-to-IPC proxy for Flatpak apps\n")
		return fmt.Errorf("no command specified")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	opts := proxyOpts{
		disableNagle: *disableNagle,
		outputHex:    *outputHex,
	}

	slog.Info("discord-proxy", "version", version)

	switch args[0] {
	case "server":
		return runServer(ctx, *serverAddr, *flatpak, opts)
	case "client":
		return runClient(ctx, *serverAddr, *flatpak, opts)
	case "flatpak":
		return runFlatpak(ctx, opts)
	default:
		return fmt.Errorf("unknown command: %s", args[0])
	}
}

type proxyOpts struct {
	disableNagle bool
	outputHex    bool
}

var connID atomic.Uint64

func nextConnLogger() *slog.Logger {
	id := connID.Add(1)
	return slog.With("conn", id)
}

// acceptLoop accepts connections from a listener and proxies them using dialFn.
// Blocks until the context is cancelled.
func acceptLoop(ctx context.Context, listener net.Listener, dialFn func() (net.Conn, error), opts proxyOpts) {
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			slog.Warn("Failed to accept connection", "error", err)
			continue
		}

		log := nextConnLogger()
		log.Info("New connection", "from", proxy.LocalAddr(conn))

		rconn, err := dialFn()
		if err != nil {
			log.Warn("Failed to connect to remote", "error", err)
			conn.Close()
			continue
		}

		p := proxy.New(conn, rconn, log)
		p.SetDisableNagle(opts.disableNagle)
		p.SetOutputHex(opts.outputHex)
		go p.Run()
	}
}

// runServer listens on TCP and forwards each connection to Discord IPC.
// With --flatpak, it also listens on the Flatpak IPC path and watches for new sockets.
func runServer(ctx context.Context, addr string, flatpak bool, opts proxyOpts) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return fmt.Errorf("resolving server address: %w", err)
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", addr, err)
	}
	slog.Info("TCP server listening", "addr", addr)

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		acceptLoop(ctx, listener, ipc.DialDiscord, opts)
	}()

	if flatpak {
		if err := startFlatpakListeners(ctx, &wg, ipc.DialDiscord, opts); err != nil {
			return err
		}
	}

	wg.Wait()
	return nil
}

// runClient listens on Discord IPC and forwards each connection to a TCP server.
// With --flatpak, it also listens on the Flatpak IPC path and watches for new sockets.
func runClient(ctx context.Context, addr string, flatpak bool, opts proxyOpts) error {
	tcpAddr, err := net.ResolveTCPAddr("tcp", addr)
	if err != nil {
		return fmt.Errorf("resolving server address: %w", err)
	}

	dialTCP := func() (net.Conn, error) {
		return net.DialTCP("tcp", nil, tcpAddr)
	}

	slog.Info("Forwarding Discord IPC to TCP", "addr", addr)

	listener, err := ipc.ListenStandard()
	if err != nil {
		return fmt.Errorf("creating Discord IPC listener: %w", err)
	}
	slog.Info("Listening on standard IPC path")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		acceptLoop(ctx, listener, dialTCP, opts)
	}()

	if flatpak {
		if err := startFlatpakListeners(ctx, &wg, dialTCP, opts); err != nil {
			return err
		}
	}

	wg.Wait()
	return nil
}

// runFlatpak runs a pure local IPC-to-IPC proxy: listens on Flatpak paths,
// dials Discord IPC, and watches for new sockets.
func runFlatpak(ctx context.Context, opts proxyOpts) error {
	slog.Info("Running Flatpak IPC proxy")

	var wg sync.WaitGroup

	if err := startFlatpakListeners(ctx, &wg, ipc.DialDiscord, opts); err != nil {
		return err
	}

	wg.Wait()
	return nil
}

// startFlatpakListeners manages the Flatpak IPC listener lifecycle based on
// Discord's presence. The listener is created when Discord is detected and
// torn down when Discord exits, so sandboxed apps see ENOENT instead of
// connecting to a proxy that can't reach Discord.
func startFlatpakListeners(ctx context.Context, wg *sync.WaitGroup, dialFn func() (net.Conn, error), opts proxyOpts) error {
	events, err := watcher.Watch(ctx)
	if err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}

	// Check if Discord is already running at startup.
	discordPresent := false
	if conn, err := ipc.DialDiscord(); err == nil {
		conn.Close()
		discordPresent = true
	}

	wg.Add(1)
	go func() {
		defer wg.Done()

		var listener net.Listener
		var listenerCancel context.CancelFunc

		startListener := func() {
			if listener != nil {
				return // already running
			}
			l, err := ipc.ListenFlatpak()
			if err != nil {
				slog.Warn("Failed to create Flatpak IPC listener", "error", err)
				return
			}
			listener = l
			slog.Info("Flatpak IPC listener started")

			listenerCtx, cancel := context.WithCancel(ctx)
			listenerCancel = cancel

			wg.Add(1)
			go func() {
				defer wg.Done()
				acceptLoop(listenerCtx, l, dialFn, opts)
			}()
		}

		stopListener := func() {
			if listener == nil {
				return // not running
			}
			slog.Info("Flatpak IPC listener stopped")
			listenerCancel()
			listener = nil
			listenerCancel = nil
		}

		defer stopListener()

		if discordPresent {
			startListener()
		} else {
			slog.Info("Discord not detected, waiting for it to start")
		}

		for event := range events {
			if event.Created {
				startListener()
			} else {
				stopListener()
			}
		}
	}()

	return nil
}
