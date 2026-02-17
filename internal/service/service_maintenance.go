package service

import (
	"errors"
	"fmt"
	"sophia/internal/model"
	"sophia/internal/store"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Service) Log() ([]LogEntry, error) {
	crs, err := s.store.ListCRs()
	if err == nil && len(crs) > 0 {
		return buildLogEntriesFromCRs(crs), nil
	}
	if err != nil && !errors.Is(err, store.ErrNotInitialized) {
		return nil, err
	}
	return s.logFromGit(200)
}

func buildLogEntriesFromCRs(crs []model.CR) []LogEntry {
	type stampedEntry struct {
		entry LogEntry
		ts    time.Time
	}
	merged := make([]stampedEntry, 0)
	active := make([]stampedEntry, 0)

	for _, cr := range crs {
		if cr.Status == model.StatusMerged {
			when := cr.MergedAt
			if when == "" {
				when = cr.UpdatedAt
			}
			filesTouched := "-"
			if cr.MergedAt != "" || cr.MergedCommit != "" {
				filesTouched = strconv.Itoa(cr.FilesTouchedCount)
			}
			entry := stampedEntry{
				entry: LogEntry{
					ID:           cr.ID,
					Title:        cr.Title,
					Status:       cr.Status,
					Who:          nonEmptyTrimmed(cr.MergedBy, "-"),
					When:         when,
					FilesTouched: filesTouched,
				},
				ts: parseRFC3339OrZero(when),
			}
			merged = append(merged, entry)
			continue
		}
		entry := stampedEntry{
			entry: LogEntry{
				ID:           cr.ID,
				Title:        cr.Title,
				Status:       cr.Status,
				Who:          "-",
				When:         cr.UpdatedAt,
				FilesTouched: "-",
			},
			ts: parseRFC3339OrZero(cr.UpdatedAt),
		}
		active = append(active, entry)
	}

	sort.Slice(merged, func(i, j int) bool {
		return merged[i].ts.After(merged[j].ts)
	})
	sort.Slice(active, func(i, j int) bool {
		return active[i].ts.After(active[j].ts)
	})

	res := make([]LogEntry, 0, len(merged)+len(active))
	for _, item := range merged {
		res = append(res, item.entry)
	}
	for _, item := range active {
		res = append(res, item.entry)
	}
	return res
}

func (s *Service) logFromGit(limit int) ([]LogEntry, error) {
	branch := s.git.DefaultBranch()
	commits, err := s.git.RecentCommits(branch, limit)
	if err != nil {
		return nil, err
	}
	entries := make([]LogEntry, 0)
	seen := map[int]struct{}{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}

		filesCount, countErr := s.git.ChangedFileCount(commit.Hash)
		filesTouched := "-"
		if countErr == nil {
			filesTouched = strconv.Itoa(filesCount)
		}
		entries = append(entries, LogEntry{
			ID:           id,
			Title:        titleFromSubjectOrBody(commit.Subject, commit.Body),
			Status:       model.StatusMerged,
			Who:          nonEmptyTrimmed(commit.Author, "-"),
			When:         nonEmptyTrimmed(commit.When, "-"),
			FilesTouched: filesTouched,
		})
	}
	return entries, nil
}

