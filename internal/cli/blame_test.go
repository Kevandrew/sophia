package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"sophia/internal/model"
	"sophia/internal/service"
)

func TestBlameCommandTextOutputIncludesIntentColumns(t *testing.T) {
	t.Parallel()
	dir, cr := setupBlameFixture(t)

	out, _, err := runCLI(t, dir, "blame", "fixture.txt")
	if err != nil {
		t.Fatalf("blame text error = %v\noutput=%s", err, out)
	}
	if !strings.Contains(out, "LINE\tAUTHOR\tDATE\tSHA\tCR\tINTENT\tCODE") {
		t.Fatalf("expected blame table header, got %q", out)
	}
	if !strings.Contains(out, "\t"+cr.Title+"\t") {
		t.Fatalf("expected intent title in output, got %q", out)
	}
	if !strings.Contains(out, "\t1\t") {
		t.Fatalf("expected CR id in output, got %q", out)
	}
}

func TestBlameCommandJSONEnvelopeAndKeys(t *testing.T) {
	t.Parallel()
	dir, _ := setupBlameFixture(t)

	out, _, err := runCLI(t, dir, "blame", "fixture.txt", "--json")
	if err != nil {
		t.Fatalf("blame --json error = %v\noutput=%s", err, out)
	}
	env := decodeEnvelope(t, out)
	if !env.OK {
		t.Fatalf("expected ok envelope, got %#v", env)
	}
	for _, key := range []string{"path", "rev", "ranges", "lines"} {
		if _, ok := env.Data[key]; !ok {
			t.Fatalf("expected data key %q in %#v", key, env.Data)
		}
	}
	lines, ok := env.Data["lines"].([]any)
	if !ok || len(lines) == 0 {
		t.Fatalf("expected non-empty lines array, got %#v", env.Data["lines"])
	}
	first, ok := lines[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first line map, got %#v", lines[0])
	}
	for _, key := range []string{"line", "commit", "cr_id", "intent", "intent_source", "text"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("expected line key %q in %#v", key, first)
		}
	}
}

func TestBlameCommandLineRangeFilter(t *testing.T) {
	t.Parallel()
	dir, _ := setupBlameFixture(t)

	out, _, err := runCLI(t, dir, "blame", "fixture.txt", "-L", "2,2", "--json")
	if err != nil {
		t.Fatalf("blame -L --json error = %v\noutput=%s", err, out)
	}
	env := decodeEnvelope(t, out)
	lines, ok := env.Data["lines"].([]any)
	if !ok {
		t.Fatalf("expected lines array, got %#v", env.Data["lines"])
	}
	if len(lines) != 1 {
		t.Fatalf("expected exactly one line in filtered output, got %d", len(lines))
	}
	first, ok := lines[0].(map[string]any)
	if !ok {
		t.Fatalf("expected line map, got %#v", lines[0])
	}
	if got, _ := first["line"].(float64); int(got) != 2 {
		t.Fatalf("expected line number 2, got %#v", first["line"])
	}
}

func TestBlameCommandRejectsInvalidLineRange(t *testing.T) {
	t.Parallel()
	dir, _ := setupBlameFixture(t)

	_, _, err := runCLI(t, dir, "blame", "fixture.txt", "-L", "two")
	if err == nil || !strings.Contains(err.Error(), "invalid --lines value") {
		t.Fatalf("expected invalid --lines error, got %v", err)
	}
}

func setupBlameFixture(t *testing.T) (string, *model.CR) {
	t.Helper()
	dir := t.TempDir()
	svc := service.New(dir)
	if _, err := svc.Init("main", ""); err != nil {
		t.Fatalf("Init() error = %v", err)
	}

	cr, err := svc.AddCR("Blame CLI intent", "CLI blame fixture")
	if err != nil {
		t.Fatalf("AddCR() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.txt"), []byte("alpha\nbeta\ngamma\n"), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
	runGit(t, dir, "add", "fixture.txt")
	runGit(t, dir, "commit", "-m", "feat: blame fixture", "-m", "Sophia-CR: "+strconv.Itoa(cr.ID)+"\nSophia-CR-UID: "+cr.UID+"\nSophia-Intent: "+cr.Title)
	return dir, cr
}
