// SPDX-License-Identifier: GPL-3.0-or-later
// Package capability validates the installed sing-box runtime shared by all platforms.
package capability

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Report struct {
	OK      bool     `json:"ok"`
	Version string   `json:"version"`
	Tags    []string `json:"tags"`
	Errors  []string `json:"errors"`
}

var versionPattern = regexp.MustCompile(`(?m)^sing-box version ([0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?)`)

func Parse(output string, required []string) Report {
	report := Report{}
	match := versionPattern.FindStringSubmatch(output)
	if len(match) != 2 {
		report.Errors = append(report.Errors, "cannot parse sing-box version output")
		return report
	}
	report.Version = match[1]
	if !supportedVersion(report.Version) {
		report.Errors = append(report.Errors, "sing-box version must be >=1.14.0-beta.2 and <1.15.0")
	}
	for _, line := range strings.Split(output, "\n") {
		if !strings.HasPrefix(line, "Tags:") {
			continue
		}
		for _, tag := range strings.FieldsFunc(strings.TrimSpace(strings.TrimPrefix(line, "Tags:")), func(r rune) bool { return r == ',' || r == ' ' || r == '\t' }) {
			if tag != "" {
				report.Tags = append(report.Tags, tag)
			}
		}
	}
	sort.Strings(report.Tags)
	tagSet := map[string]bool{}
	for _, tag := range report.Tags {
		tagSet[tag] = true
	}
	for _, capability := range required {
		requiredTag := ""
		switch capability {
		case "with_quic", "dns_quic":
			requiredTag = "with_quic"
		case "with_utls":
			requiredTag = "with_utls"
		}
		if requiredTag != "" && !tagSet[requiredTag] {
			report.Errors = append(report.Errors, "sing-box build is missing tag "+requiredTag+" required by current intent")
		}
	}
	report.Errors = unique(report.Errors)
	report.OK = len(report.Errors) == 0
	return report
}

func supportedVersion(value string) bool {
	version, ok := parseVersion(value)
	if !ok || version.major != 1 || version.minor != 14 {
		return false
	}
	minimum, _ := parseVersion("1.14.0-beta.2")
	return compareVersion(version, minimum) >= 0
}

type semanticVersion struct {
	major      int
	minor      int
	patch      int
	prerelease []string
}

func parseVersion(value string) (semanticVersion, bool) {
	core, suffix, hasSuffix := strings.Cut(value, "-")
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return semanticVersion{}, false
	}
	numbers := make([]int, 3)
	for index, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil || number < 0 {
			return semanticVersion{}, false
		}
		numbers[index] = number
	}
	parsed := semanticVersion{major: numbers[0], minor: numbers[1], patch: numbers[2]}
	if hasSuffix {
		if suffix == "" {
			return semanticVersion{}, false
		}
		parsed.prerelease = strings.Split(suffix, ".")
		for _, identifier := range parsed.prerelease {
			if identifier == "" {
				return semanticVersion{}, false
			}
		}
	}
	return parsed, true
}

func compareVersion(left, right semanticVersion) int {
	for _, pair := range [][2]int{{left.major, right.major}, {left.minor, right.minor}, {left.patch, right.patch}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	if len(left.prerelease) == 0 || len(right.prerelease) == 0 {
		if len(left.prerelease) == len(right.prerelease) {
			return 0
		}
		if len(left.prerelease) == 0 {
			return 1
		}
		return -1
	}
	limit := len(left.prerelease)
	if len(right.prerelease) < limit {
		limit = len(right.prerelease)
	}
	for index := 0; index < limit; index++ {
		leftNumber, leftNumeric := numericIdentifier(left.prerelease[index])
		rightNumber, rightNumeric := numericIdentifier(right.prerelease[index])
		switch {
		case leftNumeric && rightNumeric && leftNumber < rightNumber:
			return -1
		case leftNumeric && rightNumeric && leftNumber > rightNumber:
			return 1
		case leftNumeric != rightNumeric:
			if leftNumeric {
				return -1
			}
			return 1
		case left.prerelease[index] < right.prerelease[index]:
			return -1
		case left.prerelease[index] > right.prerelease[index]:
			return 1
		}
	}
	if len(left.prerelease) < len(right.prerelease) {
		return -1
	}
	if len(left.prerelease) > len(right.prerelease) {
		return 1
	}
	return 0
}

func numericIdentifier(value string) (int, bool) {
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	return parsed, err == nil
}

func unique(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
