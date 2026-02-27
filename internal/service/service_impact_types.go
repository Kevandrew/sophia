package service

type RiskSignal struct {
	Code    string
	Summary string
	Points  int
}

type ImpactReport struct {
	CRID                      int
	CRUID                     string
	BaseRef                   string
	BaseCommit                string
	ParentCRID                int
	RiskTierHint              string
	RiskTierFloorApplied      bool
	MatchedRiskCriticalScopes []string
	FilesChanged              int
	NewFiles                  []string
	ModifiedFiles             []string
	DeletedFiles              []string
	TestFiles                 []string
	DependencyFiles           []string
	ScopeDrift                []string
	Warnings                  []string
	TaskScopeWarnings         []string
	TaskContractWarnings      []string
	TaskChunkWarnings         []string
	Signals                   []RiskSignal
	RiskScore                 int
	RiskTier                  string
}
