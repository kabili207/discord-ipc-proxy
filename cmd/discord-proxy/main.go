package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"

	sloghelper "github.com/kabili207/slog-helper"

	"github.com/kabili207/discord-proxy/internal/ipc"
	"github.com/kabili207/discord-proxy/internal/protocol"
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

// connWrapper optionally transforms a net.Conn into a framed ReadWriteCloser.
type connWrapper func(net.Conn, *slog.Logger) (io.ReadWriteCloser, error)

var connID atomic.Uint64

func nextConnLogger() *slog.Logger {
	id := connID.Add(1)
	return slog.With("conn", id)
}

// acceptLoopOpts configures an accept loop.
type acceptLoopOpts struct {
	proxyOpts
	wrapLocal  connWrapper // applied to the accepted (local) connection
	wrapRemote connWrapper // applied to the dialed (remote) connection
}

// acceptLoop accepts connections from a listener and proxies them.
// Blocks until the context is cancelled.
func acceptLoop(ctx context.Context, listener net.Listener, dialFn func() (net.Conn, error), opts acceptLoopOpts) {
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

		var local io.ReadWriteCloser = conn
		var remote io.ReadWriteCloser = rconn

		if opts.wrapLocal != nil {
			local, err = opts.wrapLocal(conn, log)
			if err != nil {
				log.Warn("Local connection setup failed", "error", err)
				conn.Close()
				rconn.Close()
				continue
			}
		}

		if opts.wrapRemote != nil {
			remote, err = opts.wrapRemote(rconn, log)
			if err != nil {
				log.Warn("Remote connection setup failed", "error", err)
				local.Close()
				rconn.Close()
				continue
			}
		}

		p := proxy.New(local, remote, log)
		p.SetDisableNagle(opts.disableNagle)
		p.SetOutputHex(opts.outputHex)
		go p.Run()
	}
}

// serverHandshake performs the server-side protocol handshake on a TCP connection.
func serverHandshake(conn net.Conn, log *slog.Logger) (io.ReadWriteCloser, error) {
	clientHS, err := protocol.ReadHandshake(conn)
	if err != nil {
		return nil, fmt.Errorf("reading client handshake: %w", err)
	}
	log.Info("Client connected",
		"version", clientHS.Version,
		"os", clientHS.OS,
		"hostname", clientHS.Hostname,
	)

	discordAvailable := true
	if testConn, err := ipc.DialDiscord(); err != nil {
		discordAvailable = false
	} else {
		testConn.Close()
	}

	serverHS := protocol.LocalHandshake()
	serverHS.DiscordAvailable = &discordAvailable
	if err := protocol.WriteHandshake(conn, serverHS); err != nil {
		return nil, fmt.Errorf("writing server handshake: %w", err)
	}

	fc := protocol.NewFramedConn(conn)
	fc.OnControl = func(ctrl protocol.Control) {
		log.Info("Control message from client", "status", ctrl.Status, "detail", ctrl.Detail)
	}
	return fc, nil
}

// clientHandshake performs the client-side protocol handshake on a TCP connection.
func clientHandshake(conn net.Conn, log *slog.Logger) (io.ReadWriteCloser, error) {
	clientHS := protocol.LocalHandshake()
	if err := protocol.WriteHandshake(conn, clientHS); err != nil {
		return nil, fmt.Errorf("writing client handshake: %w", err)
	}

	serverHS, err := protocol.ReadHandshake(conn)
	if err != nil {
		return nil, fmt.Errorf("reading server handshake: %w", err)
	}
	log.Info("Server connected",
		"version", serverHS.Version,
		"os", serverHS.OS,
		"hostname", serverHS.Hostname,
	)

	if serverHS.DiscordAvailable != nil && !*serverHS.DiscordAvailable {
		log.Warn("Server reports Discord is not available")
	}

	fc := protocol.NewFramedConn(conn)
	fc.OnControl = func(ctrl protocol.Control) {
		log.Info("Control message from server", "status", ctrl.Status, "detail", ctrl.Detail)
	}
	return fc, nil
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

	// TCP side gets server handshake + framing; IPC side is raw.
	wg.Add(1)
	go func() {
		defer wg.Done()
		acceptLoop(ctx, listener, ipc.DialDiscord, acceptLoopOpts{
			proxyOpts: opts,
			wrapLocal: serverHandshake,
		})
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

	// IPC side is raw; TCP (remote) side gets client handshake + framing.
	loopOpts := acceptLoopOpts{
		proxyOpts:  opts,
		wrapRemote: clientHandshake,
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		acceptLoop(ctx, listener, dialTCP, loopOpts)
	}()

	if flatpak {
		if err := startFlatpakListeners(ctx, &wg, dialTCP, opts, clientHandshake); err != nil {
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
//
// wrapRemote is optional — if provided, the dialed connection is wrapped with
// framing (used when flatpak mode is combined with TCP client mode).
func startFlatpakListeners(ctx context.Context, wg *sync.WaitGroup, dialFn func() (net.Conn, error), opts proxyOpts, wrapRemote ...connWrapper) error {
	events, err := watcher.Watch(ctx)
	if err != nil {
		return fmt.Errorf("starting watcher: %w", err)
	}

	var remoteWrapper connWrapper
	if len(wrapRemote) > 0 {
		remoteWrapper = wrapRemote[0]
	}

	loopOpts := acceptLoopOpts{
		proxyOpts:  opts,
		wrapRemote: remoteWrapper,
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
				acceptLoop(listenerCtx, l, dialFn, loopOpts)
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
