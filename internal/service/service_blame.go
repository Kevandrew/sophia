package service

import (
	"fmt"
	"strings"

	"sophia/internal/gitx"
)

type blameCommitMetadata struct {
	CRID         int
	HasCR        bool
	CRUID        string
	Intent       string
	IntentSource string
}

func (s *Service) BlameFile(path string, opts BlameOptions) (*BlameView, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	rawRev := strings.TrimSpace(opts.Rev)
	viewRev := "HEAD"
	if rawRev != "" {
		viewRev = rawRev
	}

	normalizedRanges, err := normalizeBlameRanges(opts.Ranges)
	if err != nil {
		return nil, err
	}
	gitRanges := make([]gitx.BlameRange, 0, len(normalizedRanges))
	for _, r := range normalizedRanges {
		gitRanges = append(gitRanges, gitx.BlameRange{Start: r.Start, End: r.End})
	}

	blamed, err := s.git.BlameLinePorcelain(path, rawRev, gitRanges)
	if err != nil {
		return nil, err
	}

	commitCache := map[string]blameCommitMetadata{}
	lines := make([]BlameLineView, 0, len(blamed))
	for _, line := range blamed {
		hash := strings.TrimSpace(line.CommitHash)
		meta, ok := commitCache[hash]
		if !ok {
			meta, err = s.resolveBlameCommitMetadata(hash)
			if err != nil {
				return nil, err
			}
			commitCache[hash] = meta
		}

		intent := strings.TrimSpace(meta.Intent)
		intentSource := strings.TrimSpace(meta.IntentSource)
		if intent == "" {
			summary := strings.TrimSpace(line.Summary)
			if summary != "" {
				intent = summary
				intentSource = "commit_summary_fallback"
			}
		}
		if intentSource == "" {
			intentSource = "none"
		}

		lines = append(lines, BlameLineView{
			Line:         line.FinalLine,
			Commit:       shortHash(hash),
			Author:       strings.TrimSpace(line.Author),
			AuthorEmail:  strings.TrimSpace(line.AuthorMail),
			AuthorTime:   strings.TrimSpace(line.AuthorTime),
			CRID:         meta.CRID,
			HasCR:        meta.HasCR,
			CRUID:        strings.TrimSpace(meta.CRUID),
			Intent:       intent,
			IntentSource: intentSource,
			Summary:      strings.TrimSpace(line.Summary),
			Text:         line.Text,
		})
	}

	return &BlameView{
		Path:   path,
		Rev:    viewRev,
		Ranges: normalizedRanges,
		Lines:  lines,
	}, nil
}

func normalizeBlameRanges(raw []BlameRange) ([]BlameRange, error) {
	if len(raw) == 0 {
		return []BlameRange{}, nil
	}
	ranges := make([]BlameRange, 0, len(raw))
	for _, r := range raw {
		if r.Start <= 0 || r.End <= 0 {
			return nil, fmt.Errorf("invalid line range %d,%d: values must be >= 1", r.Start, r.End)
		}
		if r.Start > r.End {
			return nil, fmt.Errorf("invalid line range %d,%d: start must be <= end", r.Start, r.End)
		}
		ranges = append(ranges, BlameRange{Start: r.Start, End: r.End})
	}
	return ranges, nil
}

func (s *Service) resolveBlameCommitMetadata(hash string) (blameCommitMetadata, error) {
	hash = strings.TrimSpace(hash)
	if hash == "" || isZeroHash(hash) {
		return blameCommitMetadata{IntentSource: "none"}, nil
	}

	commit, err := s.git.CommitByHash(hash)
	if err != nil {
		return blameCommitMetadata{}, err
	}

	meta := blameCommitMetadata{}
	if id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body); ok {
		meta.CRID = id
		meta.HasCR = true
		meta.CRUID = crUIDFromBody(commit.Body)
	}

	if matches := footerIntentPattern.FindStringSubmatch(commit.Body); len(matches) == 2 {
		intent := strings.TrimSpace(matches[1])
		if intent != "" {
			meta.Intent = intent
			meta.IntentSource = "sophia_footer"
			return meta, nil
		}
	}

	if meta.HasCR && s.store.IsInitialized() {
		if cr, loadErr := s.store.LoadCR(meta.CRID); loadErr == nil {
			if strings.TrimSpace(meta.CRUID) == "" {
				meta.CRUID = strings.TrimSpace(cr.UID)
			}
			if intent := strings.TrimSpace(cr.Title); intent != "" {
				meta.Intent = intent
				meta.IntentSource = "cr_metadata_fallback"
				return meta, nil
			}
		}
	}

	summary := strings.TrimSpace(commit.Subject)
	if summary != "" {
		meta.Intent = summary
		meta.IntentSource = "commit_summary_fallback"
		return meta, nil
	}

	meta.IntentSource = "none"
	return meta, nil
}

func isZeroHash(hash string) bool {
	hash = strings.TrimSpace(hash)
	if hash == "" {
		return true
	}
	for _, ch := range hash {
		if ch != '0' {
			return false
		}
	}
	return true
}
