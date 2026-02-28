package model

const (
	StatusInProgress = "in_progress"
	StatusMerged     = "merged"

	TaskStatusOpen      = "open"
	TaskStatusDone      = "done"
	TaskStatusDelegated = "delegated"

	TaskCheckpointSourceTaskCheckpoint   = "task_checkpoint"
	TaskCheckpointSourceTaskNoCheckpoint = "task_no_checkpoint"

	TaskScopeModeAll           = "all"
	TaskScopeModeTaskContract  = "task_contract"
	TaskScopeModePatchManifest = "patch_manifest"
	TaskScopeModePath          = "path"

	EventTypeCRCreated         = "cr_created"
	EventTypeCRAmended         = "cr_amended"
	EventTypeCRBaseUpdated     = "cr_base_updated"
	EventTypeCRRestacked       = "cr_restacked"
	EventTypeCRParentMerged    = "cr_parent_merged"
	EventTypeCRRedacted        = "cr_redacted"
	EventTypeCRValidated       = "cr_validated"
	EventTypeCRRepaired        = "cr_repaired"
	EventTypeCRReconciled      = "cr_reconciled"
	EventTypeCRMergeConflict   = "cr_merge_conflict"
	EventTypeCRMergeAborted    = "cr_merge_aborted"
	EventTypeCRMergeResumed    = "cr_merge_resumed"
	EventTypeCRMergeOverridden = "cr_merge_overridden"
	EventTypeCRMerged          = "cr_merged"
	EventTypeCRReopened        = "cr_reopened"
	EventTypeCRPROpened        = "cr_pr_opened"
	EventTypeCRPRSynced        = "cr_pr_synced"
	EventTypeCRPRReady         = "cr_pr_ready"
	EventTypeCRPRMergedRemote  = "cr_pr_merged_remote"

	EventTypeNoteAdded                   = "note_added"
	EventTypeContractUpdated             = "contract_updated"
	EventTypeCRContractBaselineCaptured  = "cr_contract_baseline_captured"
	EventTypeCRContractDriftRecorded     = "cr_contract_drift_recorded"
	EventTypeCRContractDriftAcknowledged = "cr_contract_drift_acknowledged"

	EventTypeTaskAdded                     = "task_added"
	EventTypeTaskDone                      = "task_done"
	EventTypeTaskDoneAuto                  = "task_done_auto"
	EventTypeTaskCheckpointed              = "task_checkpointed"
	EventTypeTaskReopened                  = "task_reopened"
	EventTypeTaskDelegated                 = "task_delegated"
	EventTypeTaskDelegationReceived        = "task_delegation_received"
	EventTypeTaskUndelegated               = "task_undelegated"
	EventTypeTaskContractUpdated           = "task_contract_updated"
	EventTypeTaskContractDriftRecorded     = "task_contract_drift_recorded"
	EventTypeTaskContractDriftAcknowledged = "task_contract_drift_acknowledged"

	EventTypeEvidenceAdded = "evidence_added"
	EventTypePatchApplied  = "patch_applied"

	EventTypeHQSynced = "hq_synced"
	EventTypeHQPulled = "hq_pulled"
	EventTypeHQPushed = "hq_pushed"

	MetadataModeLocal   = "local"
	MetadataModeTracked = "tracked"
)

