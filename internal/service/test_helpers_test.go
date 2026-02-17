package service

import "testing"

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
