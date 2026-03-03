package collab

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sophia/internal/model"
	"strings"
)

func NoteHash(note string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(note)))
	return hex.EncodeToString(sum[:])
}

func DecodeStringChange(raw *json.RawMessage) (string, bool, error) {
	if raw == nil {
		return "", false, nil
	}
	value, err := DecodeStringRaw(*raw)
	return value, true, err
}

func DecodeStringRaw(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value, nil
}

func DecodeStringSliceChange(raw *json.RawMessage) ([]string, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	values, err := DecodeStringSliceRaw(*raw)
	return values, true, err
}

func DecodeStringSliceRaw(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []string{}, nil
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	return dedupeStrings(values), nil
}

func ReadCRField(cr *model.CR, field string) (any, error) {
	switch strings.TrimSpace(field) {
	case "cr.title":
		return cr.Title, nil
	case "cr.description":
		return cr.Description, nil
	case "cr.status":
		return cr.Status, nil
	case "cr.branch":
		return cr.Branch, nil
	case "cr.base_branch":
		return cr.BaseBranch, nil
	case "cr.base_ref":
		return cr.BaseRef, nil
	case "cr.base_commit":
		return cr.BaseCommit, nil
	case "cr.merged_at":
		return cr.MergedAt, nil
	case "cr.merged_by":
		return cr.MergedBy, nil
	case "cr.merged_commit":
		return cr.MergedCommit, nil
	case "cr.parent_cr_id":
		if cr.ParentCRID == 0 {
			return nil, nil
		}
		return cr.ParentCRID, nil
	default:
		return nil, fmt.Errorf("unsupported set_field field %q", field)
	}
}

func DecodeCRFieldValue(field string, raw *json.RawMessage) (any, bool, error) {
	if raw == nil {
		return nil, false, nil
	}
	switch strings.TrimSpace(field) {
	case "cr.title", "cr.description", "cr.status", "cr.branch", "cr.base_branch", "cr.base_ref", "cr.base_commit", "cr.merged_at", "cr.merged_by", "cr.merged_commit":
		value, err := DecodeStringRaw(*raw)
		return value, true, err
	case "cr.parent_cr_id":
		if len(*raw) == 0 || string(*raw) == "null" {
			return nil, true, nil
		}
		var value int
		if err := json.Unmarshal(*raw, &value); err != nil {
			return nil, true, err
		}
		if value < 0 {
			return nil, true, errors.New("parent_cr_id cannot be negative")
		}
		return value, true, nil
	default:
		return nil, true, fmt.Errorf("unsupported set_field field %q", field)
	}
}

func WriteCRField(cr *model.CR, field string, value any) error {
	switch strings.TrimSpace(field) {
	case "cr.title":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.Title = next
	case "cr.description":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.Description = next
	case "cr.status":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		switch next {
		case "", model.StatusInProgress, model.StatusMerged, model.StatusAbandoned:
			cr.Status = next
		default:
			return fmt.Errorf("set_field %s invalid status %q", field, next)
		}
	case "cr.branch":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.Branch = next
	case "cr.base_branch":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.BaseBranch = next
	case "cr.base_ref":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.BaseRef = next
	case "cr.base_commit":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.BaseCommit = next
	case "cr.merged_at":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.MergedAt = next
	case "cr.merged_by":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.MergedBy = next
	case "cr.merged_commit":
		next, ok := value.(string)
		if !ok {
			return fmt.Errorf("set_field %s expects string", field)
		}
		cr.MergedCommit = next
	case "cr.parent_cr_id":
		if value == nil {
			cr.ParentCRID = 0
			return nil
		}
		next, ok := value.(int)
		if !ok {
			return fmt.Errorf("set_field %s expects integer or null", field)
		}
		cr.ParentCRID = next
	default:
		return fmt.Errorf("unsupported set_field field %q", field)
	}
	return nil
}

func NormalizeHQRemoteAlias(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("hq remote alias cannot be empty")
	}
	return normalized, nil
}

func NormalizeHQRepoID(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("hq repo id cannot be empty")
	}
	return normalized, nil
}

func NormalizeHQBaseURL(raw string) (string, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return "", fmt.Errorf("hq base url cannot be empty")
	}
	parsed, err := url.Parse(normalized)
	if err != nil || !parsed.IsAbs() {
		return "", fmt.Errorf("invalid hq base url %q", raw)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("invalid hq base url %q: scheme must be http or https", raw)
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}

func dedupeStrings(values []string) []string {
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
