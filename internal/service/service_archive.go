package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"sophia/internal/gitx"
	"sophia/internal/model"
)

const (
	archiveTrackedPrefix = ".sophia-tracked"
)

type CRArchiveWriteOptions struct {
	OutPath string
	Reason  string
}

type CRArchiveWriteView struct {
	CRID       int
	CRUID      string
	Revision   int
	Path       string
	Bytes      int
	Archive    model.CRArchive
	Config     model.PolicyArchive
	GitSummary model.CRArchiveGitSummary
}

type CRArchiveBackfillOptions struct {
	Commit bool
}

type CRArchiveBackfillView struct {
	ScannedMerged int
	MissingCRIDs  []int
	WrittenPaths  []string
	Committed     bool
	CommitSHA     string
	DryRun        bool
	Config        model.PolicyArchive
}

func (s *Service) archivePolicyConfig() (model.PolicyArchive, error) {
	policy, err := s.repoPolicy()
	if err != nil {
		return model.PolicyArchive{}, err
	}
	return policy.Archive, nil
}

func archivePolicyEnabled(config model.PolicyArchive) bool {
	return config.Enabled != nil && *config.Enabled
}

func archivePolicyIncludeFullDiffs(config model.PolicyArchive) bool {
	return config.IncludeFullDiffs != nil && *config.IncludeFullDiffs
}

func (s *Service) requireArchiveConfigSupported(config model.PolicyArchive) error {
	if strings.TrimSpace(config.Format) != defaultArchiveFormat {
		return fmt.Errorf("%w: archive.format %q is unsupported (expected yaml)", ErrPolicyInvalid, config.Format)
	}
	if archivePolicyIncludeFullDiffs(config) {
		return fmt.Errorf("%w: archive.include_full_diffs=true is not implemented for archive generation", ErrPolicyInvalid)
	}
	return nil
}

func (s *Service) repoRootPath() string {
	root := strings.TrimSpace(s.repoRoot)
	if root != "" {
		return root
	}
	return strings.TrimSpace(s.git.WorkDir)
}

func (s *Service) archiveDirForConfig(config model.PolicyArchive) string {
	return filepath.Join(s.repoRootPath(), config.Path)
}

func archiveRevisionPath(dir string, crID, revision int) string {
	return filepath.Join(dir, fmt.Sprintf("cr-%d.v%d.yaml", crID, revision))
}

func nextArchiveRevision(dir string, crID int) (int, error) {
	pattern := filepath.Join(dir, fmt.Sprintf("cr-%d.v*.yaml", crID))
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return 0, err
	}
	maxRevision := 0
	prefix := fmt.Sprintf("cr-%d.v", crID)
	for _, match := range matches {
		base := filepath.Base(match)
		if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, ".yaml") {
			continue
		}
		raw := strings.TrimSuffix(strings.TrimPrefix(base, prefix), ".yaml")
		revision, parseErr := strconv.Atoi(raw)
		if parseErr != nil || revision <= 0 {
			continue
		}
		if revision > maxRevision {
			maxRevision = revision
		}
	}
	return maxRevision + 1, nil
}

func (s *Service) relativeToRepoPath(absPath string) (string, error) {
	return relativeToRootPath(s.repoRootPath(), absPath)
}

