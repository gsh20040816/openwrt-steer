// SPDX-License-Identifier: GPL-3.0-or-later
package openwrt

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/compiler"
	"github.com/gsh20040816/openwrt-steer/steer-openwrt/internal/model"
)

type Status struct {
	DesiredEnabled    bool             `json:"desired_enabled"`
	Healthy           bool             `json:"healthy"`
	CoreRunning       bool             `json:"core_running"`
	CorePID           int              `json:"core_pid,omitempty"`
	TunReady          bool             `json:"tun_ready"`
	FirewallReady     bool             `json:"firewall_ready"`
	ListenersReady    bool             `json:"listeners_ready"`
	CurrentGeneration string           `json:"current_generation,omitempty"`
	IntentDigest      string           `json:"intent_digest,omitempty"`
	CandidateDigest   string           `json:"candidate_digest,omitempty"`
	RollbackAvailable bool             `json:"rollback_available"`
	LastApply         *ApplyRecord     `json:"last_apply,omitempty"`
	Validation        model.Validation `json:"validation"`
}

type ApplyRecord struct {
	Sequence string      `json:"sequence"`
	Result   ApplyResult `json:"result"`
}

func ReadStatus(ctx context.Context, runner Runner, configPath, runDirectory, nftBinary string) Status {
	return ReadStatusWithBackup(ctx, runner, configPath, runDirectory, nftBinary, "/var/lib/steer/rollback.uci")
}

func ReadStatusWithBackup(ctx context.Context, runner Runner, configPath, runDirectory, nftBinary, backupPath string) Status {
	if configPath == "" {
		configPath = "/etc/config/steer"
	}
	if runDirectory == "" {
		runDirectory = "/run/steer"
	}
	if nftBinary == "" {
		nftBinary = "/usr/sbin/nft"
	}
	status := Status{Validation: model.Validation{Errors: []model.Issue{}, Warnings: []model.Issue{}}}
	if _, err := os.Stat(backupPath); err == nil {
		status.RollbackAvailable = true
	}
	if file, err := os.Open(filepath.Join(runDirectory, "last-apply.json")); err == nil {
		var record ApplyRecord
		if json.NewDecoder(file).Decode(&record) == nil && record.Sequence != "" {
			status.LastApply = &record
		}
		file.Close()
	}
	currentLink := filepath.Join(runDirectory, "current")
	if target, err := os.Readlink(currentLink); err == nil {
		status.CurrentGeneration = target
	}
	var current compiler.Bundle
	currentLoaded := false
	if file, err := os.Open(filepath.Join(currentLink, "bundle.json")); err == nil {
		if json.NewDecoder(file).Decode(&current) == nil {
			status.IntentDigest = current.IntentDigest
			currentLoaded = true
		}
		file.Close()
	}

	var desiredDigest string
	config, err := os.ReadFile(configPath)
	if err != nil {
		status.Validation.Errors = append(status.Validation.Errors, model.Issue{Code: "READ_FAILED", ObjectType: "uci", Message: err.Error()})
	} else if intent, decodeErr := DecodeBytes(config); decodeErr != nil {
		status.Validation.Errors = append(status.Validation.Errors, model.Issue{Code: "DECODE_FAILED", ObjectType: "uci", Message: decodeErr.Error()})
	} else {
		status.DesiredEnabled = intent.Main.Enabled
		bundle := compiler.Compile(intent)
		status.Validation = bundle.Validation
		desiredDigest = bundle.IntentDigest
		status.CandidateDigest = bundle.IntentDigest
	}
	serviceOutput, err := runner.Output(ctx, "ubus", "call", "service", "list", `{"name":"steer"}`)
	if err == nil {
		var services map[string]struct {
			Instances map[string]struct {
				Running bool `json:"running"`
				PID     int  `json:"pid"`
			} `json:"instances"`
		}
		if json.Unmarshal(serviceOutput, &services) == nil {
			instance := services["steer"].Instances["sing-box"]
			status.CoreRunning, status.CorePID = instance.Running && instance.PID > 0, instance.PID
		}
	}
	if _, err := runner.Output(ctx, "ip", "-json", "link", "show", "dev", compiler.TunInterface); err == nil {
		status.TunReady = true
	}
	if _, err := runner.Output(ctx, nftBinary, "-j", "list", "table", "inet", "steer"); err == nil {
		status.FirewallReady = true
	}
	if currentLoaded {
		ports := []int{current.Plan.Resources.DNSPort}
		for _, binding := range current.Plan.Resources.MACBindings {
			ports = append(ports, binding.TProxyPort, binding.DNSPort)
		}
		status.ListenersReady = checkListenerPorts(ports) == nil
	}
	status.Healthy = status.DesiredEnabled && status.Validation.OK && status.CoreRunning && status.TunReady && status.FirewallReady && status.ListenersReady && status.IntentDigest == desiredDigest
	return status
}

func GeoCatalog(ctx context.Context, runner Runner, kind, seedDirectory, geoViewBinary string) ([]string, error) {
	if kind != "geosite" && kind != "geoip" {
		return nil, fmt.Errorf("Geo catalog kind must be geosite or geoip")
	}
	if seedDirectory == "" {
		seedDirectory = "/usr/share/steer/geodata-seed"
	}
	if geoViewBinary == "" {
		geoViewBinary = "/usr/bin/geoview"
	}
	output, err := runner.Output(ctx, geoViewBinary, "-action", "extract", "-input", filepath.Join(seedDirectory, kind+".dat"), "-type", kind)
	if err != nil {
		return nil, err
	}
	values := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" && line != "Available codes:" {
			values = append(values, line)
		}
	}
	sort.Strings(values)
	return values, nil
}
