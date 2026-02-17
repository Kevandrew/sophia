package model

const (
	StatusInProgress = "in_progress"
	StatusMerged     = "merged"

	TaskStatusOpen      = "open"
	TaskStatusDone      = "done"
	TaskStatusDelegated = "delegated"

	MetadataModeLocal   = "local"
	MetadataModeTracked = "tracked"
)

type Config struct {
	Version      string `yaml:"version"`
	BaseBranch   string `yaml:"base_branch"`
	MetadataMode string `yaml:"metadata_mode,omitempty"`
}

type Index struct {
	NextID int `yaml:"next_id"`
}

type Event struct {
	TS              string            `yaml:"ts"`
	Actor           string            `yaml:"actor"`
	Type            string            `yaml:"type"`
	Summary         string            `yaml:"summary"`
	Ref             string            `yaml:"ref,omitempty"`
	Redacted        bool              `yaml:"redacted,omitempty"`
	RedactionReason string            `yaml:"redaction_reason,omitempty"`
	Meta            map[string]string `yaml:"meta,omitempty"`
}

type CheckpointChunk struct {
	ID       string `yaml:"id"`
	Path     string `yaml:"path"`
	OldStart int    `yaml:"old_start"`
	OldLines int    `yaml:"old_lines"`
	NewStart int    `yaml:"new_start"`
	NewLines int    `yaml:"new_lines"`
}

type TaskDelegation struct {
	ChildCRID   int    `yaml:"child_cr_id"`
	ChildCRUID  string `yaml:"child_cr_uid,omitempty"`
	ChildTaskID int    `yaml:"child_task_id,omitempty"`
	LinkedAt    string `yaml:"linked_at,omitempty"`
	LinkedBy    string `yaml:"linked_by,omitempty"`
}

type Subtask struct {
	ID                int               `yaml:"id"`
	Title             string            `yaml:"title"`
	Status            string            `yaml:"status"`
	CreatedAt         string            `yaml:"created_at"`
	UpdatedAt         string            `yaml:"updated_at"`
	CompletedAt       string            `yaml:"completed_at,omitempty"`
	CreatedBy         string            `yaml:"created_by"`
	CompletedBy       string            `yaml:"completed_by,omitempty"`
	CheckpointCommit  string            `yaml:"checkpoint_commit,omitempty"`
	CheckpointAt      string            `yaml:"checkpoint_at,omitempty"`
	CheckpointMessage string            `yaml:"checkpoint_message,omitempty"`
	CheckpointScope   []string          `yaml:"checkpoint_scope,omitempty"`
	CheckpointChunks  []CheckpointChunk `yaml:"checkpoint_chunks,omitempty"`
	Delegations       []TaskDelegation  `yaml:"delegations,omitempty"`
	Contract          TaskContract      `yaml:"contract,omitempty"`
}

type TaskContract struct {
	Intent             string   `yaml:"intent,omitempty"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	UpdatedAt          string   `yaml:"updated_at,omitempty"`
	UpdatedBy          string   `yaml:"updated_by,omitempty"`
}

type Contract struct {
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
	UpdatedAt          string   `yaml:"updated_at,omitempty"`
	UpdatedBy          string   `yaml:"updated_by,omitempty"`
}

type CR struct {
	ID                int       `yaml:"id"`
	UID               string    `yaml:"uid,omitempty"`
	Title             string    `yaml:"title"`
	Description       string    `yaml:"description"`
	Status            string    `yaml:"status"`
	BaseBranch        string    `yaml:"base_branch"`
	BaseRef           string    `yaml:"base_ref,omitempty"`
	BaseCommit        string    `yaml:"base_commit,omitempty"`
	ParentCRID        int       `yaml:"parent_cr_id,omitempty"`
	Branch            string    `yaml:"branch"`
	Notes             []string  `yaml:"notes"`
	Contract          Contract  `yaml:"contract,omitempty"`
	Subtasks          []Subtask `yaml:"subtasks"`
	Events            []Event   `yaml:"events"`
	MergedAt          string    `yaml:"merged_at,omitempty"`
	MergedBy          string    `yaml:"merged_by,omitempty"`
	MergedCommit      string    `yaml:"merged_commit,omitempty"`
	FilesTouchedCount int       `yaml:"files_touched_count,omitempty"`
	CreatedAt         string    `yaml:"created_at"`
	UpdatedAt         string    `yaml:"updated_at"`
}
