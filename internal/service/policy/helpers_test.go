package policy

import (
	"errors"
	"testing"
)

func TestNormalizeArchivePath(t *testing.T) {
	t.Parallel()
	got, err := NormalizeArchivePath(".sophia-tracked/cr")
	if err != nil {
		t.Fatalf("NormalizeArchivePath valid path error: %v", err)
	}
	if got != ".sophia-tracked/cr" {
		t.Fatalf("NormalizeArchivePath = %q, want .sophia-tracked/cr", got)
	}
}

func TestNormalizeArchivePathRejectsEscape(t *testing.T) {
	t.Parallel()
	_, err := NormalizeArchivePath("../outside")
	if err == nil {
		t.Fatalf("expected error for escaping archive path")
	}
}

func TestParseUnknownFieldsParsesStrictYAMLErrors(t *testing.T) {
	t.Parallel()
	err := errors.New("yaml: unmarshal errors:\n  line 2: field future_option not found in type model.RepoPolicy")
	fields, ok := ParseUnknownFields(err)
	if !ok {
		t.Fatalf("expected unknown-field parse to succeed")
	}
	if len(fields) != 1 || fields[0] != "future_option" {
		t.Fatalf("fields = %#v, want [future_option]", fields)
	}
}
