package json

import (
	"errors"
	"regexp"
	"strings"
)

type handledError struct {
	err error
}

func (h *handledError) Error() string {
	if h == nil || h.err == nil {
		return ""
	}
	return h.err.Error()
}

func (h *handledError) Unwrap() error {
	if h == nil {
		return nil
	}
	return h.err
}

func MarkHandled(err error) error {
	if err == nil {
		return nil
	}
	return &handledError{err: err}
}

func IsHandled(err error) bool {
	var handled *handledError
	return errors.As(err, &handled)
}

func StringSliceOrEmpty(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	return append([]string(nil), in...)
}

func IntSliceOrEmpty(in []int) []int {
	if len(in) == 0 {
		return []int{}
	}
	return append([]int(nil), in...)
}

func MapStringStringOrEmpty(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

var (
	legacyBranchPattern  = regexp.MustCompile(`^sophia/cr-(\d+)$`)
	humanBranchV1Pattern = regexp.MustCompile(`^(?:([a-z0-9._-]+)/)?cr-(\d+)(?:-([a-z0-9][a-z0-9-]*))?$`)
	humanBranchV2Pattern = regexp.MustCompile(`^(?:([a-z0-9._-]+)/)?cr-([a-z][a-z0-9-]*)-((?:[a-z0-9]{4}|[a-z0-9]{6}|[a-z0-9]{8}))$`)
)

func BranchIdentityToMap(branch, uid string) map[string]any {
	trimmedBranch := strings.TrimSpace(branch)
	trimmedUID := strings.TrimSpace(uid)
	res := map[string]any{
		"scheme": "custom",
		"uid":    trimmedUID,
		"slug":   "",
		"legacy": false,
	}
	if matches := legacyBranchPattern.FindStringSubmatch(trimmedBranch); len(matches) == 2 {
		res["scheme"] = "legacy_v0"
		res["legacy"] = true
		return res
	}
	if matches := humanBranchV2Pattern.FindStringSubmatch(strings.ToLower(trimmedBranch)); len(matches) == 4 {
		res["scheme"] = "human_alias_v2"
		res["slug"] = strings.TrimSpace(matches[2])
		res["uid_suffix"] = strings.TrimSpace(matches[3])
		if strings.TrimSpace(matches[1]) != "" {
			res["owner_prefix"] = strings.TrimSpace(matches[1])
		}
		return res
	}
	if matches := humanBranchV1Pattern.FindStringSubmatch(strings.ToLower(trimmedBranch)); len(matches) == 4 {
		res["scheme"] = "human_alias_v1"
		if strings.TrimSpace(matches[3]) != "" {
			res["slug"] = strings.TrimSpace(matches[3])
		}
		if strings.TrimSpace(matches[1]) != "" {
			res["owner_prefix"] = strings.TrimSpace(matches[1])
		}
		return res
	}
	return res
}
