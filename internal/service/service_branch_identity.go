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
	legacyCRBranchPattern   = regexp.MustCompile(`^sophia/cr-(\d+)$`)
	humanCRBranchV1Pattern  = regexp.MustCompile(`^(?:[a-z0-9._-]+/)?cr-(\d+)(?:-[a-z0-9][a-z0-9-]*)?$`)
	humanCRBranchV2Pattern  = regexp.MustCompile(`^(?:[a-z0-9._-]+/)?cr-[a-z][a-z0-9-]*-(?:[a-z0-9]{4}|[a-z0-9]{6}|[a-z0-9]{8})$`)
	ownerPrefixPattern      = regexp.MustCompile(`^[a-z0-9._-]+$`)
	uidTokenPattern         = regexp.MustCompile(`^[a-z0-9]+$`)
	uidSuffixLengthFallback = []int{4, 6, 8}
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
	token := previewUIDSuffixFromID(id)
	branch := fmt.Sprintf("cr-%s-%s", slug, token)
	if normalizedOwner != "" {
		return normalizedOwner + "/" + branch, nil
	}
	return branch, nil
}

func formatCRBranchAliasFromUID(title, ownerPrefix, uid string, suffixLen int) (string, error) {
	if suffixLen <= 0 {
		return "", fmt.Errorf("invalid uid suffix length %d", suffixLen)
	}
	normalizedOwner, err := normalizeCRBranchOwnerPrefix(ownerPrefix)
	if err != nil {
		return "", err
	}
	uidToken, err := uidTokenFromCRUID(uid)
	if err != nil {
		return "", err
	}
	if len(uidToken) < suffixLen {
		return "", fmt.Errorf("uid %q is too short for suffix length %d", uid, suffixLen)
	}
	slug := branchSlugFromTitle(title)
	branch := fmt.Sprintf("cr-%s-%s", slug, uidToken[:suffixLen])
	if normalizedOwner != "" {
		return normalizedOwner + "/" + branch, nil
	}
	return branch, nil
}

func formatCRBranchAliasWithFallback(title, ownerPrefix, uid string, branchExists func(string) bool) (string, error) {
	lastCandidate := ""
	for _, length := range uidSuffixLengthFallback {
		candidate, err := formatCRBranchAliasFromUID(title, ownerPrefix, uid, length)
		if err != nil {
			return "", err
		}
		lastCandidate = candidate
		if branchExists == nil || !branchExists(candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("unable to allocate branch alias; all uid suffix lengths collided (last=%q)", lastCandidate)
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

func ownerPrefixFromBranch(branch string) (string, bool) {
	trimmed := strings.TrimSpace(branch)
	if trimmed == "" {
		return "", false
	}
	lower := strings.ToLower(trimmed)
	idx := strings.Index(lower, "/cr-")
	if idx <= 0 {
		return "", false
	}
	prefix := strings.TrimSpace(trimmed[:idx])
	normalized, err := normalizeCRBranchOwnerPrefix(prefix)
	if err != nil || normalized == "" {
		return "", false
	}
	return normalized, true
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
	if len(slug) > 0 && slug[0] >= '0' && slug[0] <= '9' {
		slug = "n-" + slug
		if len(slug) > crBranchSlugMaxLen {
			slug = strings.Trim(slug[:crBranchSlugMaxLen], "-")
		}
	}
	if slug == "" {
		return "intent"
	}
	return slug
}

func uidTokenFromCRUID(uid string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(uid))
	if strings.HasPrefix(trimmed, "cr_") {
		trimmed = strings.TrimPrefix(trimmed, "cr_")
	}
	trimmed = strings.ReplaceAll(trimmed, "-", "")
	if trimmed == "" {
		return "", fmt.Errorf("cr uid cannot be empty")
	}
	if !uidTokenPattern.MatchString(trimmed) {
		return "", fmt.Errorf("invalid cr uid %q: expected lowercase alphanumeric characters", uid)
	}
	return trimmed, nil
}

func normalizeCRUID(uid string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(uid))
	if trimmed == "" {
		return "", fmt.Errorf("cr uid cannot be empty")
	}
	if !strings.HasPrefix(trimmed, "cr_") {
		trimmed = "cr_" + trimmed
	}
	if _, err := uidTokenFromCRUID(trimmed); err != nil {
		return "", err
	}
	return trimmed, nil
}

func previewUIDSuffixFromID(id int) string {
	token := strings.ToLower(strconv.FormatInt(int64(id), 36))
	switch {
	case len(token) >= 4:
		return token[:4]
	case len(token) == 3:
		return "0" + token
	case len(token) == 2:
		return "00" + token
	default:
		return "000" + token
	}
}

func parseCRIDFromBranchName(branch string) (int, bool) {
	candidate := strings.TrimSpace(branch)
	if candidate == "" {
		return 0, false
	}
	for _, pattern := range []*regexp.Regexp{legacyCRBranchPattern, humanCRBranchV1Pattern} {
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

func detectCRBranchScheme(branch string) string {
	trimmed := strings.ToLower(strings.TrimSpace(branch))
	switch {
	case legacyCRBranchPattern.MatchString(trimmed):
		return "legacy_v0"
	case humanCRBranchV2Pattern.MatchString(trimmed):
		return "human_alias_v2"
	case humanCRBranchV1Pattern.MatchString(trimmed):
		return "human_alias_v1"
	default:
		return "custom"
	}
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
	if _, ok := parseCRIDFromBranchName(trimmed); ok {
		return trimmed, nil
	}
	if humanCRBranchV2Pattern.MatchString(strings.ToLower(trimmed)) {
		return trimmed, nil
	}
	return "", fmt.Errorf("invalid branch alias %q: expected cr-<id>-<slug> or cr-<slug>-<uid4|uid6|uid8> (optionally owner-prefixed)", alias)
}

func validateExplicitCRBranchAlias(alias string, expectedID int) (string, error) {
	trimmed, err := validateCRBranchAliasShape(alias)
	if err != nil {
		return "", err
	}
	if humanCRBranchV2Pattern.MatchString(strings.ToLower(trimmed)) {
		return trimmed, nil
	}
	id, ok := parseCRIDFromBranchName(trimmed)
	if !ok {
		return "", fmt.Errorf("invalid branch alias %q", alias)
	}
	if id != expectedID {
		return "", fmt.Errorf("invalid branch alias %q: expected id %d, found %d", alias, expectedID, id)
	}
	return trimmed, nil
}
