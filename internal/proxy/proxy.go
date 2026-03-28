package proxy

import (
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net"
)

// Proxy manages a bidirectional connection, piping data between local and remote.
type Proxy struct {
	sentBytes     uint64
	receivedBytes uint64
	lconn, rconn  io.ReadWriteCloser
	erred         bool
	errsig        chan bool
	log           *slog.Logger
	outputHex     bool
	disableNagle  bool
}

type setNoDelayer interface {
	SetNoDelay(bool) error
}

// New creates a Proxy that pipes data between lconn and rconn.
// Both connections are closed when the proxy finishes.
func New(lconn, rconn io.ReadWriteCloser, log *slog.Logger) *Proxy {
	return &Proxy{
		lconn:  lconn,
		rconn:  rconn,
		errsig: make(chan bool),
		log:    log,
	}
}

// SetOutputHex enables hex-encoded data logging.
func (p *Proxy) SetOutputHex(v bool) { p.outputHex = v }

// SetDisableNagle disables Nagle's algorithm on TCP connections.
func (p *Proxy) SetDisableNagle(v bool) { p.disableNagle = v }

// Run starts the bidirectional pipe and blocks until the connection closes.
func (p *Proxy) Run() {
	defer p.lconn.Close()
	defer p.rconn.Close()

	if p.disableNagle {
		if conn, ok := p.lconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
		if conn, ok := p.rconn.(setNoDelayer); ok {
			conn.SetNoDelay(true)
		}
	}

	p.log.Info("Connection opened")

	go p.pipe(p.lconn, p.rconn, "send")
	go p.pipe(p.rconn, p.lconn, "recv")

	<-p.errsig
	p.log.Info("Connection closed", "sent", p.sentBytes, "received", p.receivedBytes)
}

func (p *Proxy) signalError(msg string, err error) {
	if p.erred {
		return
	}
	if err != io.EOF {
		p.log.Warn(msg, "error", err)
	}
	p.errsig <- true
	p.erred = true
}

func (p *Proxy) pipe(src, dst io.ReadWriter, direction string) {
	isSend := direction == "send"
	buf := make([]byte, 0xffff)

	for {
		if p.erred {
			return
		}
		n, err := src.Read(buf)
		if err != nil {
			p.signalError("Read failed", err)
			return
		}
		data := buf[:n]

		p.log.Debug(fmt.Sprintf("%s %d bytes", direction, n))
		if p.outputHex {
			p.log.Debug(hex.Dump(data))
		}

		n, err = dst.Write(data)
		if err != nil {
			p.signalError("Write failed", err)
			return
		}

		if isSend {
			p.sentBytes += uint64(n)
		} else {
			p.receivedBytes += uint64(n)
		}
	}
}

// LocalAddr returns the local address of the incoming connection, if available.
func LocalAddr(conn io.ReadWriteCloser) string {
	if c, ok := conn.(net.Conn); ok {
		return c.RemoteAddr().String()
	}
	return "unknown"
}
