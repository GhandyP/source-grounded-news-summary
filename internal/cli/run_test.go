package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
	"github.com/GhandyP/source-grounded-news-summary/internal/summarizer"
)

const batchJSON = `{"articles":[
	{"source_id":"wire","title":"Wire report","published_at":"2026-10-03","text":"The council approved the budget on Friday."},
	{"source_id":"daily","title":"Daily follow-up","published_at":"unknown","text":"The council approved the budget on Friday, the mayor said."}
]}`

func groundedCandidate() intelligence.IntelligenceSummary {
	return intelligence.IntelligenceSummary{
		Claims: []intelligence.Claim{{
			Text:       "The council approved the budget.",
			SourceIDs:  []string{"wire"},
			Evidence:   []intelligence.Quote{{SourceID: "wire", Quote: "approved the budget on Friday"}},
			Confidence: intelligence.ConfidenceHigh,
		}},
		Gaps: []intelligence.Gap{{Text: "No article states the vote margin."}},
	}
}

// fakeDeps records whether the summarizer was constructed at all, which is how
// the "empty input never reaches the model" rule is observed.
func fakeDeps(calls *int, result intelligence.IntelligenceSummary, err error) Deps {
	return Deps{
		NewSummarizer: func(context.Context, summarizer.Options) (summarizer.Summarizer, error) {
			*calls++
			return summarizer.FakeFunc(func(context.Context, news.NewsBatch) (intelligence.IntelligenceSummary, error) {
				return result, err
			}), nil
		},
	}
}

func writeBatch(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return path
}

func run(t *testing.T, args []string, stdin string, deps Deps) (int, string, string) {
	t.Helper()

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), args, strings.NewReader(stdin), &stdout, &stderr, deps)
	return code, stdout.String(), stderr.String()
}

func TestRunSummarizesAValidBatch(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)

	code, stdout, stderr := run(t, []string{"-input", path}, "", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	if calls != 1 {
		t.Fatalf("the summarizer was constructed %d times, want 1", calls)
	}
	if !strings.Contains(stderr, Banner) {
		t.Fatalf("stderr does not carry the review banner: %q", stderr)
	}

	var summary intelligence.IntelligenceSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("stdout is not a summary: %v\n%s", err, stdout)
	}
	if len(summary.Claims) != 1 || len(summary.Citations) != 2 {
		t.Fatalf("decoded summary = %#v, want one claim and two citations", summary)
	}
}

func TestRunReadsStandardInput(t *testing.T) {
	calls := 0

	code, stdout, stderr := run(t, []string{"-input", "-"}, batchJSON, fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	if !strings.Contains(stdout, `"claims"`) {
		t.Fatalf("stdout does not carry the claims: %s", stdout)
	}
}

func TestRunRendersText(t *testing.T) {
	calls := 0

	code, stdout, _ := run(t, []string{"-input", "-", "-out", "text"}, batchJSON, fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d", code, ExitOK)
	}
	for _, want := range []string{"Claims (1)", "The council approved the budget.", "Confidence: high", "Citations (2)"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("text output is missing %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, `"claims"`) {
		t.Fatalf("text output is still JSON:\n%s", stdout)
	}
}

func TestRunRejectsUsageErrors(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		stdin string
		want  string
	}{
		{name: "unknown flag", args: []string{"-nope"}, want: "flag provided but not defined"},
		{name: "positional argument", args: []string{"extra"}, stdin: batchJSON, want: "unexpected argument"},
		{name: "unknown output format", args: []string{"-out", "yaml"}, stdin: batchJSON, want: "unknown output format"},
		{name: "missing file", args: []string{"-input", filepath.Join(t.TempDir(), "absent.json")}, want: "read "},
		{name: "invalid JSON", args: []string{"-input", "-"}, stdin: "{", want: "input is not a news batch"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0

			code, _, stderr := run(t, test.args, test.stdin, fakeDeps(&calls, groundedCandidate(), nil))

			if code != ExitUsage {
				t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitUsage, stderr)
			}
			if !strings.Contains(stderr, test.want) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr, test.want)
			}
			if calls != 0 {
				t.Fatalf("a usage error reached the summarizer (%d calls)", calls)
			}
		})
	}
}

