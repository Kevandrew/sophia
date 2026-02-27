package gitx

type ConcernOwnership struct {
	Concern          string
	TargetFile       string
	Responsibilities []string
}

type SharedHelperPlacement struct {
	Helper    string
	OwnerFile string
}

func SplitConcernOwnership() []ConcernOwnership {
	return []ConcernOwnership{
		{
			Concern:    "repository_context",
			TargetFile: "internal/gitx/git_repo_context.go",
			Responsibilities: []string{
				"RepoRoot",
				"GitCommonDir",
				"GitCommonDirAbs",
				"InRepo",
				"GitDir",
			},
		},
		{
			Concern:    "branch_and_ref_lifecycle",
			TargetFile: "internal/gitx/git_branch_refs.go",
			Responsibilities: []string{
				"CurrentBranch",
				"BranchExists",
				"ResolveRef",
				"ListRefs",
				"CreateBranch* and CheckoutBranch",
			},
		},
		{
			Concern:    "worktree_discovery",
			TargetFile: "internal/gitx/git_worktree.go",
			Responsibilities: []string{
				"ListWorktrees",
				"WorktreeForBranch",
				"parseWorktreeListPorcelain",
			},
		},
		{
			Concern:    "staging_and_index_lock",
			TargetFile: "internal/gitx/git_index_stage.go",
			Responsibilities: []string{
				"StageAll",
				"StagePaths",
				"ApplyPatchToIndex",
				"HasStagedChanges",
				"IndexLockError and retry policy",
			},
		},
		{
			Concern:    "diff_status_surface",
			TargetFile: "internal/gitx/git_diff_status.go",
			Responsibilities: []string{
				"DiffNames*",
				"DiffNameStatus*",
				"DiffShortStat*",
				"DiffNumStat*",
				"WorkingTreeStatus",
			},
		},
		{
			Concern:    "history_and_blame_reads",
			TargetFile: "internal/gitx/git_history_blame.go",
			Responsibilities: []string{
				"RecentCommits",
				"CommitByHash",
				"CommitFiles",
				"CommitPatch",
				"BlameLinePorcelain and parse helpers",
			},
		},
		{
			Concern:    "merge_rebase_mutations",
			TargetFile: "internal/gitx/git_merge_rebase.go",
			Responsibilities: []string{
				"MergeNoFF*",
				"MergeAbort and MergeContinue",
				"MergeHeadSHA and MergeConflictFiles",
				"Rebase*",
				"SquashMerge",
			},
		},
		{
			Concern:    "execution_and_identity_shared",
			TargetFile: "internal/gitx/git_exec_shared.go",
			Responsibilities: []string{
				"run",
				"identityFlags",
				"Actor",
				"containsIndexLockFailure",
			},
		},
	}
}

func SplitSharedHelperPlacement() []SharedHelperPlacement {
	return []SharedHelperPlacement{
		{Helper: "run", OwnerFile: "internal/gitx/git_exec_shared.go"},
		{Helper: "identityFlags", OwnerFile: "internal/gitx/git_exec_shared.go"},
		{Helper: "containsIndexLockFailure", OwnerFile: "internal/gitx/git_exec_shared.go"},
		{Helper: "parseDiffNames", OwnerFile: "internal/gitx/git_diff_status.go"},
		{Helper: "parseWorktreeListPorcelain", OwnerFile: "internal/gitx/git_worktree.go"},
		{Helper: "parseBlamePorcelain", OwnerFile: "internal/gitx/git_history_blame.go"},
		{Helper: "buildBlameArgs", OwnerFile: "internal/gitx/git_history_blame.go"},
	}
}
