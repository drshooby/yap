package events

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var ErrClosed = errors.New("events: writer is closed")

type EventWriter struct {
	buf    *bufio.Writer
	enc    *json.Encoder
	mu     sync.Mutex
	closed bool
}

func NewEventWriter(w io.Writer) *EventWriter {
	buf := bufio.NewWriter(w)
	return &EventWriter{
		buf: buf,
		enc: json.NewEncoder(buf),
	}
}

func (ew *EventWriter) Close() error {
	ew.mu.Lock()
	defer ew.mu.Unlock()
	// make repeated calls no-op
	if ew.closed {
		return nil
	}
	ew.closed = true
	if err := ew.buf.Flush(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	return nil
}

func (ew *EventWriter) Write(e Event) error {
	ew.mu.Lock()
	defer ew.mu.Unlock()

	if ew.closed {
		return ErrClosed
	}

	if e.headers().Ts.IsZero() {
		e.headers().Ts = time.Now()
	}

	if err := ew.enc.Encode(e); err != nil {
		return fmt.Errorf("write event: %w", err)
	}
	return nil
}
