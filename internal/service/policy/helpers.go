package policy

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
)

var (
	unknownFieldLinePattern    = regexp.MustCompile(`^line\s+\d+:\s+field\s+(.+)\s+not found in type`)
	unknownFieldCompactPattern = regexp.MustCompile(`^field\s+(.+)\s+not found in type`)
)

func ParseUnknownFields(err error) ([]string, bool) {
	if err == nil {
		return nil, false
	}
	lines := strings.Split(strings.TrimSpace(err.Error()), "\n")
	fields := []string{}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.EqualFold(line, "yaml: unmarshal errors:") {
			continue
		}
		field, ok := ParseUnknownFieldLine(line)
		if !ok {
			return nil, false
		}
		fields = append(fields, field)
	}
	if len(fields) == 0 {
		return nil, false
	}
	sort.Strings(fields)
	return slices.Compact(fields), true
}

func ParseUnknownFieldLine(line string) (string, bool) {
	for _, pattern := range []*regexp.Regexp{unknownFieldLinePattern, unknownFieldCompactPattern} {
		matches := pattern.FindStringSubmatch(line)
		if len(matches) != 2 {
			continue
		}
		field := strings.Trim(strings.TrimSpace(matches[1]), `"'`)
		if field == "" {
			return "", false
		}
		return field, true
	}
	return "", false
}

func UnknownFieldWarnings(fields []string) []string {
	warnings := make([]string, 0, len(fields))
	for _, field := range fields {
		warnings = append(warnings, fmt.Sprintf("SOPHIA.yaml contains unknown field %q; field is ignored for forward compatibility", field))
	}
	return warnings
}

func NormalizeArchivePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	slashPath := strings.ReplaceAll(trimmed, "\\", "/")
	if slashPath == "" {
		return "", errors.New("archive.path cannot be empty")
	}
	if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
		return "", fmt.Errorf("archive.path %q must be repo-relative", raw)
	}
	if strings.ContainsAny(slashPath, "*?[]{}") {
		return "", fmt.Errorf("archive.path %q must be an exact path (no glob patterns)", raw)
	}
	cleaned := path.Clean(slashPath)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("archive.path %q escapes repository root", raw)
	}
	if cleaned != slashPath {
		return "", fmt.Errorf("archive.path %q must be normalized", raw)
	}
	return cleaned, nil
}

func NormalizeStringList(values []string, lower bool) []string {
	normalized := []string{}
	seen := map[string]struct{}{}
	for _, raw := range values {
		candidate := strings.TrimSpace(raw)
		if lower {
			candidate = strings.ToLower(candidate)
		}
		if candidate == "" {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		normalized = append(normalized, candidate)
	}
	sort.Strings(normalized)
	return normalized
}

func NormalizeIntList(values []int) []int {
	if len(values) == 0 {
		return []int{}
	}
	seen := map[int]struct{}{}
	normalized := make([]int, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	sort.Ints(normalized)
	return normalized
}
