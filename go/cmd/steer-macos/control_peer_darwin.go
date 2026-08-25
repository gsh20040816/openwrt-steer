// SPDX-License-Identifier: GPL-3.0-or-later

//go:build darwin

package main

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

func peerCredentials(connection *net.UnixConn) (uint32, []uint32, error) {
	raw, err := connection.SyscallConn()
	if err != nil {
		return 0, nil, err
	}
	var credentials *unix.Xucred
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		credentials, controlErr = unix.GetsockoptXucred(int(fd), unix.SOL_LOCAL, unix.LOCAL_PEERCRED)
	}); err != nil {
		return 0, nil, err
	}
	if controlErr != nil {
		return 0, nil, fmt.Errorf("read control peer credentials: %w", controlErr)
	}
	if credentials == nil {
		return 0, nil, fmt.Errorf("invalid control peer credentials")
	}
	groupCount := int(credentials.Ngroups)
	if groupCount < 0 || groupCount > len(credentials.Groups) {
		return 0, nil, fmt.Errorf("invalid control peer group count")
	}
	groups := append([]uint32(nil), credentials.Groups[:groupCount]...)
	return credentials.Uid, groups, nil
}
