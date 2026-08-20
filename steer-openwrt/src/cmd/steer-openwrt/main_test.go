// SPDX-License-Identifier: GPL-3.0-or-later
package main

import (
	"testing"
	"time"
)

func TestApplyLockSerializesTransactions(t *testing.T) {
	runDirectory := t.TempDir()
	first, err := acquireApplyLock(runDirectory)
	if err != nil {
		t.Fatal(err)
	}
	acquired := make(chan error, 1)
	release := make(chan struct{})
	go func() {
		second, err := acquireApplyLock(runDirectory)
		acquired <- err
		if err == nil {
			<-release
			second.Close()
		}
	}()
	select {
	case err := <-acquired:
		t.Fatalf("second Apply did not wait for the first lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-acquired:
		if err != nil {
			t.Fatal(err)
		}
		close(release)
	case <-time.After(time.Second):
		t.Fatal("second Apply did not acquire the released lock")
	}
}