func TestRunRejectsABatchWithoutUsableText(t *testing.T) {
	calls := 0
	path := writeBatch(t, `{"articles":[{"source_id":"blank","title":"Blank","published_at":"2026-10-03","text":"  \n\t "}]}`)

	code, _, stderr := run(t, []string{"-input", path}, "", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitUsage {
		t.Fatalf("Run() = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "input has no article text to summarize") {
		t.Fatalf("stderr = %q, want the empty-batch message", stderr)
	}
	if calls != 0 {
		t.Fatalf("an empty batch reached the summarizer (%d calls)", calls)
	}
}

func TestRunReportsAContractViolation(t *testing.T) {
	calls := 0
	invented := groundedCandidate()
	invented.Claims[0].Evidence = []intelligence.Quote{{SourceID: "wire", Quote: "The council rejected the budget."}}

	code, _, stderr := run(t, []string{"-input", "-"}, batchJSON, fakeDeps(&calls, invented, nil))

	if code != ExitContractViolation {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitContractViolation, stderr)
	}
	if !strings.Contains(stderr, "ungrounded quote") {
		t.Fatalf("stderr = %q, want the grounding failure", stderr)
	}
}

func TestRunReportsAModelFailure(t *testing.T) {
	calls := 0

	code, _, stderr := run(t, []string{"-input", "-"}, batchJSON,
		fakeDeps(&calls, intelligence.IntelligenceSummary{}, summarizer.ErrModelUnavailable))

	if code != ExitFailure {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "model is unavailable") {
		t.Fatalf("stderr = %q, want the model failure", stderr)
	}
}

func TestRunReportsAnUnusableModelResult(t *testing.T) {
	calls := 0

	code, _, stderr := run(t, []string{"-input", "-"}, batchJSON,
		fakeDeps(&calls, intelligence.IntelligenceSummary{}, summarizer.ErrModelOutput))

	if code != ExitFailure {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "no usable structured output") {
		t.Fatalf("stderr = %q, want the missing-output failure", stderr)
	}
}

func TestRunChecksWhateverTheSummarizerReturned(t *testing.T) {
	// A fake that bypasses its own validation still cannot publish an invented
	// quotation: the CLI re-runs the contract at the trust boundary.
	calls := 0
	invented := groundedCandidate()
	invented.Claims[0].SourceIDs = []string{"s9"}

	code, _, stderr := run(t, []string{"-input", "-"}, batchJSON, fakeDeps(&calls, invented, nil))

	if code != ExitContractViolation {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitContractViolation, stderr)
	}
	if !strings.Contains(stderr, `unknown source id "s9"`) {
		t.Fatalf("stderr = %q, want the ungrounced source failure", stderr)
	}
}

func TestRunVerboseWritesDiagnostics(t *testing.T) {
	calls := 0

	code, _, stderr := run(t, []string{"-input", "-", "-verbose"}, batchJSON, fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d", code, ExitOK)
	}
	for _, want := range []string{"articles: 2", "exit reason:"} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr is missing %q:\n%s", want, stderr)
		}
	}
}

func TestRunIsQuietWithoutVerbose(t *testing.T) {
	calls := 0

	_, _, stderr := run(t, []string{"-input", "-"}, batchJSON, fakeDeps(&calls, groundedCandidate(), nil))

	if strings.Contains(stderr, "articles:") {
		t.Fatalf("stderr carries diagnostics without -verbose:\n%s", stderr)
	}
	if !strings.Contains(stderr, Banner) {
		t.Fatalf("stderr = %q, want the banner", stderr)
	}
}

func TestRunHonorsACancelledContext(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var stdout, stderr bytes.Buffer
	code := Run(ctx, []string{"-input", path}, strings.NewReader(""), &stdout, &stderr,
		fakeDeps(&calls, intelligence.IntelligenceSummary{}, context.Canceled))

	if code != ExitFailure {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitFailure, stderr.String())
	}
	if !strings.Contains(stderr.String(), "context canceled") {
		t.Fatalf("stderr = %q, want the cancellation reason", stderr.String())
	}
}

func TestRunDefaultsToTheLiveSummarizer(t *testing.T) {
	// A zero Deps must not panic or silently succeed: without a key the live
	// summarizer reports an unavailable model, which is a failure exit, not a
	// crash.
	t.Setenv("GOOGLE_API_KEY", "")

	code, _, stderr := run(t, []string{"-input", "-"}, batchJSON, Deps{})

	if code != ExitFailure {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitFailure, stderr)
	}
	if !strings.Contains(stderr, "GOOGLE_API_KEY is not set") {
		t.Fatalf("stderr = %q, want the missing-key message", stderr)
	}
}
