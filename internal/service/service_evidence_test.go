package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sophia/internal/model"
	"strings"
	"testing"
)

func TestAddEvidenceManualNoteAndHistory(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Evidence CR", "manual evidence"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "evidence.txt"), []byte("artifact\n"), 0o644); err != nil {
		t.Fatalf("write attachment: %v", err)
	}

	entry, err := svc.AddEvidence(1, AddEvidenceOptions{
		Type:        "manual_note",
		Scope:       "internal/service/service_cr.go",
		Summary:     "Reviewed edge case behavior.",
		Attachments: []string{"evidence.txt"},
	})
	if err != nil {
		t.Fatalf("AddEvidence() error = %v", err)
	}
	if entry.Type != "manual_note" {
		t.Fatalf("expected manual_note type, got %q", entry.Type)
	}
	if entry.ExitCode != nil {
		t.Fatalf("expected nil exit code for manual note, got %v", *entry.ExitCode)
	}
	if len(entry.Attachments) != 1 || entry.Attachments[0] != "evidence.txt" {
		t.Fatalf("unexpected attachments: %#v", entry.Attachments)
	}

	loaded, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if len(loaded.Evidence) != 1 {
		t.Fatalf("expected one evidence entry, got %#v", loaded.Evidence)
	}
	if loaded.Events[len(loaded.Events)-1].Type != model.EventTypeEvidenceAdded {
		t.Fatalf("expected evidence_added event, got %#v", loaded.Events[len(loaded.Events)-1])
	}

	history, err := svc.HistoryCR(1, false)
	if err != nil {
		t.Fatalf("HistoryCR() error = %v", err)
	}
	if len(history.Evidence) != 1 || history.Evidence[0].Index != 1 {
		t.Fatalf("expected indexed history evidence, got %#v", history.Evidence)
	}
}

func TestAddEvidenceCommandCapture(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Evidence Capture", "capture evidence"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	entry, err := svc.AddEvidence(1, AddEvidenceOptions{
		Type:    "command_run",
		Command: "printf 'capture-ok\\n'; exit 3",
		Capture: true,
		Scope:   "internal/service",
	})
	if err != nil {
		t.Fatalf("AddEvidence(capture) error = %v", err)
	}
	if entry.ExitCode == nil || *entry.ExitCode != 3 {
		t.Fatalf("expected captured exit code 3, got %#v", entry.ExitCode)
	}
	if strings.TrimSpace(entry.OutputHash) == "" {
		t.Fatalf("expected non-empty output hash")
	}
	if !strings.Contains(entry.Summary, "exited 3") {
		t.Fatalf("expected summary to include exit code, got %q", entry.Summary)
	}
	if !strings.Contains(entry.Summary, "capture-ok") {
		t.Fatalf("expected summary to include first output line, got %q", entry.Summary)
	}
}

func TestAddEvidenceRejectsInvalidTypeAndCaptureMismatch(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Evidence Invalid", "invalid evidence"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}

	if _, err := svc.AddEvidence(1, AddEvidenceOptions{Type: "unknown", Summary: "x"}); err == nil {
		t.Fatalf("expected invalid type error")
	}
	if _, err := svc.AddEvidence(1, AddEvidenceOptions{Type: "manual_note", Command: "echo x", Capture: true}); err == nil {
		t.Fatalf("expected capture/type mismatch error")
	}
}

func TestAddEvidenceCaptureDoesNotExecuteCommandWhenCRMissing(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	marker := filepath.Join(dir, "capture-side-effect.txt")
	command := fmt.Sprintf("echo sideeffect > %q", marker)
	if _, err := svc.AddEvidence(999, AddEvidenceOptions{
		Type:    "command_run",
		Command: command,
		Capture: true,
		Summary: "should not execute",
	}); err == nil {
		t.Fatalf("expected AddEvidence() to fail for missing CR")
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("expected capture command side effects to be skipped for missing CR, stat error: %v", statErr)
	}
}
