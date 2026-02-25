package service

import (
	"sophia/internal/gitx"
	"sophia/internal/model"
)

type lifecycleRuntimeStore interface {
	EnsureInitialized() error
	LoadConfig() (model.Config, error)
	NextCRID() (int, error)
	LoadCR(id int) (*model.CR, error)
	SaveCR(cr *model.CR) error
	ListCRs() ([]model.CR, error)
	LoadIndex() (model.Index, error)
	SaveIndex(idx model.Index) error
}

type lifecycleRuntimeGit interface {
	CurrentBranch() (string, error)
	BranchExists(branch string) bool
	DiffNames(baseBranch, branch string) ([]string, error)
	EnsureBranchExists(branch string) error
	EnsureBootstrapCommit(message string) error
	CreateBranchFrom(branch, ref string) error
	CreateBranchAt(branch, ref string) error
	ResolveRef(ref string) (string, error)
	Actor() string
	LocalBranches(prefix string) ([]string, error)
	RecentCommits(branch string, limit int) ([]gitx.Commit, error)
	RebaseBranchOnto(branch, ontoRef string) error
	RebaseCurrentBranchOnto(ontoRef string) error
	WorkingTreeStatus() ([]gitx.StatusEntry, error)
	GitCommonDirAbs() (string, error)
}

type statusRuntimeStore interface {
	LoadCR(id int) (*model.CR, error)
	SaveCR(cr *model.CR) error
}

type statusRuntimeGit interface {
	Actor() string
	CurrentBranch() (string, error)
	WorkingTreeStatus() ([]gitx.StatusEntry, error)
}

type mergeRuntimeStore interface {
	EnsureInitialized() error
	LoadConfig() (model.Config, error)
	LoadCR(id int) (*model.CR, error)
	SaveCR(cr *model.CR) error
	ListCRs() ([]model.CR, error)
	SophiaDir() string
}

type mergeRuntimeGit interface {
	Actor() string
	BranchExists(branch string) bool
	ChangedFileCount(hash string) (int, error)
	CheckoutBranch(branch string) error
	Commit(message string) error
	CurrentBranch() (string, error)
	DeleteBranch(branch string, force bool) error
	DiffNameStatusCached() ([]gitx.FileChange, error)
	DiffNumStatCached() ([]gitx.DiffNumStat, error)
	HeadShortSHA() (string, error)
	IsMergeInProgress() (bool, error)
	MergeAbort() error
	MergeConflictFiles() ([]string, error)
	MergeContinue() error
	MergeHeadSHA() (string, error)
	MergeNoFFNoCommitOnCurrentBranch(branch, message string) error
	RecentCommits(branch string, limit int) ([]gitx.Commit, error)
	RebaseBranchOnto(branch, ontoRef string) error
	RebaseCurrentBranchOnto(ontoRef string) error
	ResolveRef(ref string) (string, error)
	StagePaths(paths []string) error
	TrackedFiles(pathspec string) ([]string, error)
	WorkingTreeStatus() ([]gitx.StatusEntry, error)
	WorktreeForBranch(branch string) (*gitx.Worktree, error)
}

type mergeRuntimeGitFactory func(root string) mergeRuntimeGit

func (s *Service) activeLifecycleStoreProvider() lifecycleRuntimeStore {
	if s.lifecycleStore != nil {
		return s.lifecycleStore
	}
	return s.store
}

func (s *Service) activeLifecycleGitProvider() lifecycleRuntimeGit {
	if s.lifecycleGit != nil {
		return s.lifecycleGit
	}
	return s.git
}

func (s *Service) overrideLifecycleRuntimeProvidersForTests(git lifecycleRuntimeGit, store lifecycleRuntimeStore) {
	s.lifecycleGit = git
	s.lifecycleStore = store
}

func (s *Service) activeStatusStoreProvider() statusRuntimeStore {
	if s.statusStore != nil {
		return s.statusStore
	}
	return s.store
}

func (s *Service) activeStatusGitProvider() statusRuntimeGit {
	if s.statusGit != nil {
		return s.statusGit
	}
	return s.git
}

func (s *Service) overrideStatusRuntimeProvidersForTests(git statusRuntimeGit, store statusRuntimeStore) {
	s.statusGit = git
	s.statusStore = store
}

func (s *Service) activeMergeStoreProvider() mergeRuntimeStore {
	if s.mergeStore != nil {
		return s.mergeStore
	}
	return s.store
}

func (s *Service) activeMergeGitProvider() mergeRuntimeGit {
	if s.mergeGit != nil {
		return s.mergeGit
	}
	return s.git
}

func (s *Service) activeMergeGitFactory() mergeRuntimeGitFactory {
	if s.mergeGitFactory != nil {
		return s.mergeGitFactory
	}
	return func(root string) mergeRuntimeGit {
		return gitx.New(root)
	}
}

func (s *Service) overrideMergeRuntimeProvidersForTests(git mergeRuntimeGit, store mergeRuntimeStore, factory mergeRuntimeGitFactory) {
	s.mergeGit = git
	s.mergeStore = store
	s.mergeGitFactory = factory
}
