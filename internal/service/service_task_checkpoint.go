package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

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

func validateDoneTaskOptions(opts DoneTaskOptions) error {
	if !opts.Checkpoint {
		if opts.StageAll || opts.FromContract || len(opts.Paths) > 0 || strings.TrimSpace(opts.PatchFile) != "" {
			return fmt.Errorf("%w: --no-checkpoint cannot be combined with --from-contract, --path, --patch-file, or --all", ErrInvalidTaskScope)
		}
		if strings.TrimSpace(opts.NoCheckpointReason) == "" {
			return fmt.Errorf("%w: --no-checkpoint requires --no-checkpoint-reason", ErrInvalidTaskScope)
		}
		return nil
	}
	if strings.TrimSpace(opts.NoCheckpointReason) != "" {
		return fmt.Errorf("%w: --no-checkpoint-reason requires --no-checkpoint", ErrInvalidTaskScope)
	}
	modes := 0
	if opts.StageAll {
		modes++
	}
	if opts.FromContract {
		modes++
	}
	if len(opts.Paths) > 0 {
		modes++
	}
	if strings.TrimSpace(opts.PatchFile) != "" {
		modes++
	}
	if modes > 1 {
		return fmt.Errorf("%w: exactly one of --all, --from-contract, --path, or --patch-file must be provided", ErrInvalidTaskScope)
	}
	if modes == 0 {
		return ErrTaskScopeRequired
	}
	return nil
}

func (s *Service) resolveTaskCheckpointPathsFromContract(scopePrefixes []string) ([]string, error) {
	statusEntries, err := s.git.WorkingTreeStatus()
	if err != nil {
		return nil, err
	}
	matches := make([]string, 0)
	seen := map[string]struct{}{}
	for _, entry := range statusEntries {
		candidate := strings.TrimSpace(entry.Path)
		if candidate == "" {
			continue
		}
		inScope := false
		for _, prefix := range scopePrefixes {
			if pathMatchesScopePrefix(candidate, prefix) {
				inScope = true
				break
			}
		}
		if !inScope {
			continue
		}
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		matches = append(matches, candidate)
	}
	if len(matches) == 0 {
		return nil, ErrNoTaskScopeMatches
	}
	sort.Strings(matches)
	return matches, nil
}

func (s *Service) normalizeTaskScopePaths(paths []string) ([]string, error) {
	normalized := make([]string, 0, len(paths))
	seen := map[string]struct{}{}
	for _, raw := range paths {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: empty path", ErrInvalidTaskScope)
		}

		slashPath := strings.ReplaceAll(trimmed, "\\", "/")
		if filepath.IsAbs(trimmed) || strings.HasPrefix(slashPath, "/") {
			return nil, fmt.Errorf("%w: path %q must be repo-relative", ErrInvalidTaskScope, raw)
		}
		if strings.ContainsAny(slashPath, "*?[]{}") {
			return nil, fmt.Errorf("%w: path %q must be exact (no glob patterns)", ErrInvalidTaskScope, raw)
		}

		cleaned := path.Clean(slashPath)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			return nil, fmt.Errorf("%w: path %q escapes repository root", ErrInvalidTaskScope, raw)
		}
		if cleaned != slashPath {
			return nil, fmt.Errorf("%w: path %q must be normalized", ErrInvalidTaskScope, raw)
		}

		absPath := filepath.Join(s.git.WorkDir, filepath.FromSlash(cleaned))
		if info, statErr := os.Stat(absPath); statErr == nil && info.IsDir() {
			return nil, fmt.Errorf("%w: path %q is a directory; select files only", ErrInvalidTaskScope, raw)
		}
		if _, exists := seen[cleaned]; exists {
			return nil, fmt.Errorf("%w: duplicate path %q", ErrInvalidTaskScope, raw)
		}
		seen[cleaned] = struct{}{}
		normalized = append(normalized, cleaned)
	}
	return normalized, nil
}

func (s *Service) normalizePatchFilePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("%w: patch file path is required", ErrInvalidTaskScope)
	}
	patchPath := trimmed
	if !filepath.IsAbs(patchPath) {
		patchPath = filepath.Join(s.git.WorkDir, patchPath)
	}
	info, err := os.Stat(patchPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("%w: patch file %q does not exist", ErrInvalidTaskScope, raw)
		}
		return "", fmt.Errorf("%w: patch file %q: %v", ErrInvalidTaskScope, raw, err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("%w: patch file %q is a directory", ErrInvalidTaskScope, raw)
	}
	return patchPath, nil
}

