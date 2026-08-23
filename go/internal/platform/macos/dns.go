// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const MaxDNSMessageSize = 65535

var (
	ErrEmptyDNSMessage    = errors.New("DNS message is empty")
	ErrDNSMessageTooLarge = errors.New("DNS message exceeds the 65535-byte limit")
	ErrDNSFrameTooLarge   = errors.New("DNS TCP frame exceeds the configured limit")
)

func ValidateDNSMessage(message []byte, maximum int) error {
	if len(message) == 0 {
		return ErrEmptyDNSMessage
	}
	if maximum <= 0 || len(message) > maximum {
		return ErrDNSMessageTooLarge
	}
	return nil
}

// TCPFramer handles the two-byte length prefix used by DNS over TCP. Push may
// receive arbitrary partial or coalesced reads and returns every complete
// message while retaining an incomplete tail.
type TCPFramer struct {
	maximum int
	buffer  []byte
}

func NewTCPFramer(maximum int) (*TCPFramer, error) {
	if maximum <= 0 || maximum > MaxDNSMessageSize {
		return nil, fmt.Errorf("invalid DNS TCP frame limit %d", maximum)
	}
	return &TCPFramer{maximum: maximum}, nil
}

func (framer *TCPFramer) Push(data []byte) ([][]byte, error) {
	if len(data) > 0 {
		framer.buffer = append(framer.buffer, data...)
	}
	var messages [][]byte
	for {
		if len(framer.buffer) < 2 {
			break
		}
		length := int(binary.BigEndian.Uint16(framer.buffer[:2]))
		if length == 0 {
			return nil, ErrEmptyDNSMessage
		}
		if length > framer.maximum {
			return nil, ErrDNSFrameTooLarge
		}
		if len(framer.buffer) < 2+length {
			break
		}
		message := append([]byte(nil), framer.buffer[2:2+length]...)
		if err := ValidateDNSMessage(message, framer.maximum); err != nil {
			return nil, err
		}
		messages = append(messages, message)
		framer.buffer = framer.buffer[2+length:]
	}
	if len(framer.buffer) > framer.maximum+2 {
		return nil, ErrDNSFrameTooLarge
	}
	return messages, nil
}

func (framer *TCPFramer) PendingBytes() int {
	return len(framer.buffer)
}

func (framer *TCPFramer) Reset() {
	framer.buffer = nil
}

func EncodeTCPMessage(message []byte) ([]byte, error) {
	if err := ValidateDNSMessage(message, MaxDNSMessageSize); err != nil {
		return nil, err
	}
	encoded := make([]byte, 2+len(message))
	binary.BigEndian.PutUint16(encoded[:2], uint16(len(message)))
	copy(encoded[2:], message)
	return encoded, nil
}
