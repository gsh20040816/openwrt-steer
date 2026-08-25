// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !darwin

package main

import (
	"errors"
	"net"
)

func peerCredentials(*net.UnixConn) (uint32, []uint32, error) {
	return 0, nil, errors.New("Steer control peer credentials require macOS")
}