func relativeToRootPath(rootPath, absPath string) (string, error) {
	rootPath = strings.TrimSpace(rootPath)
	if rootPath == "" {
		return "", fmt.Errorf("root path is required")
	}
	rel, err := filepath.Rel(rootPath, absPath)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func archiveWritePath(repoRoot, outPath string) string {
	outPath = strings.TrimSpace(outPath)
	if outPath == "" {
		return ""
	}
	if filepath.IsAbs(outPath) {
		return outPath
	}
	return filepath.Join(repoRoot, outPath)
}

func (s *Service) WriteCRArchive(id int, opts CRArchiveWriteOptions) (*CRArchiveWriteView, error) {
	config, err := s.archivePolicyConfig()
	if err != nil {
		return nil, err
	}
	if err := s.requireArchiveConfigSupported(config); err != nil {
		return nil, err
	}
	cr, err := s.store.LoadCR(id)
	if err != nil {
		return nil, err
	}
	if cr.Status != model.StatusMerged {
		return nil, fmt.Errorf("cr %d must be merged before archive write", id)
	}
	mergedCommit := strings.TrimSpace(cr.MergedCommit)
	if mergedCommit == "" {
		return nil, fmt.Errorf("cr %d has no merged commit", id)
	}

	revisionDir := s.archiveDirForConfig(config)
	revision, err := nextArchiveRevision(revisionDir, cr.ID)
	if err != nil {
		return nil, err
	}
	defaultOutPath := archiveRevisionPath(revisionDir, cr.ID, revision)
	targetPath := archiveWritePath(s.repoRootPath(), opts.OutPath)
	if strings.TrimSpace(targetPath) == "" {
		targetPath = defaultOutPath
	}
	gitSummary, err := s.buildArchiveGitSummaryFromMergeCommit(mergedCommit)
	if err != nil {
		return nil, err
	}
	archive := buildCRArchiveDocument(cr, revision, strings.TrimSpace(opts.Reason), s.timestamp(), gitSummary)
	payload, err := marshalCRArchiveYAML(archive)
	if err != nil {
		return nil, err
	}
	if err := writeArchivePayload(targetPath, payload); err != nil {
		return nil, err
	}
	return &CRArchiveWriteView{
		CRID:       cr.ID,
		CRUID:      strings.TrimSpace(cr.UID),
		Revision:   revision,
		Path:       targetPath,
		Bytes:      len(payload),
		Archive:    archive,
		Config:     config,
		GitSummary: gitSummary,
	}, nil
}

func (s *Service) BackfillCRArchives(opts CRArchiveBackfillOptions) (*CRArchiveBackfillView, error) {
	config, err := s.archivePolicyConfig()
	if err != nil {
		return nil, err
	}
	if err := s.requireArchiveConfigSupported(config); err != nil {
		return nil, err
	}
	crs, err := s.store.ListCRs()
	if err != nil {
		return nil, err
	}
	sort.Slice(crs, func(i, j int) bool { return crs[i].ID < crs[j].ID })

	archiveDir := s.archiveDirForConfig(config)
	missing := make([]model.CR, 0)
	scannedMerged := 0
	for _, cr := range crs {
		if cr.Status != model.StatusMerged {
			continue
		}
		scannedMerged++
		path := archiveRevisionPath(archiveDir, cr.ID, 1)
		if _, statErr := os.Stat(path); statErr == nil {
			continue
		} else if !os.IsNotExist(statErr) {
			return nil, statErr
		}
		missing = append(missing, cr)
	}
	missingIDs := make([]int, 0, len(missing))
	for _, cr := range missing {
		missingIDs = append(missingIDs, cr.ID)
	}
	out := &CRArchiveBackfillView{
		ScannedMerged: scannedMerged,
		MissingCRIDs:  missingIDs,
		WrittenPaths:  []string{},
		Committed:     false,
		CommitSHA:     "",
		DryRun:        !opts.Commit,
		Config:        config,
	}
	if !opts.Commit || len(missing) == 0 {
		return out, nil
	}
	cfg, err := s.store.LoadConfig()
	if err != nil {
		return nil, err
	}
	baseBranch := strings.TrimSpace(cfg.BaseBranch)
	if baseBranch == "" {
		return nil, fmt.Errorf("backfill --commit requires configured base branch")
	}
	currentBranch, err := s.git.CurrentBranch()
	if err != nil {
		return nil, err
	}
	currentBranch = strings.TrimSpace(currentBranch)
	if currentBranch != baseBranch {
		return nil, fmt.Errorf("backfill --commit must run on base branch %q (current branch %q)", baseBranch, currentBranch)
	}
	if dirty, summary, err := s.workingTreeDirtySummaryFor(s.git); err != nil {
		return nil, err
	} else if dirty {
		return nil, fmt.Errorf("%w: %s", ErrWorkingTreeDirty, summary)
	}

	pathsToStage := make([]string, 0, len(missing))
	for _, cr := range missing {
		if strings.TrimSpace(cr.MergedCommit) == "" {
			return nil, fmt.Errorf("cr %d has no merged commit", cr.ID)
		}
		gitSummary, summaryErr := s.buildArchiveGitSummaryFromMergeCommit(cr.MergedCommit)
		if summaryErr != nil {
			return nil, summaryErr
		}
		archive := buildCRArchiveDocument(&cr, 1, "", s.timestamp(), gitSummary)
		payload, marshalErr := marshalCRArchiveYAML(archive)
		if marshalErr != nil {
			return nil, marshalErr
		}
		absPath := archiveRevisionPath(archiveDir, cr.ID, 1)
		if err := writeArchivePayload(absPath, payload); err != nil {
			return nil, err
		}
		relPath, relErr := s.relativeToRepoPath(absPath)
		if relErr != nil {
			return nil, relErr
		}
		pathsToStage = append(pathsToStage, relPath)
		out.WrittenPaths = append(out.WrittenPaths, absPath)
	}
	if err := s.git.StagePaths(pathsToStage); err != nil {
		return nil, err
	}
	if err := s.git.Commit("chore: backfill CR archive artifacts"); err != nil {
		return nil, err
	}
	sha, err := s.git.HeadShortSHA()
	if err != nil {
		return nil, err
	}
	out.Committed = true
	out.CommitSHA = sha
	out.DryRun = false
	return out, nil
}

func marshalCRArchiveYAML(archive model.CRArchive) ([]byte, error) {
	payload, err := yaml.Marshal(archive)
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 || payload[len(payload)-1] != '\n' {
		payload = append(payload, '\n')
	}
	return payload, nil
}

func writeArchivePayload(path string, payload []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o644)
}

func archiveFileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func buildCRArchiveDocument(cr *model.CR, revision int, reason, archivedAt string, gitSummary model.CRArchiveGitSummary) model.CRArchive {
	tasks := make([]model.CRArchiveTask, 0, len(cr.Subtasks))
	for _, task := range cr.Subtasks {
		delegated := make([]model.CRArchiveTaskDelegated, 0, len(task.Delegations))
		for _, delegation := range task.Delegations {
			delegated = append(delegated, model.CRArchiveTaskDelegated{
				ChildCRID:   delegation.ChildCRID,
				ChildCRUID:  strings.TrimSpace(delegation.ChildCRUID),
				ChildTaskID: delegation.ChildTaskID,
			})
		}
		sort.Slice(delegated, func(i, j int) bool {
			if delegated[i].ChildCRID != delegated[j].ChildCRID {
				return delegated[i].ChildCRID < delegated[j].ChildCRID
			}
			return delegated[i].ChildTaskID < delegated[j].ChildTaskID
		})
		tasks = append(tasks, model.CRArchiveTask{
			ID:     task.ID,
			Title:  task.Title,
			Status: task.Status,
			Contract: model.CRArchiveTaskContract{
				Intent:             strings.TrimSpace(task.Contract.Intent),
				AcceptanceCriteria: sortedStringCopy(task.Contract.AcceptanceCriteria),
				Scope:              sortedStringCopy(task.Contract.Scope),
				AcceptanceChecks:   sortedStringCopy(task.Contract.AcceptanceChecks),
			},
			Checkpoint: model.CRArchiveTaskCheckpoint{
				Commit: strings.TrimSpace(task.CheckpointCommit),
				At:     strings.TrimSpace(task.CheckpointAt),
				Source: strings.TrimSpace(task.CheckpointSource),
				Scope:  sortedStringCopy(task.CheckpointScope),
			},
			Delegated: delegated,
		})
	}
	sort.Slice(tasks, func(i, j int) bool {
		return tasks[i].ID < tasks[j].ID
	})
	return model.CRArchive{
		SchemaVersion: model.CRArchiveSchemaV1,
		Notice:        model.CRArchiveNotice,
		ArchivedAt:    strings.TrimSpace(archivedAt),
		Revision:      revision,
		Reason:        strings.TrimSpace(reason),
		CR: model.CRArchiveCR{
			ID:          cr.ID,
			UID:         strings.TrimSpace(cr.UID),
			Title:       strings.TrimSpace(cr.Title),
			Description: strings.TrimSpace(cr.Description),
			Status:      strings.TrimSpace(cr.Status),
			BaseBranch:  strings.TrimSpace(cr.BaseBranch),
			BaseRef:     strings.TrimSpace(cr.BaseRef),
			BaseCommit:  strings.TrimSpace(cr.BaseCommit),
			Branch:      strings.TrimSpace(cr.Branch),
			MergedAt:    strings.TrimSpace(cr.MergedAt),
			MergedBy:    strings.TrimSpace(cr.MergedBy),
		},
		Contract: model.CRArchiveContract{
			Why:                strings.TrimSpace(cr.Contract.Why),
			Scope:              sortedStringCopy(cr.Contract.Scope),
			NonGoals:           sortedStringCopy(cr.Contract.NonGoals),
			Invariants:         sortedStringCopy(cr.Contract.Invariants),
			BlastRadius:        strings.TrimSpace(cr.Contract.BlastRadius),
			RiskCriticalScopes: sortedStringCopy(cr.Contract.RiskCriticalScopes),
			RiskTierHint:       strings.TrimSpace(cr.Contract.RiskTierHint),
			RiskRationale:      strings.TrimSpace(cr.Contract.RiskRationale),
			TestPlan:           strings.TrimSpace(cr.Contract.TestPlan),
			RollbackPlan:       strings.TrimSpace(cr.Contract.RollbackPlan),
		},
		Tasks:      tasks,
		GitSummary: gitSummary,
	}
}

