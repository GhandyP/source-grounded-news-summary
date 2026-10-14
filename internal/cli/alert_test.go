package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/alert"
	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
)

var digestPattern = regexp.MustCompile(`\b[0-9a-f]{64}\b`)

func TestRunRequiresApprovalBeforeWritingAnAlert(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)
	alertPath := filepath.Join(t.TempDir(), "alert.json")

	code, stdout, stderr := run(t, []string{"-input", path, "-alert", alertPath}, "", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitApprovalRequired {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitApprovalRequired, stderr)
	}
	if _, err := os.Stat(alertPath); !os.IsNotExist(err) {
		t.Fatalf("an unapproved run wrote %s", alertPath)
	}
	if !strings.Contains(stderr, alertPath) {
		t.Fatalf("stderr = %q, want the destination in it", stderr)
	}
	if !digestPattern.MatchString(stderr) {
		t.Fatalf("stderr = %q, want a digest to approve", stderr)
	}
	if !strings.Contains(stdout, `"claims"`) {
		t.Fatalf("stdout is missing the summary the human is approving: %s", stdout)
	}
}

func TestRunWritesTheAlertWithTheApprovedDigest(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)
	alertPath := filepath.Join(t.TempDir(), "alert.json")

	_, _, pending := run(t, []string{"-input", path, "-alert", alertPath}, "", fakeDeps(&calls, groundedCandidate(), nil))
	digest := digestPattern.FindString(pending)
	if digest == "" {
		t.Fatalf("no digest was offered for approval: %q", pending)
	}

	code, _, stderr := run(t, []string{"-input", path, "-alert", alertPath, "-approve", digest}, "", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	if !strings.Contains(stderr, "alert written to") {
		t.Fatalf("stderr = %q, want the delivery reported", stderr)
	}

	written, err := os.ReadFile(alertPath)
	if err != nil {
		t.Fatalf("the approved alert was not written: %v", err)
	}
	var candidate alert.Candidate
	if err := json.Unmarshal(written, &candidate); err != nil {
		t.Fatalf("the alert is not readable JSON: %v", err)
	}
	if candidate.Digest != digest {
		t.Fatalf("written digest = %q, want the approved %q", candidate.Digest, digest)
	}
	if len(candidate.Summary.Citations) == 0 {
		t.Fatal("the alert carries no citations, so nothing downstream can trace it")
	}
}

func TestRunRefusesAnApprovalThatDoesNotMatch(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)
	alertPath := filepath.Join(t.TempDir(), "alert.json")

	code, _, stderr := run(t, []string{"-input", path, "-alert", alertPath, "-approve", strings.Repeat("0", 64)},
		"", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitUsage {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitUsage, stderr)
	}
	if !strings.Contains(stderr, "does not match") {
		t.Fatalf("stderr = %q, want the mismatch reported", stderr)
	}
	if _, err := os.Stat(alertPath); !os.IsNotExist(err) {
		t.Fatalf("a mismatched approval wrote %s", alertPath)
	}
}

func TestRunRefusesToDeliverTheSameAlertTwice(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)
	alertPath := filepath.Join(t.TempDir(), "alert.json")

	_, _, pending := run(t, []string{"-input", path, "-alert", alertPath}, "", fakeDeps(&calls, groundedCandidate(), nil))
	digest := digestPattern.FindString(pending)

	args := []string{"-input", path, "-alert", alertPath, "-approve", digest}
	if code, _, stderr := run(t, args, "", fakeDeps(&calls, groundedCandidate(), nil)); code != ExitOK {
		t.Fatalf("first delivery = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	first, err := os.ReadFile(alertPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}

	code, _, stderr := run(t, args, "", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitAlreadyDelivered {
		t.Fatalf("second delivery = %d, want %d; stderr: %s", code, ExitAlreadyDelivered, stderr)
	}
	if !strings.Contains(stderr, "already delivered") {
		t.Fatalf("stderr = %q, want the repeat reported", stderr)
	}
	second, err := os.ReadFile(alertPath)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(first) != string(second) {
		t.Fatal("the refused repeat changed the delivered alert")
	}
}

func TestRunRejectsAnApprovalWithoutAnAlertPath(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)

	code, _, stderr := run(t, []string{"-input", path, "-approve", strings.Repeat("a", 64)}, "", fakeDeps(&calls, groundedCandidate(), nil))

	if code != ExitUsage {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitUsage, stderr)
	}
	if !strings.Contains(stderr, "without -alert") {
		t.Fatalf("stderr = %q, want the refusal explained", stderr)
	}
}

func TestRunNeverBuildsAnAlertFromARefusedSummary(t *testing.T) {
	calls := 0
	path := writeBatch(t, batchJSON)
	alertPath := filepath.Join(t.TempDir(), "alert.json")

	invented := groundedCandidate()
	invented.Claims[0].Evidence = []intelligence.Quote{{SourceID: "wire", Quote: "The council rejected the budget."}}

	code, stdout, stderr := run(t, []string{"-input", path, "-alert", alertPath}, "", fakeDeps(&calls, invented, nil))

	if code != ExitContractViolation {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitContractViolation, stderr)
	}
	if digestPattern.MatchString(stderr) {
		t.Fatalf("a refused summary still offered a digest to approve: %q", stderr)
	}
	if _, err := os.Stat(alertPath); !os.IsNotExist(err) {
		t.Fatalf("a refused summary produced an alert at %s", alertPath)
	}
	if stdout != "" {
		t.Fatalf("a refused run wrote to stdout:\n%s", stdout)
	}
}
