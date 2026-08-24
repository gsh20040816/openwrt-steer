// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net/http"
	"strings"

	"github.com/gsh20040816/steer/go/internal/capability"
	"github.com/gsh20040816/steer/go/internal/geodata"
	model "github.com/gsh20040816/steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

type runtimeTool struct {
	Version string   `json:"version,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type runtimeInfo struct {
	Steer           string         `json:"steer"`
	SingBox         runtimeTool    `json:"sing_box"`
	GeoData         runtimeGeoData `json:"geodata"`
	CanonicalSchema int            `json:"canonical_schema"`
}

type runtimeGeoData struct {
	Version   string `json:"version,omitempty"`
	RuleCount int    `json:"rule_count,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (app webApplication) handleRuntime(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeWebJSON(writer, app.runtimeInfo(request.Context()))
}

func (app webApplication) runtimeInfo(ctx context.Context) runtimeInfo {
	info := runtimeInfo{
		Steer:           version,
		CanonicalSchema: model.SchemaVersion,
	}
	runner := app.Runner
	if runner == nil {
		runner = linuxplatform.ExecRunner{}
	}

	singBoxOutput, err := runner.Output(ctx, "/usr/bin/sing-box", "version")
	if err != nil {
		info.SingBox.Error = err.Error()
	} else {
		report := capability.Parse(string(singBoxOutput), nil)
		info.SingBox.Version = report.Version
		info.SingBox.Tags = report.Tags
		if len(report.Errors) > 0 {
			info.SingBox.Error = strings.Join(report.Errors, "; ")
		}
	}

	manifest, err := geodata.ReadManifest(app.seedDirectory())
	if err != nil {
		info.GeoData.Error = err.Error()
	} else {
		info.GeoData.Version = manifest.Upstream.Version
		info.GeoData.RuleCount = len(manifest.Rules)
	}
	return info
}
