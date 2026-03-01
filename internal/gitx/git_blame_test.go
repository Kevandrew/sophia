package gitx

import (
	"reflect"
	"testing"
)

func TestParseBlamePorcelainParsesMetadataAndLines(t *testing.T) {
	t.Parallel()
	raw := "1111111111111111111111111111111111111111 1 1 2\n" +
		"author Test One\n" +
		"author-mail <one@example.com>\n" +
		"author-time 1700000000\n" +
		"author-tz +0000\n" +
		"summary feat: first line\n" +
		"filename sample.txt\n" +
		"\talpha\n" +
		"1111111111111111111111111111111111111111 2 2\n" +
		"author Test One\n" +
		"author-mail <one@example.com>\n" +
		"author-time 1700000000\n" +
		"author-tz +0000\n" +
		"summary feat: second line\n" +
		"filename sample.txt\n" +
		"\tbeta\n" +
		"0000000000000000000000000000000000000000 3 3\n" +
		"author Not Committed Yet\n" +
		"author-mail <not.committed.yet>\n" +
		"author-time 1700000600\n" +
		"author-tz -0700\n" +
		"summary Version of sample.txt from sample.txt\n" +
		"filename sample.txt\n" +
		"\tgamma\n"

	parsed, err := parseBlamePorcelain(raw)
	if err != nil {
		t.Fatalf("parseBlamePorcelain() error = %v", err)
	}
	if len(parsed) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(parsed))
	}

	if parsed[0].CommitHash != "1111111111111111111111111111111111111111" || parsed[0].OrigLine != 1 || parsed[0].FinalLine != 1 {
		t.Fatalf("unexpected first parsed entry: %#v", parsed[0])
	}
	if parsed[0].Author != "Test One" || parsed[0].AuthorMail != "one@example.com" {
		t.Fatalf("unexpected author metadata: %#v", parsed[0])
	}
	if parsed[0].Summary != "feat: first line" || parsed[0].Text != "alpha" {
		t.Fatalf("unexpected summary/text: %#v", parsed[0])
	}
	if parsed[0].AuthorTime != "2023-11-14T22:13:20Z" {
		t.Fatalf("unexpected author time %q", parsed[0].AuthorTime)
	}

	if parsed[2].CommitHash != "0000000000000000000000000000000000000000" || parsed[2].Text != "gamma" {
		t.Fatalf("unexpected uncommitted entry: %#v", parsed[2])
	}
	if parsed[2].AuthorTime != "2023-11-14T15:23:20-07:00" {
		t.Fatalf("unexpected timezone conversion: %q", parsed[2].AuthorTime)
	}
}

func TestBuildBlameArgsWithRevAndRanges(t *testing.T) {
	t.Parallel()
	args := buildBlameArgs("internal/service/service.go", "HEAD~1", []BlameRange{{Start: 10, End: 20}, {Start: 33, End: 34}})
	want := []string{"blame", "--line-porcelain", "-L", "10,20", "-L", "33,34", "HEAD~1", "--", "internal/service/service.go"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("buildBlameArgs() = %#v, want %#v", args, want)
	}
}