func parsePatchChunks(diff string) ([]parsedPatchChunk, error) {
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	if strings.TrimSpace(diff) == "" {
		return []parsedPatchChunk{}, nil
	}

	lines := strings.Split(diff, "\n")
	chunks := make([]parsedPatchChunk, 0)
	currentPath := ""
	currentHeader := ""
	currentBody := []string{}

	flush := func() error {
		if currentHeader == "" {
			return nil
		}
		if strings.TrimSpace(currentPath) == "" {
			return fmt.Errorf("chunk header %q is missing file path", currentHeader)
		}
		oldStart, oldLines, newStart, newLines, err := parseHunkHeader(currentHeader)
		if err != nil {
			return err
		}
		body := strings.Join(currentBody, "\n")
		chunks = append(chunks, parsedPatchChunk{
			ID:       chunkIDFor(currentPath, currentHeader, body),
			Path:     currentPath,
			OldStart: oldStart,
			OldLines: oldLines,
			NewStart: newStart,
			NewLines: newLines,
			Header:   currentHeader,
			Body:     body,
			Preview:  chunkPreview(currentBody),
		})
		currentHeader = ""
		currentBody = nil
		return nil
	}

	for _, rawLine := range lines {
		line := strings.TrimSuffix(rawLine, "\r")
		switch {
		case strings.HasPrefix(line, "diff --git "):
			if err := flush(); err != nil {
				return nil, err
			}
			currentPath = pathFromDiffHeader(line)
		case strings.HasPrefix(line, "+++ "):
			nextPath := pathFromPatchLine(line)
			if nextPath != "" {
				currentPath = nextPath
			}
		case strings.HasPrefix(line, "@@ "):
			if err := flush(); err != nil {
				return nil, err
			}
			currentHeader = line
			currentBody = []string{}
		default:
			if currentHeader != "" {
				currentBody = append(currentBody, line)
			}
		}
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func pathFromDiffHeader(line string) string {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 4 {
		return ""
	}
	return stripDiffPathPrefix(parts[3])
}

func pathFromPatchLine(line string) string {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) < 2 {
		return ""
	}
	if parts[1] == "/dev/null" {
		return ""
	}
	return stripDiffPathPrefix(parts[1])
}

func stripDiffPathPrefix(raw string) string {
	raw = strings.Trim(raw, "\"")
	switch {
	case strings.HasPrefix(raw, "a/"):
		return strings.TrimPrefix(raw, "a/")
	case strings.HasPrefix(raw, "b/"):
		return strings.TrimPrefix(raw, "b/")
	default:
		return raw
	}
}

func parseHunkHeader(line string) (int, int, int, int, error) {
	matches := hunkHeaderPattern.FindStringSubmatch(strings.TrimSpace(line))
	if len(matches) != 5 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header %q", line)
	}
	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid old start in hunk header %q", line)
	}
	oldLines := 1
	if strings.TrimSpace(matches[2]) != "" {
		oldLines, err = strconv.Atoi(matches[2])
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid old line count in hunk header %q", line)
		}
	}
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid new start in hunk header %q", line)
	}
	newLines := 1
	if strings.TrimSpace(matches[4]) != "" {
		newLines, err = strconv.Atoi(matches[4])
		if err != nil {
			return 0, 0, 0, 0, fmt.Errorf("invalid new line count in hunk header %q", line)
		}
	}
	return oldStart, oldLines, newStart, newLines, nil
}

func chunkIDFor(path, header, body string) string {
	sum := sha256.Sum256([]byte(path + "\n" + header + "\n" + body))
	return "chk_" + hex.EncodeToString(sum[:8])
}

func chunkPreview(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	snippets := make([]string, 0, 2)
	for _, line := range lines {
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			snippets = append(snippets, strings.TrimSpace(line))
		}
		if len(snippets) >= 2 {
			break
		}
	}
	if len(snippets) == 0 {
		snippets = append(snippets, strings.TrimSpace(lines[0]))
	}
	return strings.Join(snippets, " | ")
}

func checkpointChunkPaths(chunks []parsedPatchChunk) []string {
	seen := map[string]struct{}{}
	paths := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		p := strings.TrimSpace(chunk.Path)
		if p == "" {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
