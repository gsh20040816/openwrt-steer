// SPDX-License-Identifier: GPL-3.0-or-later

// Package uci parses the deliberately small, strict subset of UCI used by
// Steer. The OpenWrt adapter owns this representation; the semantic core never
// depends on UCI section shapes.
package uci

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type Section struct {
	Type    string
	ID      string
	Options map[string]string
	Lists   map[string][]string
	Line    int
}

type Document struct {
	Sections []Section
}

func Parse(r io.Reader) (Document, error) {
	return parse(r, true)
}

// ParseSystemConfig accepts anonymous sections used by OpenWrt-owned files.
// Steer's own intent continues to use Parse and therefore requires stable IDs.
func ParseSystemConfig(r io.Reader) (Document, error) {
	return parse(r, false)
}

func parse(r io.Reader, requireSectionID bool) (Document, error) {
	var document Document
	var current *Section
	scanner := bufio.NewScanner(r)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		tokens, err := splitLine(scanner.Text())
		if err != nil {
			return Document{}, fmt.Errorf("UCI line %d: %w", lineNumber, err)
		}
		if len(tokens) == 0 {
			continue
		}
		switch tokens[0] {
		case "config":
			if len(tokens) < 2 || len(tokens) > 3 || tokens[1] == "" {
				return Document{}, fmt.Errorf("UCI line %d: config requires a type and optional section ID", lineNumber)
			}
			if requireSectionID && (len(tokens) != 3 || tokens[2] == "") {
				return Document{}, fmt.Errorf("UCI line %d: config requires a type and an explicit section ID", lineNumber)
			}
			id := ""
			if len(tokens) == 3 {
				id = tokens[2]
			}
			document.Sections = append(document.Sections, Section{
				Type: tokens[1], ID: id, Line: lineNumber,
				Options: make(map[string]string), Lists: make(map[string][]string),
			})
			current = &document.Sections[len(document.Sections)-1]
		case "option", "list":
			if current == nil {
				return Document{}, fmt.Errorf("UCI line %d: %s appears before config", lineNumber, tokens[0])
			}
			if len(tokens) != 3 || tokens[1] == "" {
				return Document{}, fmt.Errorf("UCI line %d: %s requires a key and value", lineNumber, tokens[0])
			}
			key, value := tokens[1], tokens[2]
			if tokens[0] == "option" {
				if _, exists := current.Options[key]; exists {
					return Document{}, fmt.Errorf("UCI line %d: duplicate option %q in %s %q", lineNumber, key, current.Type, current.ID)
				}
				if _, exists := current.Lists[key]; exists {
					return Document{}, fmt.Errorf("UCI line %d: %q cannot be both option and list", lineNumber, key)
				}
				current.Options[key] = value
			} else {
				if _, exists := current.Options[key]; exists {
					return Document{}, fmt.Errorf("UCI line %d: %q cannot be both option and list", lineNumber, key)
				}
				current.Lists[key] = append(current.Lists[key], value)
			}
		default:
			return Document{}, fmt.Errorf("UCI line %d: unsupported directive %q", lineNumber, tokens[0])
		}
	}
	if err := scanner.Err(); err != nil {
		return Document{}, fmt.Errorf("read UCI: %w", err)
	}
	return document, nil
}

func splitLine(line string) ([]string, error) {
	var tokens []string
	var token strings.Builder
	inSingle, inDouble, escaped, started := false, false, false, false
	flush := func() {
		if started {
			tokens = append(tokens, token.String())
			token.Reset()
			started = false
		}
	}
	for _, r := range line {
		if escaped {
			token.WriteRune(r)
			started, escaped = true, false
			continue
		}
		if inSingle {
			if r == '\'' {
				inSingle = false
			} else {
				token.WriteRune(r)
			}
			started = true
			continue
		}
		if inDouble {
			switch r {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
			default:
				token.WriteRune(r)
			}
			started = true
			continue
		}
		switch {
		case r == '#':
			flush()
			return tokens, nil
		case unicode.IsSpace(r):
			flush()
		case r == '\'':
			inSingle, started = true, true
		case r == '"':
			inDouble, started = true, true
		case r == '\\':
			escaped, started = true, true
		default:
			token.WriteRune(r)
			started = true
		}
	}
	if escaped || inSingle || inDouble {
		return nil, fmt.Errorf("unterminated quoted or escaped value")
	}
	flush()
	return tokens, nil
}