func sortedStringCopy(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	sort.Strings(out)
	return out
}

func (s *Service) buildArchiveGitSummaryFromMergeCommit(mergeCommit string) (model.CRArchiveGitSummary, error) {
	mergeCommit = strings.TrimSpace(mergeCommit)
	if mergeCommit == "" {
		return model.CRArchiveGitSummary{}, fmt.Errorf("merge commit is required for archive summary")
	}
	parents, err := s.git.CommitParents(mergeCommit)
	if err != nil {
		return model.CRArchiveGitSummary{}, err
	}
	if len(parents) < 2 {
		return model.CRArchiveGitSummary{}, fmt.Errorf("commit %s is not a merge commit", shortHash(mergeCommit))
	}
	baseRef := mergeCommit + "^1"
	changes, err := s.git.DiffNameStatusBetween(baseRef, mergeCommit)
	if err != nil {
		return model.CRArchiveGitSummary{}, err
	}
	numStats, err := s.git.DiffNumStatBetween(baseRef, mergeCommit)
	if err != nil {
		return model.CRArchiveGitSummary{}, err
	}
	return buildArchiveGitSummary(changes, numStats, parents[0], parents[1]), nil
}

func buildArchiveGitSummaryFromCachedDiff(gitClient *gitx.Client, baseParent, crParent string) (model.CRArchiveGitSummary, error) {
	if gitClient == nil {
		return model.CRArchiveGitSummary{}, fmt.Errorf("git client is required")
	}
	changes, err := gitClient.DiffNameStatusCached()
	if err != nil {
		return model.CRArchiveGitSummary{}, err
	}
	numStats, err := gitClient.DiffNumStatCached()
	if err != nil {
		return model.CRArchiveGitSummary{}, err
	}
	return buildArchiveGitSummary(changes, numStats, baseParent, crParent), nil
}

func buildArchiveGitSummary(changes []gitx.FileChange, numStats []gitx.DiffNumStat, baseParent, crParent string) model.CRArchiveGitSummary {
	files := make([]string, 0, len(changes))
	fileSeen := map[string]struct{}{}
	for _, change := range changes {
		path := normalizeArchiveSummaryPath(change.Path)
		if path == "" || isArchiveTrackedPath(path) {
			continue
		}
		if _, ok := fileSeen[path]; ok {
			continue
		}
		fileSeen[path] = struct{}{}
		files = append(files, path)
	}
	sort.Strings(files)

	diffRows := make([]model.CRArchiveDiffStatRow, 0, len(numStats))
	totalInsertions := 0
	totalDeletions := 0
	for _, row := range numStats {
		path := normalizeArchiveSummaryPath(row.Path)
		if path == "" || isArchiveTrackedPath(path) {
			continue
		}
		item := model.CRArchiveDiffStatRow{
			Path:   path,
			Binary: row.Binary,
		}
		if row.Insertions != nil {
			insertions := *row.Insertions
			item.Insertions = &insertions
			totalInsertions += insertions
		}
		if row.Deletions != nil {
			deletions := *row.Deletions
			item.Deletions = &deletions
			totalDeletions += deletions
		}
		diffRows = append(diffRows, item)
	}
	sort.Slice(diffRows, func(i, j int) bool {
		return diffRows[i].Path < diffRows[j].Path
	})
	return model.CRArchiveGitSummary{
		BaseParent:   strings.TrimSpace(baseParent),
		CRParent:     strings.TrimSpace(crParent),
		FilesChanged: files,
		DiffStat: model.CRArchiveDiffStat{
			Summary: archiveShortStatSummary(len(files), totalInsertions, totalDeletions),
			Files:   diffRows,
		},
	}
}

func archiveShortStatSummary(files, insertions, deletions int) string {
	fileNoun := "files"
	if files == 1 {
		fileNoun = "file"
	}
	insertionNoun := "insertions"
	if insertions == 1 {
		insertionNoun = "insertion"
	}
	deletionNoun := "deletions"
	if deletions == 1 {
		deletionNoun = "deletion"
	}
	return fmt.Sprintf("%d %s changed, %d %s(+), %d %s(-)", files, fileNoun, insertions, insertionNoun, deletions, deletionNoun)
}

func normalizeArchiveSummaryPath(path string) string {
	return strings.TrimSpace(strings.ReplaceAll(path, "\\", "/"))
}

func isArchiveTrackedPath(path string) bool {
	return pathMatchesScopePrefix(normalizeArchiveSummaryPath(path), archiveTrackedPrefix)
}
