package service

type TrustDimension struct {
	Code            string
	Label           string
	Score           int
	Max             int
	Reasons         []string
	RequiredActions []string
}

type TrustReport struct {
	Verdict          string
	Score            int
	Max              int
	AdvisoryOnly     bool
	HardFailures     []string
	Dimensions       []TrustDimension
	RequiredActions  []string
	Advisories       []string
	Summary          string
	RiskTier         string
	Requirements     []TrustRequirement
	CheckResults     []TrustCheckResult
	ReviewDepth      TrustReviewDepthResult
	ContractDrift    TaskContractDriftSummary
	CRContractDrift  CRContractDriftSummary
	Gate             TrustGateSummary
	AttentionActions []string
}

type TrustRequirement struct {
	Key       string
	Title     string
	Satisfied bool
	Reason    string
	Action    string
	TaskID    int
	Source    string
}

type TrustCheckResult struct {
	Key               string
	Command           string
	Required          bool
	Status            string
	Reason            string
	AllowExitCodes    []int
	ExitCode          *int
	LastRunAt         string
	FreshnessHours    int
	RequiredByTaskIDs []int
	Sources           []string
}

type TrustReviewDepthResult struct {
	RiskTier                     string
	RequiredSamples              int
	SampleCount                  int
	RequireCriticalScopeCoverage bool
	CoveredCriticalScopes        []string
	MissingCriticalScopes        []string
	Satisfied                    bool
}

type TrustGateSummary struct {
	Enabled bool
	Applies bool
	Blocked bool
	Reason  string
}

type TrustCheckStatusReport struct {
	CRID               int
	CRUID              string
	RiskTier           string
	Requirements       []TrustRequirement
	CheckResults       []TrustCheckResult
	FreshnessHours     int
	CheckMode          string
	RequiredCheckCount int
	Guidance           []string
}

type TrustCheckRunReport struct {
	CRID               int
	CRUID              string
	RiskTier           string
	Requirements       []TrustRequirement
	CheckResults       []TrustCheckResult
	Executed           int
	CheckMode          string
	RequiredCheckCount int
	Guidance           []string
}
