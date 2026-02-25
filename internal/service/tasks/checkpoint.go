package tasks

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func DedupeStrings(values []string) []string {
	seen := map[string]struct{}{}
	res := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		res = append(res, value)
	}
	return res
}

func NormalizeScopePaths(workDir string, paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, errors.New("empty path")
		}

		slashPath := strings.ReplaceAll(trimmed, "\\", "/")
		if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
			return nil, fmt.Errorf("path %q must be repo-relative", raw)
		}
		if strings.ContainsAny(slashPath, "*?[]{}") {
			return nil, fmt.Errorf("path %q must be exact (no glob patterns)", raw)
		}

		cleaned := path.Clean(slashPath)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("path %q escapes repository root", raw)
		}
		if cleaned != slashPath {
			return nil, fmt.Errorf("path %q must be normalized", raw)
		}

		absPath := filepath.Join(workDir, filepath.FromSlash(cleaned))
		if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
			return nil, fmt.Errorf("path %q is a directory; select files only", raw)
		}
		if _, exists := seen[cleaned]; exists {
			return nil, fmt.Errorf("duplicate path %q", raw)
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	return normalized, nil
}

func NormalizePatchFilePath(workDir, raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", errors.New("patch file path is required")
	}
	patchPath := trimmed
	if !filepath.IsAbs(patchPath) {
		patchPath = filepath.Join(workDir, patchPath)
	}
	info, err := os.Stat(patchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("patch file %q does not exist", raw)
		}
		return "", fmt.Errorf("patch file %q: %v", raw, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("patch file %q is a directory", raw)
	}
	return patchPath, nil
}
