// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	coreapply "github.com/gsh20040816/steer/go/internal/apply"
)

// Status is intentionally minimal and shared by future platform commands.
// Detailed diagnostics belong in platform logs, not in the public contract.
type Status struct {
	Healthy   bool              `json:"healthy"`
	LastApply *coreapply.Record `json:"last_apply,omitempty"`
}

func ReadStatus(ctx context.Context, runner Runner, runDirectory, nftBinary string) Status {
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	if nftBinary == "" {
		nftBinary = "/usr/sbin/nft"
	}
	status := Status{}
	if file, err := os.Open(filepath.Join(runDirectory, "last-apply.json")); err == nil {
		var record coreapply.Record
		if json.NewDecoder(file).Decode(&record) == nil && record.Sequence != "" {
			status.LastApply = &record
		}
		file.Close()
	}
	plan, err := readCurrentPlan(runDirectory)
	if err == nil && checkHealthOnce(ctx, runner, plan, checkListenerPorts, nftBinary) == nil {
		status.Healthy = true
	}
	return status
}
