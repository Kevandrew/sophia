package model

type RepoPolicy struct {
	Version        string               `yaml:"version"`
	Contract       PolicyContract       `yaml:"contract,omitempty"`
	TaskContract   PolicyTaskContract   `yaml:"task_contract,omitempty"`
	Scope          PolicyScope          `yaml:"scope,omitempty"`
	Classification PolicyClassification `yaml:"classification,omitempty"`
	Merge          PolicyMerge          `yaml:"merge,omitempty"`
	Archive        PolicyArchive        `yaml:"archive,omitempty"`
	Trust          PolicyTrust          `yaml:"trust,omitempty"`
}

type PolicyContract struct {
	RequiredFields []string `yaml:"required_fields,omitempty"`
}

type PolicyTaskContract struct {
	RequiredFields []string `yaml:"required_fields,omitempty"`
}

type PolicyScope struct {
	AllowedPrefixes []string `yaml:"allowed_prefixes,omitempty"`
}

type PolicyClassification struct {
	Test       PolicyClassificationTest       `yaml:"test,omitempty"`
	Dependency PolicyClassificationDependency `yaml:"dependency,omitempty"`
}

type PolicyClassificationTest struct {
	Suffixes     []string `yaml:"suffixes,omitempty"`
	PathContains []string `yaml:"path_contains,omitempty"`
}

type PolicyClassificationDependency struct {
	FileNames []string `yaml:"file_names,omitempty"`
}

type PolicyMerge struct {
	AllowOverride            *bool  `yaml:"allow_override,omitempty"`
	Mode                     string `yaml:"mode,omitempty"`
	RequiredApprovals        *int   `yaml:"required_approvals,omitempty"`
	RequireNonAuthorApproval *bool  `yaml:"require_non_author_approval,omitempty"`
	RequireReadyForReview    *bool  `yaml:"require_ready_for_review,omitempty"`
	RequirePassingChecks     *bool  `yaml:"require_passing_checks,omitempty"`
}

type PolicyArchive struct {
	Enabled          *bool  `yaml:"enabled,omitempty"`
	Path             string `yaml:"path,omitempty"`
	Format           string `yaml:"format,omitempty"`
	IncludeFullDiffs *bool  `yaml:"include_full_diffs,omitempty"`
}

type PolicyTrust struct {
	Mode        string                 `yaml:"mode,omitempty"`
	Gate        PolicyTrustGate        `yaml:"gate,omitempty"`
	Thresholds  PolicyTrustThresholds  `yaml:"thresholds,omitempty"`
	Checks      PolicyTrustChecks      `yaml:"checks,omitempty"`
	ReviewDepth PolicyTrustReviewDepth `yaml:"review_depth,omitempty"`
}

type PolicyTrustGate struct {
	Enabled        *bool    `yaml:"enabled,omitempty"`
	ApplyRiskTiers []string `yaml:"apply_risk_tiers,omitempty"`
	MinVerdict     string   `yaml:"min_verdict,omitempty"`
}

type PolicyTrustThresholds struct {
	Low    *float64 `yaml:"low,omitempty"`
	Medium *float64 `yaml:"medium,omitempty"`
	High   *float64 `yaml:"high,omitempty"`
}

type PolicyTrustChecks struct {
	FreshnessHours *int                         `yaml:"freshness_hours,omitempty"`
	Definitions    []PolicyTrustCheckDefinition `yaml:"definitions,omitempty"`
}

type PolicyTrustCheckDefinition struct {
	Key            string   `yaml:"key,omitempty"`
	Command        string   `yaml:"command,omitempty"`
	Tiers          []string `yaml:"tiers,omitempty"`
	AllowExitCodes []int    `yaml:"allow_exit_codes,omitempty"`
}

type PolicyTrustReviewDepth struct {
	Low    PolicyTrustReviewTier `yaml:"low,omitempty"`
	Medium PolicyTrustReviewTier `yaml:"medium,omitempty"`
	High   PolicyTrustReviewTier `yaml:"high,omitempty"`
}

type PolicyTrustReviewTier struct {
	MinSamples                   *int  `yaml:"min_samples,omitempty"`
	RequireCriticalScopeCoverage *bool `yaml:"require_critical_scope_coverage,omitempty"`
}
