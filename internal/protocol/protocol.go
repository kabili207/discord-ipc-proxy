package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"runtime"
)

// Version is the current protocol version.
const Version = 1

// Frame header: [type:1][length:4][crc32c:4] = 9 bytes
const headerSize = 9

// Maximum payload size (1 MB).
const maxPayload = 1 << 20

// Message types.
const (
	TypeData    byte = 0x00
	TypeControl byte = 0x01
)

var crc32cTable = crc32.MakeTable(crc32.Castagnoli)

// Handshake is exchanged once at the start of a connection.
type Handshake struct {
	Version  int    `json:"version"`
	OS       string `json:"os"`
	Hostname string `json:"hostname"`

	// Server-only fields
	DiscordAvailable *bool `json:"discord_available,omitempty"`
}

// Control is a JSON-encoded control message sent in TypeControl frames.
type Control struct {
	Status string `json:"status"`           // e.g. "discord_connected", "discord_disconnected"
	Detail string `json:"detail,omitempty"` // optional human-readable detail
}

// LocalHandshake returns a Handshake populated with local system info.
func LocalHandshake() Handshake {
	hostname, _ := os.Hostname()
	return Handshake{
		Version:  Version,
		OS:       runtime.GOOS,
		Hostname: hostname,
	}
}

// WriteHandshake sends a JSON-encoded handshake as a control frame.
func WriteHandshake(w io.Writer, h Handshake) error {
	data, err := json.Marshal(h)
	if err != nil {
		return fmt.Errorf("marshaling handshake: %w", err)
	}
	return WriteFrame(w, TypeControl, data)
}

// ReadHandshake reads and decodes a handshake from a control frame.
func ReadHandshake(r io.Reader) (Handshake, error) {
	msgType, payload, err := ReadFrame(r)
	if err != nil {
		return Handshake{}, fmt.Errorf("reading handshake frame: %w", err)
	}
	if msgType != TypeControl {
		return Handshake{}, fmt.Errorf("expected control frame for handshake, got type 0x%02x", msgType)
	}
	var h Handshake
	if err := json.Unmarshal(payload, &h); err != nil {
		return Handshake{}, fmt.Errorf("decoding handshake: %w", err)
	}
	return h, nil
}

// WriteFrame writes a framed message: [type:1][length:4][crc32c:4][payload].
func WriteFrame(w io.Writer, msgType byte, payload []byte) error {
	if len(payload) > maxPayload {
		return fmt.Errorf("payload too large: %d > %d", len(payload), maxPayload)
	}

	var header [headerSize]byte
	header[0] = msgType
	binary.BigEndian.PutUint32(header[1:5], uint32(len(payload)))
	binary.BigEndian.PutUint32(header[5:9], crc32.Checksum(payload, crc32cTable))

	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := w.Write(payload)
		return err
	}
	return nil
}

// ReadFrame reads a framed message and validates the CRC-32C checksum.
func ReadFrame(r io.Reader) (msgType byte, payload []byte, err error) {
	var header [headerSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return 0, nil, err
	}

	msgType = header[0]
	length := binary.BigEndian.Uint32(header[1:5])
	checksum := binary.BigEndian.Uint32(header[5:9])

	if length > maxPayload {
		return 0, nil, fmt.Errorf("payload too large: %d > %d", length, maxPayload)
	}

	payload = make([]byte, length)
	if length > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return 0, nil, err
		}
	}

	if actual := crc32.Checksum(payload, crc32cTable); actual != checksum {
		return 0, nil, fmt.Errorf("checksum mismatch: expected 0x%08x, got 0x%08x", checksum, actual)
	}

	return msgType, payload, nil
}
