package service

import (
	"sophia/internal/model"
	"sort"
	"strings"
)

func crContractBaselineIsEmpty(baseline model.CRContractBaseline) bool {
	return strings.TrimSpace(baseline.CapturedAt) == "" &&
		strings.TrimSpace(baseline.CapturedBy) == "" &&
		len(baseline.Scope) == 0
}

func crContractBaselineFromScope(scope []string, capturedAt, capturedBy string) model.CRContractBaseline {
	return model.CRContractBaseline{
		CapturedAt: strings.TrimSpace(capturedAt),
		CapturedBy: strings.TrimSpace(capturedBy),
		Scope:      append([]string(nil), scope...),
	}
}

func nextCRContractDriftID(drifts []model.CRContractDrift) int {
	maxID := 0
	for _, drift := range drifts {
		if drift.ID > maxID {
			maxID = drift.ID
		}
	}
	return maxID + 1
}

func summarizeCRContractDrift(drifts []model.CRContractDrift) CRContractDriftSummary {
	summary := CRContractDriftSummary{
		DriftIDs:               []int{},
		UnacknowledgedDriftIDs: []int{},
	}
	for _, drift := range drifts {
		summary.Total++
		summary.DriftIDs = append(summary.DriftIDs, drift.ID)
		if !drift.Acknowledged {
			summary.Unacknowledged++
			summary.UnacknowledgedDriftIDs = append(summary.UnacknowledgedDriftIDs, drift.ID)
		}
	}
	sort.Ints(summary.DriftIDs)
	sort.Ints(summary.UnacknowledgedDriftIDs)
	return summary
}
