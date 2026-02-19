package model

import "encoding/json"

const (
	CRPatchSchemaV1 = "sophia.cr_patch.v1"
	CRDocSchemaV1   = "sophia.cr_doc.v1"
)

type CRPatch struct {
	SchemaVersion string            `json:"schema_version"`
	Target        CRPatchTarget     `json:"target"`
	Base          CRPatchBase       `json:"base"`
	Ops           []json.RawMessage `json:"ops"`
	Meta          CRPatchMeta       `json:"meta,omitempty"`
}

type CRPatchTarget struct {
	CRUID string `json:"cr_uid"`
}

type CRPatchBase struct {
	CRFingerprint string `json:"cr_fingerprint"`
	ExportedAt    string `json:"exported_at,omitempty"`
}

type CRPatchMeta struct {
	Author  string `json:"author,omitempty"`
	Tool    string `json:"tool,omitempty"`
	Message string `json:"message,omitempty"`
}
