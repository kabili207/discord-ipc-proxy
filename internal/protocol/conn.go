package protocol

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
)

// FramedConn wraps an io.ReadWriteCloser with the framing protocol.
// Writes are sent as TypeData frames. Reads unframe TypeData frames.
// Control frames received during reads are dispatched to the OnControl
// callback if set, otherwise discarded.
type FramedConn struct {
	inner     io.ReadWriteCloser
	readBuf   []byte // leftover payload from a partially-consumed data frame
	writeMu   sync.Mutex
	OnControl func(Control)
}

// NewFramedConn wraps a connection with framing.
func NewFramedConn(conn io.ReadWriteCloser) *FramedConn {
	return &FramedConn{inner: conn}
}

// Read returns data from the next TypeData frame(s). Control frames are
// dispatched to OnControl and then the next frame is read.
func (fc *FramedConn) Read(p []byte) (int, error) {
	// Drain any leftover from a previous frame first.
	if len(fc.readBuf) > 0 {
		n := copy(p, fc.readBuf)
		fc.readBuf = fc.readBuf[n:]
		return n, nil
	}

	for {
		msgType, payload, err := ReadFrame(fc.inner)
		if err != nil {
			return 0, err
		}

		switch msgType {
		case TypeData:
			n := copy(p, payload)
			if n < len(payload) {
				fc.readBuf = payload[n:]
			}
			return n, nil

		case TypeControl:
			if fc.OnControl != nil {
				var ctrl Control
				if err := json.Unmarshal(payload, &ctrl); err == nil {
					fc.OnControl(ctrl)
				}
			}
			// Loop to read the next frame.

		default:
			return 0, fmt.Errorf("unknown frame type: 0x%02x", msgType)
		}
	}
}

// Write sends p as a TypeData frame.
func (fc *FramedConn) Write(p []byte) (int, error) {
	fc.writeMu.Lock()
	defer fc.writeMu.Unlock()

	if err := WriteFrame(fc.inner, TypeData, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

// WriteControl sends a control message as a TypeControl frame.
func (fc *FramedConn) WriteControl(ctrl Control) error {
	data, err := json.Marshal(ctrl)
	if err != nil {
		return err
	}
	fc.writeMu.Lock()
	defer fc.writeMu.Unlock()
	return WriteFrame(fc.inner, TypeControl, data)
}

// Close closes the underlying connection.
func (fc *FramedConn) Close() error {
	return fc.inner.Close()
}
