// SPDX-License-Identifier: GPL-3.0-or-later

package macos

import (
	"bytes"
	"errors"
	"testing"
)

func TestTCPFramerHandlesPartialAndCoalescedReads(t *testing.T) {
	first := bytes.Repeat([]byte{0x01}, 12)
	second := bytes.Repeat([]byte{0x02}, 24)
	firstFrame, err := EncodeTCPMessage(first)
	if err != nil {
		t.Fatal(err)
	}
	secondFrame, err := EncodeTCPMessage(second)
	if err != nil {
		t.Fatal(err)
	}
	framer, err := NewTCPFramer(MaxDNSMessageSize)
	if err != nil {
		t.Fatal(err)
	}
	messages, err := framer.Push(firstFrame[:1])
	if err != nil || len(messages) != 0 || framer.PendingBytes() != 1 {
		t.Fatalf("unexpected partial frame state: %#v %v %d", messages, err, framer.PendingBytes())
	}
	messages, err = framer.Push(append(firstFrame[1:], secondFrame...))
	if err != nil || len(messages) != 2 || !bytes.Equal(messages[0], first) || !bytes.Equal(messages[1], second) {
		t.Fatalf("unexpected framed messages: %#v %v", messages, err)
	}
	if framer.PendingBytes() != 0 {
		t.Fatalf("framer retained complete data: %d", framer.PendingBytes())
	}
}

func TestTCPFramerRejectsMalformedFrames(t *testing.T) {
	framer, err := NewTCPFramer(32)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := framer.Push([]byte{0, 0}); !errors.Is(err, ErrEmptyDNSMessage) {
		t.Fatalf("empty DNS frame was accepted: %v", err)
	}
	framer.Reset()
	if _, err := framer.Push([]byte{0, 33}); !errors.Is(err, ErrDNSFrameTooLarge) {
		t.Fatalf("oversized DNS frame was accepted: %v", err)
	}
}

func TestTCPFramerRetainsHalfHeader(t *testing.T) {
	framer, err := NewTCPFramer(128)
	if err != nil {
		t.Fatal(err)
	}
	if messages, err := framer.Push([]byte{0x00}); err != nil || len(messages) != 0 {
		t.Fatalf("unexpected half-header result: %#v %v", messages, err)
	}
	if _, err := framer.Push([]byte{0x04, 1, 2}); err != nil {
		t.Fatal(err)
	}
	messages, err := framer.Push([]byte{3, 4})
	if err != nil || len(messages) != 1 || !bytes.Equal(messages[0], []byte{1, 2, 3, 4}) {
		t.Fatalf("half-message was not completed: %#v %v", messages, err)
	}
}
