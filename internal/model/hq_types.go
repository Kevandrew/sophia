package model

import "encoding/json"

const HQSchemaV1 = "sophia.hq.v1"

type HQCRSummary struct {
	UID        string `json:"uid,omitempty"`
	Title      string `json:"title,omitempty"`
	Status     string `json:"status,omitempty"`
	Branch     string `json:"branch,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type HQListCRsResponse struct {
	SchemaVersion string        `json:"schema_version,omitempty"`
	Summaries     []HQCRSummary `json:"summaries,omitempty"`
	Items         []HQCRSummary `json:"items,omitempty"`
}

type HQGetCRResponse struct {
	SchemaVersion string          `json:"schema_version,omitempty"`
	CRUID         string          `json:"cr_uid,omitempty"`
	CRFingerprint string          `json:"cr_fingerprint,omitempty"`
	Doc           json.RawMessage `json:"doc,omitempty"`
	CR            json.RawMessage `json:"cr,omitempty"`
}

type HQPatchApplyRequest struct {
	SchemaVersion string  `json:"schema_version"`
	Patch         CRPatch `json:"patch"`
}

type HQPatchConflict struct {
	OpIndex int    `json:"op_index"`
	Op      string `json:"op,omitempty"`
	Field   string `json:"field,omitempty"`
	Message string `json:"message,omitempty"`
}

type HQPatchApplyResponse struct {
	SchemaVersion string            `json:"schema_version,omitempty"`
	CRUID         string            `json:"cr_uid,omitempty"`
	CRFingerprint string            `json:"cr_fingerprint,omitempty"`
	AppliedOps    []int             `json:"applied_ops,omitempty"`
	SkippedOps    []int             `json:"skipped_ops,omitempty"`
	Conflicts     []HQPatchConflict `json:"conflicts,omitempty"`
	Warnings      []string          `json:"warnings,omitempty"`
}
