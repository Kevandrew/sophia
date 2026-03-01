package trust

import (
	"sophia/internal/model"
	"testing"
)

func TestParseShortStatMetrics(t *testing.T) {
	t.Parallel()
	got := ParseShortStatMetrics("21 files changed, 995 insertions(+), 70 deletions(-)")
	if got.FilesChanged != 21 || got.Insertions != 995 || got.Deletions != 70 {
		t.Fatalf("ParseShortStatMetrics = %#v, want files=21 ins=995 del=70", got)
	}
}

func TestTrustThresholdForTier(t *testing.T) {
	t.Parallel()
	trust := model.PolicyTrust{
		Thresholds: model.PolicyTrustThresholds{
			Low:    floatPtr(0.7),
			Medium: floatPtr(0.8),
			High:   floatPtr(0.9),
		},
	}
	if got := TrustThresholdForTier(trust, "high", 0.1, 0.2, 0.3); got != 0.9 {
		t.Fatalf("high threshold = %.2f, want 0.9", got)
	}
	if got := TrustThresholdForTier(trust, "low", 0.1, 0.2, 0.3); got != 0.7 {
		t.Fatalf("low threshold = %.2f, want 0.7", got)
	}
}

func floatPtr(v float64) *float64 {
	return &v
}