func (s *Service) RepairFromGit(baseBranch string, refresh bool) (*RepairReport, error) {
	targetBase := strings.TrimSpace(baseBranch)
	if targetBase == "" {
		if s.store.IsInitialized() {
			cfg, err := s.store.LoadConfig()
			if err == nil && strings.TrimSpace(cfg.BaseBranch) != "" {
				targetBase = strings.TrimSpace(cfg.BaseBranch)
			}
		}
	}
	if targetBase == "" {
		targetBase = s.git.DefaultBranch()
	}
	if !s.git.BranchExists(targetBase) {
		return nil, fmt.Errorf("base branch %q does not exist", targetBase)
	}

	mode := model.MetadataModeLocal
	if s.store.IsInitialized() {
		if cfg, err := s.store.LoadConfig(); err == nil && strings.TrimSpace(cfg.MetadataMode) != "" {
			mode = cfg.MetadataMode
		}
	}
	if err := s.store.Init(targetBase, mode); err != nil {
		return nil, err
	}

	existingMap := map[int]*model.CR{}
	if existing, err := s.store.ListCRs(); err == nil {
		for i := range existing {
			cr := existing[i]
			c := cr
			existingMap[cr.ID] = &c
		}
	}

	commits, err := s.git.RecentCommits(targetBase, 5000)
	if err != nil {
		return nil, err
	}

	report := &RepairReport{
		BaseBranch:    targetBase,
		Scanned:       len(commits),
		RepairedCRIDs: []int{},
	}

	uidBackfilledSet := map[int]struct{}{}
	for id, existing := range existingMap {
		changedUID, uidErr := ensureCRUID(existing)
		if uidErr != nil {
			return nil, uidErr
		}
		changedBase, baseErr := s.ensureCRBaseFields(existing, false)
		if baseErr != nil {
			return nil, baseErr
		}
		if !changedUID && !changedBase {
			continue
		}
		if strings.TrimSpace(existing.CreatedAt) == "" {
			existing.CreatedAt = s.timestamp()
		}
		existing.UpdatedAt = s.timestamp()
		if err := s.store.SaveCR(existing); err != nil {
			return nil, err
		}
		report.Updated++
		uidBackfilledSet[id] = struct{}{}
	}

	repairedSet := map[int]struct{}{}
	for _, commit := range commits {
		id, ok := crIDFromSubjectOrBody(commit.Subject, commit.Body)
		if !ok {
			continue
		}

		existing, exists := existingMap[id]
		if exists && existing.Status == model.StatusInProgress && !refresh {
			report.Skipped++
			continue
		}

		description := intentFromCommitBody(commit.Body)
		notes := notesFromCommitBody(commit.Body)
		subtasks := subtasksFromCommitBody(commit.Body, commit.When, commit.Author)
		when := nonEmptyTrimmed(commit.When, s.timestamp())
		actor := nonEmptyTrimmed(commit.Author, "unknown")
		filesTouched := 0
		if count, countErr := s.git.ChangedFileCount(commit.Hash); countErr == nil {
			filesTouched = count
		}

		cr := &model.CR{
			ID:                id,
			UID:               crUIDFromBody(commit.Body),
			Title:             titleFromSubjectOrBody(commit.Subject, commit.Body),
			Description:       description,
			Status:            model.StatusMerged,
			BaseBranch:        targetBase,
			BaseRef:           baseRefFromBody(commit.Body),
			BaseCommit:        baseCommitFromBody(commit.Body),
			ParentCRID:        parentCRIDFromBody(commit.Body),
			Branch:            fmt.Sprintf("sophia/cr-%d", id),
			Notes:             notes,
			Subtasks:          subtasks,
			Events:            []model.Event{},
			MergedAt:          when,
			MergedBy:          actor,
			MergedCommit:      shortHash(commit.Hash),
			FilesTouchedCount: filesTouched,
			CreatedAt:         when,
			UpdatedAt:         when,
		}

		if exists {
			if strings.TrimSpace(cr.UID) == "" {
				cr.UID = strings.TrimSpace(existing.UID)
			}
			if strings.TrimSpace(cr.BaseRef) == "" {
				cr.BaseRef = strings.TrimSpace(existing.BaseRef)
			}
			if strings.TrimSpace(cr.BaseCommit) == "" {
				cr.BaseCommit = strings.TrimSpace(existing.BaseCommit)
			}
			if cr.ParentCRID <= 0 {
				cr.ParentCRID = existing.ParentCRID
			}
			cr.CreatedAt = existing.CreatedAt
			if strings.TrimSpace(cr.CreatedAt) == "" {
				cr.CreatedAt = when
			}
			cr.Events = append([]model.Event{}, existing.Events...)
			if _, backfilled := uidBackfilledSet[id]; !backfilled {
				report.Updated++
			}
		} else {
			report.Imported++
		}
		if _, uidErr := ensureCRUID(cr); uidErr != nil {
			return nil, uidErr
		}
		if _, baseErr := s.ensureCRBaseFields(cr, false); baseErr != nil {
			return nil, baseErr
		}
		cr.Events = append(cr.Events, model.Event{
			TS:      s.timestamp(),
			Actor:   s.git.Actor(),
			Type:    "cr_repaired",
			Summary: fmt.Sprintf("Repaired CR %d from git commit %s", id, shortHash(commit.Hash)),
			Ref:     fmt.Sprintf("cr:%d", id),
		})

		if err := s.store.SaveCR(cr); err != nil {
			return nil, err
		}
		existingMap[id] = cr
		if id > report.HighestCRID {
			report.HighestCRID = id
		}
		if _, seen := repairedSet[id]; !seen {
			repairedSet[id] = struct{}{}
			report.RepairedCRIDs = append(report.RepairedCRIDs, id)
		}
	}

	if err := s.ensureNextCRIDFloor(targetBase); err != nil {
		return nil, err
	}
	idx, err := s.store.LoadIndex()
	if err != nil {
		return nil, err
	}
	report.NextID = idx.NextID
	sort.Ints(report.RepairedCRIDs)
	return report, nil
}

func (s *Service) InstallHook(forceOverwrite bool) (string, error) {
	if err := s.store.EnsureInitialized(); err != nil {
		return "", err
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return "", err
	}
	return s.git.InstallPreCommitHook(cfg.BaseBranch, forceOverwrite)
}
