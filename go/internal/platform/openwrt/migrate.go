// SPDX-License-Identifier: GPL-3.0-or-later

package openwrt

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"

	model "github.com/gsh20040816/steer/go/internal/intent"
	"github.com/gsh20040816/steer/go/internal/platform/openwrt/uci"
)

func MigrateSchema8(ctx context.Context, configPath string) (bool, error) {
	return MigrateSchema8WithWriter(ctx, configPath, SystemUCIWriter(configPath))
}

func MigrateSchema8WithWriter(ctx context.Context, configPath string, writer UCIWriter) (bool, error) {
	if writer == nil {
		return false, fmt.Errorf("schema migration requires a UCI writer")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return false, fmt.Errorf("read UCI for schema migration: %w", err)
	}
	document, err := uci.Parse(bytes.NewReader(content))
	if err != nil {
		return false, err
	}
	mainIndex := -1
	for index := range document.Sections {
		if document.Sections[index].Type == "steer" {
			if mainIndex != -1 {
				return false, fmt.Errorf("configuration requires exactly one steer section")
			}
			mainIndex = index
		}
	}
	if mainIndex == -1 {
		return false, fmt.Errorf("configuration requires exactly one steer section")
	}
	schema := document.Sections[mainIndex].Options["schema_version"]
	switch schema {
	case fmt.Sprint(model.SchemaVersion):
		if _, err := decodeDocument(document); err != nil {
			return false, err
		}
		return false, nil
	case "8":
	default:
		return false, fmt.Errorf("cannot migrate UCI schema %q", schema)
	}

	migrated := cloneDocument(document)
	migrated.Sections[mainIndex].Options["schema_version"] = fmt.Sprint(model.SchemaVersion)
	var batch strings.Builder
	fmt.Fprintf(&batch, "set steer.%s.schema_version='%d'\n", migrated.Sections[mainIndex].ID, model.SchemaVersion)
	for index := range migrated.Sections {
		section := &migrated.Sections[index]
		if section.Type != "dns_profile" {
			continue
		}
		if _, exists := section.Options["strategy"]; exists {
			delete(section.Options, "strategy")
			fmt.Fprintf(&batch, "delete steer.%s.strategy\n", section.ID)
		}
	}
	value, err := decodeDocument(migrated)
	if err != nil {
		return false, fmt.Errorf("validate migrated UCI: %w", err)
	}
	validation := model.Validate(value)
	if !validation.OK {
		return false, ValidationError{Validation: validation}
	}
	batch.WriteString("commit steer\n")
	if err := writer(ctx, batch.String()); err != nil {
		return false, err
	}
	return true, nil
}

func cloneDocument(source uci.Document) uci.Document {
	result := uci.Document{Sections: make([]uci.Section, 0, len(source.Sections))}
	for _, section := range source.Sections {
		copySection := uci.Section{
			Type: section.Type, ID: section.ID, Line: section.Line,
			Options: make(map[string]string, len(section.Options)), Lists: make(map[string][]string, len(section.Lists)),
		}
		for key, value := range section.Options {
			copySection.Options[key] = value
		}
		for key, values := range section.Lists {
			copySection.Lists[key] = append([]string(nil), values...)
		}
		result.Sections = append(result.Sections, copySection)
	}
	return result
}