type Config struct {
	Version           string `yaml:"version"`
	BaseBranch        string `yaml:"base_branch"`
	MetadataMode      string `yaml:"metadata_mode,omitempty"`
	BranchOwnerPrefix string `yaml:"branch_owner_prefix,omitempty"`
	HQRemote          string `yaml:"hq_remote,omitempty"`
	HQRepoID          string `yaml:"hq_repo_id,omitempty"`
	HQBaseURL         string `yaml:"hq_base_url,omitempty"`
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

type EvidenceEntry struct {
	TS          string   `yaml:"ts"`
	Actor       string   `yaml:"actor"`
	Type        string   `yaml:"type"`
	Scope       string   `yaml:"scope,omitempty"`
	Command     string   `yaml:"command,omitempty"`
	ExitCode    *int     `yaml:"exit_code,omitempty"`
	OutputHash  string   `yaml:"output_hash,omitempty"`
	Summary     string   `yaml:"summary"`
	Attachments []string `yaml:"attachments,omitempty"`
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
	ID                int                  `yaml:"id"`
	Title             string               `yaml:"title"`
	Status            string               `yaml:"status"`
	CreatedAt         string               `yaml:"created_at"`
	UpdatedAt         string               `yaml:"updated_at"`
	CompletedAt       string               `yaml:"completed_at,omitempty"`
	CreatedBy         string               `yaml:"created_by"`
	CompletedBy       string               `yaml:"completed_by,omitempty"`
	CheckpointCommit  string               `yaml:"checkpoint_commit,omitempty"`
	CheckpointAt      string               `yaml:"checkpoint_at,omitempty"`
	CheckpointMessage string               `yaml:"checkpoint_message,omitempty"`
	CheckpointScope   []string             `yaml:"checkpoint_scope,omitempty"`
	CheckpointChunks  []CheckpointChunk    `yaml:"checkpoint_chunks,omitempty"`
	CheckpointOrphan  bool                 `yaml:"checkpoint_orphan,omitempty"`
	CheckpointReason  string               `yaml:"checkpoint_reason,omitempty"`
	CheckpointSource  string               `yaml:"checkpoint_source,omitempty"`
	CheckpointSyncAt  string               `yaml:"checkpoint_sync_at,omitempty"`
	Delegations       []TaskDelegation     `yaml:"delegations,omitempty"`
	Contract          TaskContract         `yaml:"contract,omitempty"`
	ContractBaseline  TaskContractBaseline `yaml:"contract_baseline,omitempty"`
	ContractDrifts    []TaskContractDrift  `yaml:"contract_drifts,omitempty"`
}

type TaskContract struct {
	Intent             string   `yaml:"intent,omitempty"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	AcceptanceChecks   []string `yaml:"acceptance_checks,omitempty"`
	UpdatedAt          string   `yaml:"updated_at,omitempty"`
	UpdatedBy          string   `yaml:"updated_by,omitempty"`
}

type TaskContractBaseline struct {
	CapturedAt         string   `yaml:"captured_at,omitempty"`
	CapturedBy         string   `yaml:"captured_by,omitempty"`
	Intent             string   `yaml:"intent,omitempty"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty"`
	Scope              []string `yaml:"scope,omitempty"`
	AcceptanceChecks   []string `yaml:"acceptance_checks,omitempty"`
}

type TaskContractDrift struct {
	ID                     int      `yaml:"id"`
	TS                     string   `yaml:"ts"`
	Actor                  string   `yaml:"actor"`
	Fields                 []string `yaml:"fields,omitempty"`
	BeforeScope            []string `yaml:"before_scope,omitempty"`
	AfterScope             []string `yaml:"after_scope,omitempty"`
	BeforeAcceptanceChecks []string `yaml:"before_acceptance_checks,omitempty"`
	AfterAcceptanceChecks  []string `yaml:"after_acceptance_checks,omitempty"`
	CheckpointCommit       string   `yaml:"checkpoint_commit,omitempty"`
	Reason                 string   `yaml:"reason,omitempty"`
	Acknowledged           bool     `yaml:"acknowledged,omitempty"`
	AcknowledgedAt         string   `yaml:"acknowledged_at,omitempty"`
	AcknowledgedBy         string   `yaml:"acknowledged_by,omitempty"`
	AckReason              string   `yaml:"ack_reason,omitempty"`
}

type CRContractBaseline struct {
	CapturedAt string   `yaml:"captured_at,omitempty"`
	CapturedBy string   `yaml:"captured_by,omitempty"`
	Scope      []string `yaml:"scope,omitempty"`
}

type CRContractDrift struct {
	ID             int      `yaml:"id"`
	TS             string   `yaml:"ts"`
	Actor          string   `yaml:"actor"`
	Fields         []string `yaml:"fields,omitempty"`
	BeforeScope    []string `yaml:"before_scope,omitempty"`
	AfterScope     []string `yaml:"after_scope,omitempty"`
	Reason         string   `yaml:"reason,omitempty"`
	Acknowledged   bool     `yaml:"acknowledged,omitempty"`
	AcknowledgedAt string   `yaml:"acknowledged_at,omitempty"`
	AcknowledgedBy string   `yaml:"acknowledged_by,omitempty"`
	AckReason      string   `yaml:"ack_reason,omitempty"`
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

type HQIntentContractSnapshot struct {
	Why                string   `yaml:"why,omitempty" json:"why,omitempty"`
	Scope              []string `yaml:"scope,omitempty" json:"scope,omitempty"`
	NonGoals           []string `yaml:"non_goals,omitempty" json:"non_goals,omitempty"`
	Invariants         []string `yaml:"invariants,omitempty" json:"invariants,omitempty"`
	BlastRadius        string   `yaml:"blast_radius,omitempty" json:"blast_radius,omitempty"`
	RiskCriticalScopes []string `yaml:"risk_critical_scopes,omitempty" json:"risk_critical_scopes,omitempty"`
	RiskTierHint       string   `yaml:"risk_tier_hint,omitempty" json:"risk_tier_hint,omitempty"`
	RiskRationale      string   `yaml:"risk_rationale,omitempty" json:"risk_rationale,omitempty"`
	TestPlan           string   `yaml:"test_plan,omitempty" json:"test_plan,omitempty"`
	RollbackPlan       string   `yaml:"rollback_plan,omitempty" json:"rollback_plan,omitempty"`
}

type HQIntentTaskContractSnapshot struct {
	Intent             string   `yaml:"intent,omitempty" json:"intent,omitempty"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria,omitempty" json:"acceptance_criteria,omitempty"`
	Scope              []string `yaml:"scope,omitempty" json:"scope,omitempty"`
	AcceptanceChecks   []string `yaml:"acceptance_checks,omitempty" json:"acceptance_checks,omitempty"`
}

type HQIntentTaskSnapshot struct {
	ID       int                          `yaml:"id" json:"id"`
	Title    string                       `yaml:"title,omitempty" json:"title,omitempty"`
	Status   string                       `yaml:"status,omitempty" json:"status,omitempty"`
	Contract HQIntentTaskContractSnapshot `yaml:"contract,omitempty" json:"contract,omitempty"`
}

type HQIntentSnapshot struct {
	Title       string                   `yaml:"title,omitempty" json:"title,omitempty"`
	Description string                   `yaml:"description,omitempty" json:"description,omitempty"`
	Status      string                   `yaml:"status,omitempty" json:"status,omitempty"`
	Contract    HQIntentContractSnapshot `yaml:"contract,omitempty" json:"contract,omitempty"`
	Notes       []string                 `yaml:"notes,omitempty" json:"notes,omitempty"`
	Subtasks    []HQIntentTaskSnapshot   `yaml:"subtasks,omitempty" json:"subtasks,omitempty"`
}

type CRHQState struct {
	RemoteAlias         string            `yaml:"remote_alias,omitempty"`
	RepoID              string            `yaml:"repo_id,omitempty"`
	UpstreamFingerprint string            `yaml:"upstream_fingerprint,omitempty"`
	UpstreamIntent      *HQIntentSnapshot `yaml:"upstream_intent,omitempty"`
	LastPullAt          string            `yaml:"last_pull_at,omitempty"`
	LastPushAt          string            `yaml:"last_push_at,omitempty"`
}

type CRPRLink struct {
	Provider                 string   `yaml:"provider,omitempty"`
	Repo                     string   `yaml:"repo,omitempty"`
	Number                   int      `yaml:"number,omitempty"`
	URL                      string   `yaml:"url,omitempty"`
	State                    string   `yaml:"state,omitempty"`
	Draft                    bool     `yaml:"draft,omitempty"`
	LastHeadSHA              string   `yaml:"last_head_sha,omitempty"`
	LastBaseRef              string   `yaml:"last_base_ref,omitempty"`
	LastBodyHash             string   `yaml:"last_body_hash,omitempty"`
	LastSyncedAt             string   `yaml:"last_synced_at,omitempty"`
	LastStatusCheckedAt      string   `yaml:"last_status_checked_at,omitempty"`
	LastMergedAt             string   `yaml:"last_merged_at,omitempty"`
	LastMergedCommit         string   `yaml:"last_merged_commit,omitempty"`
	CheckpointCommentKeys    []string `yaml:"checkpoint_comment_keys,omitempty"`
	CheckpointSyncKeys       []string `yaml:"checkpoint_sync_keys,omitempty"`
	AwaitingOpenApproval     bool     `yaml:"awaiting_open_approval,omitempty"`
	AwaitingOpenApprovalNote string   `yaml:"awaiting_open_approval_note,omitempty"`
}

type CR struct {
	ID                int                `yaml:"id"`
	UID               string             `yaml:"uid,omitempty"`
	Title             string             `yaml:"title"`
	Description       string             `yaml:"description"`
	Status            string             `yaml:"status"`
	BaseBranch        string             `yaml:"base_branch"`
	BaseRef           string             `yaml:"base_ref,omitempty"`
	BaseCommit        string             `yaml:"base_commit,omitempty"`
	ParentCRID        int                `yaml:"parent_cr_id,omitempty"`
	Branch            string             `yaml:"branch"`
	Notes             []string           `yaml:"notes"`
	Evidence          []EvidenceEntry    `yaml:"evidence,omitempty"`
	Contract          Contract           `yaml:"contract,omitempty"`
	ContractBaseline  CRContractBaseline `yaml:"contract_baseline,omitempty"`
	ContractDrifts    []CRContractDrift  `yaml:"contract_drifts,omitempty"`
	Subtasks          []Subtask          `yaml:"subtasks"`
	Events            []Event            `yaml:"events"`
	MergedAt          string             `yaml:"merged_at,omitempty"`
	MergedBy          string             `yaml:"merged_by,omitempty"`
	MergedCommit      string             `yaml:"merged_commit,omitempty"`
	FilesTouchedCount int                `yaml:"files_touched_count,omitempty"`
	HQ                CRHQState          `yaml:"hq,omitempty"`
	PR                CRPRLink           `yaml:"pr,omitempty"`
	CreatedAt         string             `yaml:"created_at"`
	UpdatedAt         string             `yaml:"updated_at"`
}

type CRSearchQuery struct {
	Status      string
	ScopePrefix string
	RiskTier    string
	Text        string
}

type CRSearchResult struct {
	ID         int
	UID        string
	Title      string
	Status     string
	Branch     string
	BaseBranch string
	ParentCRID int
	RiskTier   string
	TasksTotal int
	TasksOpen  int
	TasksDone  int
	CreatedAt  string
	UpdatedAt  string
}
