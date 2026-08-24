// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"embed"
	"fmt"
	"net/http"
	"path"
	"strings"

	linuxplatform "github.com/gsh20040816/steer/go/internal/platform/linux"
)

//go:embed web/*
var webAssets embed.FS

type webApplication struct {
	WebConfigPath  string
	ConfigPath     string
	RunDirectory   string
	StateDirectory string
	SeedDirectory  string
	Runner         linuxplatform.Runner
}

func serveWeb(listen, webConfigPath, configPath, runDirectory, stateDirectory, seedDirectory string) error {
	if _, err := configuredWebToken(webConfigPath); err != nil {
		return fmt.Errorf("read Web credentials: %w", err)
	}
	app := webApplication{
		WebConfigPath: webConfigPath, ConfigPath: configPath, RunDirectory: runDirectory,
		StateDirectory: stateDirectory, SeedDirectory: seedDirectory,
	}
	return (&http.Server{Addr: listen, Handler: webHandler(app)}).ListenAndServe()
}

func webHandler(app webApplication) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", app.handleIndex)
	mux.HandleFunc("/app.js", app.handleAsset)
	mux.HandleFunc("/style.css", app.handleAsset)
	mux.HandleFunc("/js/", app.handleAsset)
	mux.HandleFunc("/api/v1/config", app.auth(app.handleConfig))
	mux.HandleFunc("/api/v1/runtime", app.auth(app.handleRuntime))
	mux.HandleFunc("/api/v1/overview", app.auth(app.handleOverview))
	mux.HandleFunc("/api/v1/validate", app.auth(app.handleValidate))
	mux.HandleFunc("/api/v1/apply", app.auth(app.handleApply))
	mux.HandleFunc("/api/v1/nodes/import", app.auth(app.handleNodeImport))
	mux.HandleFunc("/api/v1/probes/", app.auth(app.handleProbes))
	mux.HandleFunc("/api/v1/subscriptions", app.auth(app.handleSubscriptions))
	mux.HandleFunc("/api/v1/subscriptions/", app.auth(app.handleSubscriptions))
	mux.HandleFunc("/api/v1/geodata/", app.auth(app.handleGeoData))
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(writer, request)
	})
}

func (app webApplication) seedDirectory() string {
	if app.SeedDirectory == "" {
		return "/usr/share/steer/geodata-seed"
	}
	return app.SeedDirectory
}

func (app webApplication) handleIndex(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path != "/" {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	content, err := webAssets.ReadFile("web/index.html")
	if err != nil {
		http.Error(writer, "web asset is unavailable", http.StatusInternalServerError)
		return
	}
	_, _ = writer.Write(content)
}

func (app webApplication) handleAsset(writer http.ResponseWriter, request *http.Request) {
	cleanPath := path.Clean(request.URL.Path)
	asset := "web" + cleanPath
	allowed := cleanPath == "/app.js" || cleanPath == "/style.css" || (strings.HasPrefix(cleanPath, "/js/") && strings.HasSuffix(cleanPath, ".js"))
	if !allowed {
		http.NotFound(writer, request)
		return
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	content, err := webAssets.ReadFile(asset)
	if err != nil {
		http.Error(writer, "web asset is unavailable", http.StatusInternalServerError)
		return
	}
	if strings.HasSuffix(asset, ".js") {
		writer.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	} else {
		writer.Header().Set("Content-Type", "text/css; charset=utf-8")
	}
	if request.Method == http.MethodGet {
		_, _ = writer.Write(content)
	}
}
