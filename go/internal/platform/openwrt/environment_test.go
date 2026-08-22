// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExecRunnerAppliesCommandDeadline(t *testing.T) {
	started := time.Now()
	_, err := (ExecRunner{Timeout: 20 * time.Millisecond}).Output(context.Background(), "/bin/sh", "-c", "sleep 1")
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out command returned %v", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("command deadline took too long: %v", elapsed)
	}
}
