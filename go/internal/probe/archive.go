// SPDX-License-Identifier: GPL-3.0-or-later

package probe

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	maxArchivedReportSize = 1 << 20
	maxArchivedReports    = 100
)

// Diagnostics is the shared, sanitized report-history contract. Platform
// adapters add their own validation, Apply and log facts around this payload.
type Diagnostics struct {
	Reports          []Report `json:"reports"`
	SavedDigest      string   `json:"saved_digest,omitempty"`
	ActiveGeneration string   `json:"active_generation,omitempty"`
	ActiveDigest     string   `json:"active_digest,omitempty"`
	Warnings         []string `json:"warnings"`
}

func FailureReport(scope, objectID, kind string, err error) Report {
	return SanitizeReport(Report{
		Scope: scope, ObjectID: objectID, Kind: kind,
		Error: SafeError(err), TestedAt: time.Now().UTC(), Results: []Result{},
	})
}

// SanitizeReport removes credentials, query values and process diagnostics
// before a report crosses a public UI boundary or is persisted for later UI
// display.
func SanitizeReport(report Report) Report {
	report.Error = SafeError(errors.New(report.Error))
	if report.Results == nil {
		report.Results = []Result{}
	}
	for index := range report.Results {
		report.Results[index].URL = SafeURL(report.Results[index].URL)
		if report.Results[index].Error != "" {
			report.Results[index].Error = SafeError(errors.New(report.Results[index].Error))
		}
	}
	return report
}

func SafeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	parsed.Fragment = ""
	if parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
		parsed.Path = "/REDACTED"
		parsed.RawPath = ""
	}
	if parsed.RawQuery != "" {
		query := parsed.Query()
		for key := range query {
			values := query[key]
			for index := range values {
				values[index] = "REDACTED"
			}
			query[key] = values
		}
		parsed.RawQuery = query.Encode()
	}
	return parsed.String()
}

func SafeError(err error) string {
	if err == nil || strings.TrimSpace(err.Error()) == "" {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(strings.ToLower(err.Error()), "timeout") || strings.Contains(strings.ToLower(err.Error()), "timed out") {
		return "probe timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "probe was cancelled"
	}
	message := strings.ToLower(err.Error())
	if marker := "unexpected http status "; strings.Contains(message, marker) {
		remainder := message[strings.Index(message, marker)+len(marker):]
		fields := strings.Fields(remainder)
		if len(fields) > 0 {
			if status, parseErr := strconv.Atoi(fields[0]); parseErr == nil && status >= 100 && status <= 599 {
				return fmt.Sprintf("probe target returned HTTP %d", status)
			}
		}
	}
	if strings.Contains(message, "x509") || strings.Contains(message, "tls") {
		return "TLS verification failed"
	}
	if strings.Contains(message, "connection refused") {
		return "probe connection was refused"
	}
	if strings.Contains(message, "no such host") {
		return "probe target could not be resolved"
	}
	return "probe failed"
}

func SaveReport(stateDirectory string, report Report) error {
	path, err := reportPath(stateDirectory, report)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create test log directory: %w", err)
	}
	report = SanitizeReport(report)
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("encode test report: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".report-*")
	if err != nil {
		return fmt.Errorf("create temporary test report: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("publish test report: %w", err)
	}
	return nil
}

func ReadDiagnostics(stateDirectory string) Diagnostics {
	root := filepath.Join(normalizeStateDirectory(stateDirectory), "logs", "tests")
	diagnostics := Diagnostics{Reports: []Report{}, Warnings: []string{}}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if path == root && errors.Is(walkErr, fs.ErrNotExist) {
				return nil
			}
			diagnostics.Warnings = append(diagnostics.Warnings, "a saved probe report is unavailable")
			return nil
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			diagnostics.Warnings = append(diagnostics.Warnings, "a saved probe report is invalid")
			return nil
		}
		if info, err := entry.Info(); err != nil || info.Size() > maxArchivedReportSize {
			diagnostics.Warnings = append(diagnostics.Warnings, "a saved probe report is invalid")
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			diagnostics.Warnings = append(diagnostics.Warnings, "a saved probe report is unavailable")
			return nil
		}
		decoder := json.NewDecoder(io.LimitReader(file, maxArchivedReportSize))
		decoder.DisallowUnknownFields()
		var report Report
		decodeErr := decoder.Decode(&report)
		if decodeErr == nil {
			var trailing any
			if err := decoder.Decode(&trailing); err != io.EOF {
				decodeErr = errors.New("probe report contains trailing data")
			}
		}
		closeErr := file.Close()
		if decodeErr != nil || closeErr != nil || !validReportIdentity(report) {
			diagnostics.Warnings = append(diagnostics.Warnings, "a saved probe report is invalid")
			return nil
		}
		diagnostics.Reports = append(diagnostics.Reports, SanitizeReport(report))
		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		diagnostics.Warnings = append(diagnostics.Warnings, "saved probe reports are unavailable")
	}
	sort.Slice(diagnostics.Reports, func(left, right int) bool {
		return diagnostics.Reports[left].TestedAt.After(diagnostics.Reports[right].TestedAt)
	})
	if len(diagnostics.Reports) > maxArchivedReports {
		diagnostics.Reports = diagnostics.Reports[:maxArchivedReports]
	}
	return diagnostics
}

func reportPath(stateDirectory string, report Report) (string, error) {
	if !validReportIdentity(report) {
		return "", errors.New("invalid probe report identity")
	}
	directory := filepath.Join(normalizeStateDirectory(stateDirectory), "logs", "tests", report.Scope)
	if report.ObjectID != "" {
		directory = filepath.Join(directory, report.ObjectID)
	}
	return filepath.Join(directory, report.Kind+".json"), nil
}

func validReportIdentity(report Report) bool {
	if report.Scope != "overview" && report.Scope != "nodes" && report.Scope != "routes" {
		return false
	}
	if report.Kind != "direct" && report.Kind != "proxy" && report.Kind != "speedtest" && report.Kind != "connect" && report.Kind != "download" {
		return false
	}
	if report.Scope == "overview" {
		return report.ObjectID == ""
	}
	return report.ObjectID != "" && filepath.Base(report.ObjectID) == report.ObjectID && report.ObjectID != "." && report.ObjectID != ".."
}

func normalizeStateDirectory(stateDirectory string) string {
	if stateDirectory == "" {
		return "/var/lib/steer"
	}
	return stateDirectory
}
