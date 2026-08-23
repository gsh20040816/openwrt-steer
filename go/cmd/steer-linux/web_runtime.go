// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/gsh20040816/steer/go/internal/capability"
	model "github.com/gsh20040816/steer/go/internal/intent"
	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

type runtimeTool struct {
	Version string   `json:"version,omitempty"`
	Tags    []string `json:"tags,omitempty"`
	Error   string   `json:"error,omitempty"`
}

type runtimeInfo struct {
	Steer           string      `json:"steer"`
	SingBox         runtimeTool `json:"sing_box"`
	GeoView         runtimeTool `json:"geoview"`
	CanonicalSchema int         `json:"canonical_schema"`
	PlatformSchema  int         `json:"platform_schema"`
}

var geoviewVersionPattern = regexp.MustCompile(`(?mi)^\s*Geoview\s+v?([0-9]+\.[0-9]+\.[0-9]+)\s*$`)

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
		PlatformSchema:  linuxplatform.PlatformSchemaVersion,
	}
	runner := app.GeoRunner
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

	geoViewOutput, err := runner.Output(ctx, "/usr/bin/geoview", "--version")
	if err != nil {
		info.GeoView.Error = err.Error()
	} else if match := geoviewVersionPattern.FindStringSubmatch(string(geoViewOutput)); len(match) == 2 {
		info.GeoView.Version = match[1]
	} else {
		info.GeoView.Error = "cannot parse geoview version output"
	}
	return info
}
