package alert

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
)

func checkedSummary() intelligence.IntelligenceSummary {
	return intelligence.IntelligenceSummary{
		Claims: []intelligence.Claim{{
			Text:       "The council approved the budget.",
			SourceIDs:  []string{"wire"},
			Evidence:   []intelligence.Quote{{SourceID: "wire", Quote: "The council approved the budget on Friday."}},
			Confidence: intelligence.ConfidenceMedium,
		}},
		Gaps:      []intelligence.Gap{{Text: "No article states the vote margin."}},
		Citations: []intelligence.Citation{{SourceID: "wire", Title: "Wire report", PublishedAt: "2026-10-03"}},
	}
}

func TestNewRefusesASummaryThatNeverPassedTheContract(t *testing.T) {
	unchecked := checkedSummary()
	unchecked.Citations = nil

	if _, err := New(unchecked); !errors.Is(err, ErrUncheckedSummary) {
		t.Fatalf("New() = %v, want ErrUncheckedSummary: an alert must rest on a checked summary", err)
	}
}

func TestNewProducesAStableDistinctDigest(t *testing.T) {
	first, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	second, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if first.Digest != second.Digest {
		t.Fatalf("the same summary produced %q and %q", first.Digest, second.Digest)
	}
	if len(first.Digest) != 64 {
		t.Fatalf("digest = %q, want the full SHA-256 hex digest", first.Digest)
	}
	if strings.Trim(first.Digest, "0123456789abcdef") != "" {
		t.Fatalf("digest = %q, want lowercase hex", first.Digest)
	}

	other := checkedSummary()
	other.Claims[0].Text = "The council rejected the budget."
	changed, err := New(other)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if changed.Digest == first.Digest {
		t.Fatal("a different summary produced the same digest, so approval would not bind to content")
	}
}

func TestWriteRefusesAnEmptyPath(t *testing.T) {
	candidate, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := Write("  ", candidate, candidate.Digest); !errors.Is(err, ErrEmptyPath) {
		t.Fatalf("Write() = %v, want ErrEmptyPath", err)
	}
}

func TestWriteRefusesAnApprovalThatDoesNotMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alert.json")
	candidate, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	tests := []struct {
		name     string
		approval string
	}{
		{name: "empty", approval: ""},
		{name: "different digest", approval: strings.Repeat("0", 64)},
		{name: "truncated digest", approval: candidate.Digest[:16]},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := Write(path, candidate, test.approval); !errors.Is(err, ErrApprovalMismatch) {
				t.Fatalf("Write() = %v, want ErrApprovalMismatch", err)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("an unapproved alert was written to %s", path)
			}
		})
	}
}

func TestWriteRefusesACandidateThatDoesNotMatchItsOwnDigest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alert.json")
	candidate, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	candidate.Summary.Claims[0].Text = "The council rejected the budget."

	if err := Write(path, candidate, candidate.Digest); !errors.Is(err, ErrTamperedCandidate) {
		t.Fatalf("Write() = %v, want ErrTamperedCandidate", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a tampered candidate was written to %s", path)
	}
}

func TestWriteDeliversOnceWithAnApprovedDigest(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "alert.json")
	candidate, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := Write(path, candidate, candidate.Digest); err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the approved alert was not written: %v", err)
	}
	var decoded Candidate
	if err := json.Unmarshal(written, &decoded); err != nil {
		t.Fatalf("the alert is not readable JSON: %v", err)
	}
	if decoded.Digest != candidate.Digest {
		t.Fatalf("written digest = %q, want %q", decoded.Digest, candidate.Digest)
	}
	if len(decoded.Summary.Claims) != 1 || len(decoded.Summary.Citations) != 1 {
		t.Fatalf("the alert lost the claims or the citations: %#v", decoded.Summary)
	}

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	for _, name := range names {
		if strings.HasPrefix(name, ".") || strings.Contains(name, "tmp") {
			t.Fatalf("a temporary file was left behind: %v", names)
		}
	}
	if len(names) != 2 {
		t.Fatalf("directory holds %v, want the alert and its delivery ledger", names)
	}
}

func TestWriteRefusesToDeliverTheSameDigestTwice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alert.json")
	candidate, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := Write(path, candidate, candidate.Digest); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	if err := Write(path, candidate, candidate.Digest); !errors.Is(err, ErrAlreadyDelivered) {
		t.Fatalf("second Write() = %v, want ErrAlreadyDelivered", err)
	}

	second, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("the refused repeat changed the delivered alert")
	}
}

// Regression tests handed over by the independent verifier, which falsified both
// claims on the published commit 153f677. They are kept verbatim so the evidence
// and the fix stay tied to each other.
func TestWriteRefusesRepeatAfterLedgerRemoval(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alert.json")
	c, err := New(checkedSummary())
	if err != nil {
		t.Fatal(err)
	}
	if err = Write(path, c, c.Digest); err != nil {
		t.Fatal(err)
	}
	if err = os.Remove(path + ledgerSuffix); err != nil {
		t.Fatal(err)
	}
	if err = Write(path, c, c.Digest); !errors.Is(err, ErrAlreadyDelivered) {
		t.Fatalf("Write after ledger removal = %v, want ErrAlreadyDelivered", err)
	}
}

func TestWriteRefusesCandidateWithoutCitations(t *testing.T) {
	s := checkedSummary()
	s.Citations = nil
	d, err := digestOf(s)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "uncited.json")
	err = Write(path, Candidate{Summary: s, Digest: d}, d)
	if !errors.Is(err, ErrUncheckedSummary) {
		t.Fatalf("Write = %v, want ErrUncheckedSummary", err)
	}
	if _, err = os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("uncited alert exists: %v", err)
	}
}

func TestWriteDeliversADifferentSummaryToTheSamePath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alert.json")

	first, err := New(checkedSummary())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := Write(path, first, first.Digest); err != nil {
		t.Fatalf("first Write() error = %v", err)
	}

	other := checkedSummary()
	other.Claims[0].Text = "The ferry resumed."
	second, err := New(other)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := Write(path, second, second.Digest); err != nil {
		t.Fatalf("second Write() error = %v, want a different alert to be delivered", err)
	}

	written, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	var decoded Candidate
	if err := json.Unmarshal(written, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded.Digest != second.Digest {
		t.Fatalf("the file holds digest %q, want the newer %q", decoded.Digest, second.Digest)
	}
}
