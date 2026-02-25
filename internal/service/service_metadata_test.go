package service

import (
	"errors"
	"os"
	"path/filepath"
	"sophia/internal/model"
	"strings"
	"testing"
)

func TestEditCRMergedAndNoOpValidation(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	cr, err := svc.AddCR("Old title", "old description")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, dir, "add", "f.txt")
	runGit(t, dir, "commit", "-m", "feat: content")
	setValidContract(t, svc, cr.ID)
	if _, err := svc.MergeCR(cr.ID, false, ""); err != nil {
		t.Fatalf("MergeCR() error = %v", err)
	}

	newTitle := "Intent integrity and visibility layer"
	changed, err := svc.EditCR(cr.ID, &newTitle, nil)
	if err != nil {
		t.Fatalf("EditCR() error = %v", err)
	}
	if len(changed) != 1 || changed[0] != "title" {
		t.Fatalf("unexpected changed fields: %#v", changed)
	}

	loaded, err := svc.store.LoadCR(cr.ID)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if loaded.Title != newTitle {
		t.Fatalf("expected title %q, got %q", newTitle, loaded.Title)
	}
	last := loaded.Events[len(loaded.Events)-1]
	if last.Type != model.EventTypeCRAmended {
		t.Fatalf("expected cr_amended event, got %#v", last)
	}
	if last.Meta == nil || last.Meta["fields"] != "title" {
		t.Fatalf("expected amended fields metadata, got %#v", last.Meta)
	}

	_, err = svc.EditCR(cr.ID, &newTitle, nil)
	if !errors.Is(err, ErrNoCRChanges) {
		t.Fatalf("expected ErrNoCRChanges, got %v", err)
	}
}

func TestRedactNoteAndEventWithAuditTrail(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("Redact", "redaction"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := svc.AddNote(1, "sensitive note text"); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}

	if err := svc.RedactCRNote(1, 1, "contains internal wording"); err != nil {
		t.Fatalf("RedactCRNote() error = %v", err)
	}
	cr, err := svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() error = %v", err)
	}
	if cr.Notes[0] != redactedPlaceholder {
		t.Fatalf("expected note placeholder, got %q", cr.Notes[0])
	}
	last := cr.Events[len(cr.Events)-1]
	if last.Type != model.EventTypeCRRedacted || last.Ref != "note:1" {
		t.Fatalf("expected note redaction event, got %#v", last)
	}
	if strings.TrimSpace(last.RedactionReason) == "" {
		t.Fatalf("expected redaction reason to be stored")
	}

	if err := svc.RedactCRNote(1, 1, "repeat"); !errors.Is(err, ErrAlreadyRedacted) {
		t.Fatalf("expected ErrAlreadyRedacted, got %v", err)
	}
	if err := svc.RedactCRNote(1, 2, "oob"); err == nil {
		t.Fatalf("expected out-of-range error for note index")
	}
	if err := svc.RedactCRNote(1, 1, ""); err == nil {
		t.Fatalf("expected empty reason error")
	}

	if err := svc.RedactCREvent(1, 1, "sensitive summary"); err != nil {
		t.Fatalf("RedactCREvent() error = %v", err)
	}
	cr, err = svc.store.LoadCR(1)
	if err != nil {
		t.Fatalf("LoadCR() second error = %v", err)
	}
	if cr.Events[0].Summary != redactedPlaceholder || !cr.Events[0].Redacted {
		t.Fatalf("expected first event redacted, got %#v", cr.Events[0])
	}
	if err := svc.RedactCREvent(1, 1, "repeat"); !errors.Is(err, ErrAlreadyRedacted) {
		t.Fatalf("expected ErrAlreadyRedacted for event, got %v", err)
	}
	if err := svc.RedactCREvent(1, 999, "oob"); err == nil {
		t.Fatalf("expected out-of-range error for event index")
	}
}

func TestHistoryCRIndexesAndRedactionVisibility(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	if _, err := svc.AddCR("History", "history flow"); err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := svc.AddNote(1, "note one"); err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
	if err := svc.RedactCRNote(1, 1, "cleanup"); err != nil {
		t.Fatalf("RedactCRNote() error = %v", err)
	}
	if err := svc.RedactCREvent(1, 1, "cleanup event"); err != nil {
		t.Fatalf("RedactCREvent() error = %v", err)
	}

	historyDefault, err := svc.HistoryCR(1, false)
	if err != nil {
		t.Fatalf("HistoryCR(false) error = %v", err)
	}
	if len(historyDefault.Notes) == 0 || historyDefault.Notes[0].Index != 1 {
		t.Fatalf("expected indexed notes, got %#v", historyDefault.Notes)
	}
	if historyDefault.Notes[0].Text != redactedPlaceholder || !historyDefault.Notes[0].Redacted {
		t.Fatalf("expected redacted note placeholder, got %#v", historyDefault.Notes[0])
	}
	if strings.Contains(historyDefault.Notes[0].Text, "note one") {
		t.Fatalf("expected original note payload hidden")
	}
	if len(historyDefault.Events) == 0 || historyDefault.Events[0].Index != 1 {
		t.Fatalf("expected indexed events, got %#v", historyDefault.Events)
	}
	if historyDefault.Events[0].Summary != redactedPlaceholder {
		t.Fatalf("expected redacted event placeholder, got %#v", historyDefault.Events[0])
	}
	if historyDefault.Events[0].RedactionReason != "" {
		t.Fatalf("expected redaction reason hidden by default")
	}

	historyWithMeta, err := svc.HistoryCR(1, true)
	if err != nil {
		t.Fatalf("HistoryCR(true) error = %v", err)
	}
	if historyWithMeta.Events[0].RedactionReason == "" {
		t.Fatalf("expected redaction reason when show-redacted=true")
	}
}
