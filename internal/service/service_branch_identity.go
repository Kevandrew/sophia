package service

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

const crBranchSlugMaxLen = 48

var (
	legacyCRBranchPattern = regexp.MustCompile(`^sophia/cr-(\d+)$`)
	humanCRBranchPattern  = regexp.MustCompile(`^(?:[a-z0-9._-]+/)?cr-(\d+)(?:-[a-z0-9][a-z0-9-]*)?$`)
	ownerPrefixPattern    = regexp.MustCompile(`^[a-z0-9._-]+$`)
)

func legacyCRBranchName(id int) string {
	return fmt.Sprintf("sophia/cr-%d", id)
}

func formatCRBranchAlias(id int, title, ownerPrefix string) (string, error) {
	if id <= 0 {
		return "", fmt.Errorf("invalid CR id %d", id)
	}
	normalizedOwner, err := normalizeCRBranchOwnerPrefix(ownerPrefix)
	if err != nil {
		return "", err
	}
	slug := branchSlugFromTitle(title)
	branch := fmt.Sprintf("cr-%d-%s", id, slug)
	if normalizedOwner != "" {
		return normalizedOwner + "/" + branch, nil
	}
	return branch, nil
}

func normalizeCRBranchOwnerPrefix(raw string) (string, error) {
	trimmed := strings.Trim(strings.ToLower(strings.TrimSpace(raw)), "/")
	if trimmed == "" {
		return "", nil
	}
	if strings.Contains(trimmed, "/") {
		return "", fmt.Errorf("invalid branch owner prefix %q: nested prefixes are not supported", raw)
	}
	if !ownerPrefixPattern.MatchString(trimmed) {
		return "", fmt.Errorf("invalid branch owner prefix %q: expected [a-z0-9._-]+", raw)
	}
	return trimmed, nil
}

func branchSlugFromTitle(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	if lower == "" {
		return "intent"
	}

	var b strings.Builder
	b.Grow(len(lower))
	lastDash := false
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		default:
			if b.Len() == 0 || lastDash {
				continue
			}
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "intent"
	}
	if len(slug) > crBranchSlugMaxLen {
		slug = strings.Trim(slug[:crBranchSlugMaxLen], "-")
	}
	if slug == "" {
		return "intent"
	}
	return slug
}

func parseCRIDFromBranchName(branch string) (int, bool) {
	candidate := strings.TrimSpace(branch)
	if candidate == "" {
		return 0, false
	}
	for _, pattern := range []*regexp.Regexp{legacyCRBranchPattern, humanCRBranchPattern} {
		matches := pattern.FindStringSubmatch(candidate)
		if len(matches) != 2 {
			continue
		}
		id, err := strconv.Atoi(matches[1])
		if err != nil || id <= 0 {
			continue
		}
		return id, true
	}
	return 0, false
}

func validateCRBranchAliasShape(alias string) (string, error) {
	trimmed := strings.TrimSpace(alias)
	if trimmed == "" {
		return "", fmt.Errorf("branch alias cannot be empty")
	}
	for _, r := range trimmed {
		if unicode.IsSpace(r) {
			return "", fmt.Errorf("invalid branch alias %q: whitespace is not allowed", alias)
		}
	}
	if _, ok := parseCRIDFromBranchName(trimmed); !ok {
		return "", fmt.Errorf("invalid branch alias %q: expected cr-<id>-<slug> (optionally owner-prefixed)", alias)
	}
	return trimmed, nil
}

func validateExplicitCRBranchAlias(alias string, expectedID int) (string, error) {
	trimmed, err := validateCRBranchAliasShape(alias)
	if err != nil {
		return "", err
	}
	id, ok := parseCRIDFromBranchName(trimmed)
	if !ok {
		return "", fmt.Errorf("invalid branch alias %q: expected cr-<id>-<slug> (optionally owner-prefixed)", alias)
	}
	if id != expectedID {
		return "", fmt.Errorf("invalid branch alias %q: expected id %d, found %d", alias, expectedID, id)
	}
	return trimmed, nil
}
