package service

import (
	"sophia/internal/gitx"
	"sophia/internal/model"
)

type taskLifecycleStoreProvider interface {
	LoadCR(id int) (*model.CR, error)
	SaveCR(cr *model.CR) error
}

type taskLifecycleGitProvider interface {
	Actor() string
	CurrentBranch() (string, error)
	HasStagedChanges() (bool, error)
	StageAll() error
	StagePaths(paths []string) error
	ApplyPatchToIndex(patchPath string) error
	PathHasChanges(path string) (bool, error)
	WorkingTreeStatus() ([]gitx.StatusEntry, error)
	Commit(msg string) error
	HeadShortSHA() (string, error)
}

func (s *Service) activeTaskStoreProvider() taskLifecycleStoreProvider {
	if s.taskStore != nil {
		return s.taskStore
	}
	return s.store
}

func (s *Service) activeTaskGitProvider() taskLifecycleGitProvider {
	if s.taskGit != nil {
		return s.taskGit
	}
	return s.git
}

func (s *Service) overrideTaskRuntimeProvidersForTests(git taskLifecycleGitProvider, store taskLifecycleStoreProvider) {
	s.taskGit = git
	s.taskStore = store
}

func (s *Service) activeTaskMergeGuard() func(*model.CR) error {
	if s.taskMergeGuard != nil {
		return s.taskMergeGuard
	}
	return s.ensureNoMergeInProgressForCR
}

func (s *Service) overrideTaskMergeGuardForTests(guard func(*model.CR) error) {
	s.taskMergeGuard = guard
}
