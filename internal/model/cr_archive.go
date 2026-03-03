package model

const (
	CRArchiveSchemaV1 = "sophia.cr_archive.v1"
	CRArchiveSchemaV2 = "sophia.cr_archive.v2"
	CRArchiveNotice   = "Historical archive snapshot. Corrections are append-only revisions."
)

type CRArchive struct {
	SchemaVersion string              `yaml:"schema_version"`
	Notice        string              `yaml:"notice"`
	ArchivedAt    string              `yaml:"archived_at"`
	Revision      int                 `yaml:"revision"`
	Reason        string              `yaml:"reason,omitempty"`
	CR            CRArchiveCR         `yaml:"cr"`
	Contract      CRArchiveContract   `yaml:"contract"`
	Tasks         []CRArchiveTask     `yaml:"tasks,omitempty"`
	GitSummary    CRArchiveGitSummary `yaml:"git_summary"`
	FullDiff      *CRArchiveFullDiff  `yaml:"full_diff,omitempty"`
}

const (
	CRArchiveFullDiffEncodingGitUnifiedPatch = "git_unified_patch"
)

type CRArchiveFullDiff struct {
	Encoding string `yaml:"encoding"`
	Bytes    int    `yaml:"bytes"`
	Patch    string `yaml:"patch"`
}

type CRArchiveCR struct {
	ID          int    `yaml:"id"`
	UID         string `yaml:"uid,omitempty"`
	Title       string `yaml:"title"`
	Description string `yaml:"description,omitempty"`
	Status      string `yaml:"status"`
	BaseBranch  string `yaml:"base_branch,omitempty"`
	BaseRef     string `yaml:"base_ref,omitempty"`
	BaseCommit  string `yaml:"base_commit,omitempty"`
	Branch      string `yaml:"branch,omitempty"`
	MergedAt    string `yaml:"merged_at,omitempty"`
	MergedBy    string `yaml:"merged_by,omitempty"`
}

type CRArchiveContract struct {
	Why                string   `yaml:"why,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	NonGoals           []string `yaml:"non_goals,omitempty"`
	Invariants         []string `yaml:"invariants,omitempty"`
	BlastRadius        string   `yaml:"blast_radius,omitempty"`
	RiskCriticalScopes []string `yaml:"risk_critical_scopes,omitempty"`
	RiskTierHint       string   `yaml:"risk_tier_hint,omitempty"`
	RiskRationale      string   `yaml:"risk_rationale,omitempty"`
	TestPlan           string   `yaml:"test_plan,omitempty"`
	RollbackPlan       string   `yaml:"rollback_plan,omitempty"`
}

type CRArchiveTask struct {
	ID         int                      `yaml:"id"`
	Title      string                   `yaml:"title"`
	Status     string                   `yaml:"status"`
	Contract   CRArchiveTaskContract    `yaml:"contract,omitempty"`
	Checkpoint CRArchiveTaskCheckpoint  `yaml:"checkpoint,omitempty"`
	Delegated  []CRArchiveTaskDelegated `yaml:"delegated,omitempty"`
}

type CRArchiveTaskContract struct {
	Intent             string   `yaml:"intent,omitempty"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	AcceptanceChecks   []string `yaml:"acceptance_checks,omitempty"`
}

type CRArchiveTaskCheckpoint struct {
	Commit string   `yaml:"commit,omitempty"`
	At     string   `yaml:"at,omitempty"`
	Source string   `yaml:"source,omitempty"`
	Scope  []string `yaml:"scope,omitempty"`
}

type CRArchiveTaskDelegated struct {
	ChildCRID   int    `yaml:"child_cr_id"`
	ChildCRUID  string `yaml:"child_cr_uid,omitempty"`
	ChildTaskID int    `yaml:"child_task_id,omitempty"`
}

type CRArchiveGitSummary struct {
	BaseParent   string            `yaml:"base_parent,omitempty"`
	CRParent     string            `yaml:"cr_parent,omitempty"`
	FilesChanged []string          `yaml:"files_changed"`
	DiffStat     CRArchiveDiffStat `yaml:"diffstat"`
}

type CRArchiveDiffStat struct {
	Summary string                 `yaml:"summary"`
	Files   []CRArchiveDiffStatRow `yaml:"files,omitempty"`
}

type CRArchiveDiffStatRow struct {
	Path       string `yaml:"path"`
	Insertions *int   `yaml:"insertions,omitempty"`
	Deletions  *int   `yaml:"deletions,omitempty"`
	Binary     bool   `yaml:"binary,omitempty"`
}
