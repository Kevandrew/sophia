package service

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sophia/internal/model"
	"sort"
	"strconv"
	"strings"
	"time"
)

func commitTiedToCR(subject, body string) bool {
	if crSubjectPattern.MatchString(strings.TrimSpace(subject)) {
		return true
	}
	return crFooterPattern.MatchString(body)
}

func crIDFromSubjectOrBody(subject, body string) (int, bool) {
	if matches := crSubjectPattern.FindStringSubmatch(strings.TrimSpace(subject)); len(matches) >= 2 {
		id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
		if err == nil && id > 0 {
			return id, true
		}
	}
	matches := footerCRIDPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return 0, false
	}
	id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func crUIDFromBody(body string) string {
	matches := footerCRUIDPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func baseRefFromBody(body string) string {
	matches := footerBaseRefPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func baseCommitFromBody(body string) string {
	matches := footerBaseSHApattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func parentCRIDFromBody(body string) int {
	matches := footerParentPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func taskIDFromBody(body string) int {
	matches := footerTaskPattern.FindStringSubmatch(body)
	if len(matches) != 2 {
		return 0
	}
	id, err := strconv.Atoi(strings.TrimSpace(matches[1]))
	if err != nil || id <= 0 {
		return 0
	}
	return id
}

func titleFromSubjectOrBody(subject, body string) string {
	if matches := crSubjectPattern.FindStringSubmatch(strings.TrimSpace(subject)); len(matches) >= 3 {
		title := strings.TrimSpace(matches[2])
		if title != "" {
			return title
		}
	}
	if matches := footerIntentPattern.FindStringSubmatch(body); len(matches) == 2 {
		title := strings.TrimSpace(matches[1])
		if title != "" {
			return title
		}
	}
	return "(unknown)"
}

func sectionFromCommitBody(body, section string) string {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	needle := section + ":\n"
	start := strings.Index(body, needle)
	if start < 0 {
		return ""
	}
	rest := body[start+len(needle):]
	marker := "\n\n"
	if idx := strings.Index(rest, marker); idx >= 0 {
		return strings.TrimSpace(rest[:idx])
	}
	return strings.TrimSpace(rest)
}

func intentFromCommitBody(body string) string {
	section := sectionFromCommitBody(body, "Intent")
	if strings.EqualFold(strings.TrimSpace(section), "(none)") {
		return ""
	}
	return section
}

func notesFromCommitBody(body string) []string {
	section := sectionFromCommitBody(body, "Notes")
	if section == "" || strings.EqualFold(strings.TrimSpace(section), "- (none)") {
		return []string{}
	}
	res := make([]string, 0)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		note := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if note == "" || strings.EqualFold(note, "(none)") {
			continue
		}
		res = append(res, note)
	}
	return res
}

func subtasksFromCommitBody(body, when, actor string) []model.Subtask {
	section := sectionFromCommitBody(body, "Subtasks")
	if section == "" || strings.EqualFold(strings.TrimSpace(section), "- (none)") {
		return []model.Subtask{}
	}
	res := make([]model.Subtask, 0)
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [") {
			continue
		}
		open := strings.HasPrefix(line, "- [ ]")
		done := strings.HasPrefix(line, "- [x]") || strings.HasPrefix(line, "- [X]")
		delegated := strings.HasPrefix(line, "- [~]")
		if !open && !done && !delegated {
			continue
		}
		rest := strings.TrimSpace(line[5:])
		taskID := len(res) + 1
		title := rest
		if strings.HasPrefix(rest, "#") {
			parts := strings.SplitN(rest, " ", 2)
			if len(parts) == 2 {
				if parsed, err := strconv.Atoi(strings.TrimPrefix(parts[0], "#")); err == nil && parsed > 0 {
					taskID = parsed
				}
				title = strings.TrimSpace(parts[1])
			}
		}
		status := model.TaskStatusOpen
		completedAt := ""
		completedBy := ""
		if done {
			status = model.TaskStatusDone
			completedAt = when
			completedBy = actor
		} else if delegated {
			status = model.TaskStatusDelegated
		}
		res = append(res, model.Subtask{
			ID:          taskID,
			Title:       title,
			Status:      status,
			CreatedAt:   when,
			UpdatedAt:   when,
			CompletedAt: completedAt,
			CreatedBy:   actor,
			CompletedBy: completedBy,
		})
	}
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}

func parseCRBranchID(branch string) (int, bool) {
	return parseCRIDFromBranchName(branch)
}

func shortHash(hash string) string {
	hash = strings.TrimSpace(hash)
	if len(hash) > 7 {
		return hash[:7]
	}
	return hash
}

func newCRUID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate cr uid: %w", err)
	}
	// RFC 4122 variant/version bits for compatibility with UUID tooling.
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	return fmt.Sprintf("cr_%x-%x-%x-%x-%x", raw[0:4], raw[4:6], raw[6:8], raw[8:10], raw[10:16]), nil
}

func ensureCRUID(cr *model.CR) (bool, error) {
	if cr == nil {
		return false, errors.New("cr cannot be nil")
	}
	if strings.TrimSpace(cr.UID) != "" {
		return false, nil
	}
	uid, err := newCRUID()
	if err != nil {
		return false, err
	}
	cr.UID = uid
	return true, nil
}

func parseRFC3339OrZero(raw string) time.Time {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(raw))
	if err != nil {
		return time.Time{}
	}
	return t
}
