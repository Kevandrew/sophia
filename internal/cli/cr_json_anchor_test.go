package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sophia/internal/service"
)

func TestCRRangeAndRevParseJSONCommands(t *testing.T) {
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")

	_, err := svc.AddCR("Anchor JSON", "json range/rev-parse commands")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "anchor.txt"), []byte("one\n"), 0o644); err != nil {
		t.Fatalf("write anchor.txt: %v", err)
	}
	runGit(t, dir, "add", "anchor.txt")
	runGit(t, dir, "commit", "-m", "feat: anchor fixture")

	rangeOut, _, rangeErr := runCLI(t, dir, "cr", "range", "1", "--json")
	if rangeErr != nil {
		t.Fatalf("cr range --json error = %v\noutput=%s", rangeErr, rangeOut)
	}
	rangeEnv := decodeEnvelope(t, rangeOut)
	if !rangeEnv.OK {
		t.Fatalf("expected ok envelope from cr range --json, got %#v", rangeEnv)
	}
	for _, key := range []string{"cr_id", "base", "head", "merge_base"} {
		if _, ok := rangeEnv.Data[key]; !ok {
			t.Fatalf("expected range key %q in %#v", key, rangeEnv.Data)
		}
	}

	revOut, _, revErr := runCLI(t, dir, "cr", "rev-parse", "1", "--kind", "head")
	if revErr != nil {
		t.Fatalf("cr rev-parse --kind head error = %v\noutput=%s", revErr, revOut)
	}
	revText := strings.TrimSpace(revOut)
	if revText == "" || strings.Contains(revText, "\n") {
		t.Fatalf("expected single-line commit hash, got %q", revOut)
	}

	revJSONOut, _, revJSONErr := runCLI(t, dir, "cr", "rev-parse", "1", "--kind", "merge-base", "--json")
	if revJSONErr != nil {
		t.Fatalf("cr rev-parse --json error = %v\noutput=%s", revJSONErr, revJSONOut)
	}
	revEnv := decodeEnvelope(t, revJSONOut)
	if !revEnv.OK {
		t.Fatalf("expected ok envelope from rev-parse --json, got %#v", revEnv)
	}
	for _, key := range []string{"cr_id", "kind", "commit"} {
		if _, ok := revEnv.Data[key]; !ok {
			t.Fatalf("expected rev-parse key %q in %#v", key, revEnv.Data)
		}
	}

	invalidOut, _, invalidErr := runCLI(t, dir, "cr", "rev-parse", "1", "--kind", "bad", "--json")
	if invalidErr == nil {
		t.Fatalf("expected invalid --kind error")
	}
	invalidEnv := decodeEnvelope(t, invalidOut)
	if invalidEnv.OK || invalidEnv.Error == nil {
		t.Fatalf("expected structured error envelope, got %#v", invalidEnv)
	}
}
