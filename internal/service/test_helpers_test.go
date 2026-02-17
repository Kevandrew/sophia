package service

import (
	"path/filepath"
	"testing"
)

func setValidContract(t *testing.T, svc *Service, crID int) {
	t.Helper()
	scope := []string{"."}
	nonGoals := []string{"No unrelated refactors"}
	invariants := []string{"Existing behavior stays compatible"}
	why := "Deliver scoped intent safely."
	blast := "Limited to this CR branch."
	testPlan := "Run go test ./... and go vet ./..."
	rollback := "Revert merge commit."

	_, err := svc.SetCRContract(crID, ContractPatch{
		Why:          &why,
		Scope:        &scope,
		NonGoals:     &nonGoals,
		Invariants:   &invariants,
		BlastRadius:  &blast,
		TestPlan:     &testPlan,
		RollbackPlan: &rollback,
	})
	if err != nil {
		t.Fatalf("SetCRContract() error = %v", err)
	}
}

func setValidTaskContract(t *testing.T, svc *Service, crID, taskID int) {
	t.Helper()
	intent := "Implement scoped task outcome."
	acceptance := []string{"Behavior works as specified."}
	scope := []string{"."}

	_, err := svc.SetTaskContract(crID, taskID, TaskContractPatch{
		Intent:             &intent,
		AcceptanceCriteria: &acceptance,
		Scope:              &scope,
	})
	if err != nil {
		t.Fatalf("SetTaskContract() error = %v", err)
	}
}

func localMetadataDir(t *testing.T, dir string) string {
	t.Helper()
	commonDir := runGit(t, dir, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(commonDir) {
		commonDir = filepath.Join(dir, commonDir)
	}
	return filepath.Join(commonDir, "sophia-local")
}
