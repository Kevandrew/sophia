package service

type CRDoctorFinding struct {
	Code    string
	Message string
	TaskID  int
	Commit  string
}

type CRDoctorReport struct {
	CRID             int
	CRUID            string
	Branch           string
	BranchExists     bool
	BranchHead       string
	BaseRef          string
	BaseCommit       string
	ResolvedBaseRef  string
	ParentCRID       int
	ExpectedParentID int
	Findings         []CRDoctorFinding
}

type ReconcileCROptions struct {
	Regenerate bool
}

type ReconcileTaskResult struct {
	TaskID           int
	Title            string
	Status           string
	PreviousCommit   string
	CurrentCommit    string
	Action           string
	Reason           string
	Source           string
	CheckpointAt     string
	CheckpointOrphan bool
}

type ReconcileCRReport struct {
	CRID             int
	CRUID            string
	Branch           string
	BranchExists     bool
	PreviousParentID int
	CurrentParentID  int
	ParentRelinked   bool
	ScanRef          string
	ScannedCommits   int
	Relinked         int
	Orphaned         int
	ClearedOrphans   int
	Regenerated      bool
	FilesChanged     int
	DiffStat         string
	Warnings         []string
	Findings         []CRDoctorFinding
	TaskResults      []ReconcileTaskResult
}

type ValidationReport struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Impact   *ImpactReport
}
