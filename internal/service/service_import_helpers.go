package service

import (
	"fmt"
	"strings"
)

func normalizeImportMode(raw string) (string, error) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		mode = importModeCreate
	}
	if mode != importModeCreate && mode != importModeReplace && mode != importModeMerge {
		return "", fmt.Errorf("invalid import mode %q (expected create, replace, or merge)", mode)
	}
	return mode, nil
}
