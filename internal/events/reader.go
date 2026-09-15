package events

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

const startLineSize = 64 * 1024 // 64kb
const maxLineSize = 1024 * 1024 // 1mb

type EventReader struct {
	sc   *bufio.Scanner
	line int
}

func NewEventReader(r io.Reader) *EventReader {
	buf := bufio.NewScanner(r)
	initBuff := make([]byte, startLineSize)
	buf.Buffer(initBuff, maxLineSize)
	return &EventReader{
		sc: buf,
	}
}

func readHelper(data []byte) (Event, error) {
	var probe struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("probe event: %w", err)
	}

	var ev Event
	switch probe.Type {
	case TypeRunHeader:
		var e RunHeaderEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal %s event: %w", probe.Type, err)
		}
		ev = &e
	case TypeExchange:
		var e ExchangeEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal %s event: %w", probe.Type, err)
		}
		ev = &e
	case TypeBeliefUpdated:
		var e BeliefUpdatedEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal %s event: %w", probe.Type, err)
		}
		ev = &e
	case TypeCallFailed:
		var e CallFailedEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal %s event: %w", probe.Type, err)
		}
		ev = &e
	case TypeRoundSummary:
		var e RoundSummaryEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("unmarshal %s event: %w", probe.Type, err)
		}
		ev = &e
	default:
		return nil, fmt.Errorf("unknown event type %q", probe.Type)
	}
	return ev, nil
}

func (er *EventReader) Next() (Event, error) {
	// note that on error Scan() will skip and continue
	if ok := er.sc.Scan(); !ok {
		if err := er.sc.Err(); err != nil {
			return nil, fmt.Errorf("event reader: %w", err)
		}
		return nil, io.EOF
	}
	er.line++
	ev, err := readHelper(er.sc.Bytes())
	if err != nil {
		return nil, fmt.Errorf("line %d: %w", er.line, err)
	}
	return ev, nil
}
