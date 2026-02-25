package diff

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

type Summary struct {
	Files           []string
	ShortStat       string
	NewFiles        []string
	ModifiedFiles   []string
	DeletedFiles    []string
	TestFiles       []string
	DependencyFiles []string
}

func BuildSummaryFromChanges(
	changes []gitx.FileChange,
	shortStat string,
	isTestFile func(string) bool,
	isDependencyFile func(string) bool,
) *Summary {
	files := make([]string, 0, len(changes))
	newFiles := []string{}
	modifiedFiles := []string{}
	deletedFiles := []string{}
	testFiles := []string{}
	depFiles := []string{}
	seenTest := map[string]struct{}{}
	seenDep := map[string]struct{}{}

	for _, change := range changes {
		changePath := strings.TrimSpace(change.Path)
		if changePath == "" {
			continue
		}
		files = append(files, changePath)
		switch change.Status {
		case "A":
			newFiles = append(newFiles, changePath)
		case "D":
			deletedFiles = append(deletedFiles, changePath)
		default:
			modifiedFiles = append(modifiedFiles, changePath)
		}
		if isTestFile != nil && isTestFile(changePath) {
			if _, ok := seenTest[changePath]; !ok {
				seenTest[changePath] = struct{}{}
				testFiles = append(testFiles, changePath)
			}
		}
		if isDependencyFile != nil && isDependencyFile(changePath) {
			if _, ok := seenDep[changePath]; !ok {
				seenDep[changePath] = struct{}{}
				depFiles = append(depFiles, changePath)
			}
		}
	}

	sort.Strings(files)
	sort.Strings(newFiles)
	sort.Strings(modifiedFiles)
	sort.Strings(deletedFiles)
	sort.Strings(testFiles)
	sort.Strings(depFiles)

	return &Summary{
		Files:           files,
		ShortStat:       shortStat,
		NewFiles:        newFiles,
		ModifiedFiles:   modifiedFiles,
		DeletedFiles:    deletedFiles,
		TestFiles:       testFiles,
		DependencyFiles: depFiles,
	}
}

func ResolveCRBaseAnchor(cr *model.CR, resolveRef func(string) (string, error)) (string, error) {
	if cr == nil {
		return "", errors.New("cr is required")
	}
	if resolveRef == nil {
		return "", errors.New("resolveRef is required")
	}
	resolveCommitish := func(ref string) (string, error) {
		trimmed := strings.TrimSpace(ref)
		if trimmed == "" {
			return "", fmt.Errorf("empty ref")
		}
		return resolveRef(trimmed + "^{commit}")
	}

	failures := []string{}
	baseCommit := strings.TrimSpace(cr.BaseCommit)
	if baseCommit != "" {
		resolved, err := resolveCommitish(baseCommit)
		if err == nil && strings.TrimSpace(resolved) != "" {
			return strings.TrimSpace(resolved), nil
		}
		failures = append(failures, fmt.Sprintf("resolve base commit %q", baseCommit))
	}
	baseRef := strings.TrimSpace(cr.BaseRef)
	if baseRef != "" {
		resolved, err := resolveCommitish(baseRef)
		if err != nil {
			failures = append(failures, fmt.Sprintf("resolve base ref %q", baseRef))
		} else {
			return strings.TrimSpace(resolved), nil
		}
	}
	baseBranch := strings.TrimSpace(cr.BaseBranch)
	if baseBranch != "" && baseBranch != baseRef {
		resolved, err := resolveCommitish(baseBranch)
		if err != nil {
			failures = append(failures, fmt.Sprintf("resolve base branch %q", baseBranch))
		} else {
			return strings.TrimSpace(resolved), nil
		}
	}

	if len(failures) > 0 {
		return "", fmt.Errorf("unable to resolve CR %d base anchor locally (%s)", cr.ID, strings.Join(failures, "; "))
	}
	return "", errors.New("cr has no base anchor")
}
