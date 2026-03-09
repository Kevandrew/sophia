export interface DashboardFilterState {
  status: string;
  risk_tier: string;
  scope: string;
  text: string;
  selected_cr_id: number | null;
}

export interface DashboardCounts {
  list_total: number;
  list_returned: number;
  timeline_total: number;
  timeline_returned: number;
}

export interface StackNativity {
  is_root_parent?: boolean;
  is_aggregate_parent?: boolean;
  is_child?: boolean;
  parent_cr_id?: number;
  parent_title?: string;
  child_count?: number;
  resolved_child_count?: number;
  pending_child_count?: number;
  role_label?: string;
}

export interface StackLineageNode {
  id: number;
  uid?: string;
  title?: string;
  status?: string;
  branch?: string;
  base_ref?: string;
  depth?: number;
  role?: string;
  role_label?: string;
}

export interface StackTreeNode {
  cr_id?: number;
  title?: string;
  status?: string;
  branch?: string;
  risk_tier?: string;
  role_label?: string;
  tasks_total?: number;
  tasks_open?: number;
  tasks_done?: number;
  children?: StackTreeNode[];
}

export interface DashboardCRRow {
  id: number;
  uid: string;
  title: string;
  description: string;
  status: string;
  branch: string;
  base_branch: string;
  base_ref: string;
  base_commit: string;
  parent_cr_id?: number;
  risk_tier: string;
  created_at: string;
  updated_at: string;
  last_event_at: string;
  contract_why: string;
  contract_scope: string[];
  contract_non_goals: string[];
  contract_invariants: string[];
  stack_nativity?: StackNativity;
  stack_lineage?: StackLineageNode[];
  stack_tree?: StackTreeNode;
  tasks: {
    total: number;
    open: number;
    done: number;
  };
  lifecycle_state?: string;
  abandoned_at?: string;
  abandoned_by?: string;
  abandoned_reason?: string;
  pr_linkage_state?: string;
  action_required?: string;
  action_reason?: string;
  suggested_commands?: string[];
}

export interface DashboardTimelineEvent {
  ts: string;
  type: string;
  summary: string;
  actor: string;
  ref: string;
  redacted: boolean;
  cr_id: number;
  cr_uid: string;
  cr_title: string;
  cr_status: string;
}

export interface DashboardSnapshot {
  generated_at: string;
  dashboard: {
    selected_cr_id?: number | null;
    filters: {
      status: string;
      risk_tier: string;
      scope: string;
      text: string;
      list_limit: number;
      timeline_limit: number;
    };
    counts: DashboardCounts;
  };
  crs: DashboardCRRow[];
  timeline: DashboardTimelineEvent[];
  selected_cr?: DashboardCRRow | null;
}

export interface ValidationReport {
  valid?: boolean;
  errors?: string[];
  warnings?: string[];
}

export interface TrustReport {
  verdict?: string;
  risk_tier?: string;
  summary?: string;
  advisories?: string[];
  attention_actions?: string[];
  required_actions?: string[];
}

export interface ImpactReport {
  risk_tier?: string;
  risk_score?: number;
  files_changed?: number;
  warnings?: string[];
  scope_drift?: string[];
}

export interface CRDetailRecord {
  id: number;
  uid: string;
  title: string;
  description: string;
  status: string;
  lifecycle_state: string;
  branch: string;
  base_branch: string;
  base_ref: string;
  base_commit: string;
  parent_cr_id?: number;
  created_at: string;
  updated_at: string;
  notes: string[];
  events: RecentEvent[];
  contract: CRContract;
  pr?: {
    url?: string;
    number?: number;
    state?: string;
  };
}

export interface CRContract {
  why?: string;
  scope?: string[];
  non_goals?: string[];
  invariants?: string[];
  blast_radius?: string;
  test_plan?: string;
  rollback_plan?: string;
  risk_tier_hint?: string;
}

export interface PreviewTask {
  id: number;
  title: string;
  status: string;
  contract?: {
    intent?: string;
    scope?: string[];
    acceptance_criteria?: string[];
    acceptance_checks?: string[];
  };
  checkpoint_commit?: string;
  checkpoint_at?: string;
  checkpoint_scope?: string[];
  checkpoint_message?: string;
  checkpoint_reason?: string;
}

export interface RecentEvent {
  ts: string;
  actor: string;
  type: string;
  summary: string;
  ref: string;
  meta?: Record<string, string>;
}

export interface RecentCheckpoint {
  task_id: number;
  title: string;
  status: string;
  commit: string;
  at: string;
  message: string;
  scope?: string[];
  source?: string;
  orphan?: boolean;
  reason?: string;
}

export interface SliceMeta {
  total?: number;
  returned?: number;
  truncated?: boolean;
}

export interface DelegationRun {
  id?: string;
  status?: string;
  created_at?: string;
  updated_at?: string;
  finished_at?: string;
  request?: {
    task_ids?: number[];
    runtime?: string;
  };
  result?: {
    summary?: string;
    validation_errors?: string[];
    validation_warnings?: string[];
  };
}

export interface DelegationSnapshot {
  current_run?: DelegationRun;
  recent_runs?: DelegationRun[];
  counts?: {
    total?: number;
    running?: number;
    terminal?: number;
  };
}

export interface DelegationLaunch {
  available?: boolean;
  reason?: string;
  runtime?: string;
  default_task_ids?: number[];
  open_task_ids?: number[];
  all_task_ids?: number[];
  skill_refs?: string[];
}

export interface StatusSnapshot {
  status?: string;
  lifecycle_state?: string;
  pr_linkage_state?: string;
  action_required?: string;
  action_reason?: string;
  suggested_commands?: string[];
  tasks?: {
    total?: number;
    open?: number;
    done?: number;
    delegated?: number;
    delegated_pending?: number;
  };
}

export interface CRSnapshot {
  generated_at: string;
  cr: CRDetailRecord;
  contract?: CRContract;
  tasks: PreviewTask[];
  delegation?: DelegationSnapshot;
  delegation_launch?: DelegationLaunch;
  stack_nativity?: StackNativity;
  stack_lineage?: StackLineageNode[];
  stack_tree?: StackTreeNode;
  status?: StatusSnapshot;
  recent_events: RecentEvent[];
  events_meta?: SliceMeta;
  recent_checkpoints: RecentCheckpoint[];
  checkpoints_meta?: SliceMeta;
  diff_stat?: string;
  files_changed?: string[];
  impact?: ImpactReport;
  validation?: ValidationReport;
  trust?: TrustReport;
  warnings?: string[];
}

export type PreviewRoute =
  | {
      kind: 'dashboard';
      status: string;
      risk_tier: string;
      scope: string;
      text: string;
      selected_cr_id?: number;
    }
  | {
      kind: 'cr';
      cr_id: number;
    };
