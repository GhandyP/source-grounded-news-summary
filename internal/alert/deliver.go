package alert

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
)

// ledgerSuffix names the file that records which digests were already delivered.
// It sits next to the alert, so a delivery is remembered without any external
// state and a second approval of the same digest is refused even after a restart.
const ledgerSuffix = ".approved"

// New builds the candidate for a summary that passed the contract.
//
// A summary with no citations never went through Check, so it cannot become an
// alert: the approval gate must never be the thing that legitimises an
// unvalidated claim.
func New(summary intelligence.IntelligenceSummary) (Candidate, error) {
	if len(summary.Citations) == 0 {
		return Candidate{}, fmt.Errorf("%w: a summary with no citations has nothing to trace", ErrUncheckedSummary)
	}

	digest, err := digestOf(summary)
	if err != nil {
		return Candidate{}, err
	}

	return Candidate{Summary: summary, Digest: digest}, nil
}

// Write delivers the candidate to path, and only when approval matches the digest
// of exactly this content.
//
// The order matters: the candidate is verified against its own digest, the digest
// against the approval, and the ledger against a repeat, all before anything is
// written. A refusal leaves the filesystem untouched.
func Write(path string, candidate Candidate, approval string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return ErrEmptyPath
	}

	// The gate has to hold at the write boundary too, not only in New: a caller
	// that builds a Candidate by hand must not be able to deliver an unvalidated
	// summary that carries nothing to trace.
	if len(candidate.Summary.Citations) == 0 {
		return fmt.Errorf("%w: a summary with no citations has nothing to trace", ErrUncheckedSummary)
	}

	expected, err := digestOf(candidate.Summary)
	if err != nil {
		return err
	}
	if expected != candidate.Digest {
		return fmt.Errorf("%w: the digest describes different content", ErrTamperedCandidate)
	}

	if strings.TrimSpace(approval) != candidate.Digest {
		return fmt.Errorf("%w: approve this alert with -approve %s", ErrApprovalMismatch, candidate.Digest)
	}

	ledger := path + ledgerSuffix
	delivered, err := deliveredDigests(ledger)
	if err != nil {
		return err
	}
	if delivered[candidate.Digest] {
		return fmt.Errorf("%w: %s holds it already", ErrAlreadyDelivered, path)
	}

	// The ledger is not the only evidence that this alert was delivered: the
	// destination file is evidence as well. Deleting the ledger must not reopen
	// the gate, or the same alert could leave the machine twice.
	existing, err := digestAt(path)
	if err != nil {
		return err
	}
	if existing != "" && existing == candidate.Digest {
		return fmt.Errorf("%w: %s holds it already", ErrAlreadyDelivered, path)
	}

	if err := writeAtomically(path, candidate); err != nil {
		return err
	}
	return appendLedger(ledger, candidate.Digest)
}

// digestAt reports the digest of the alert already sitting at path, or an empty
// string when there is nothing readable there.
func digestAt(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("read the delivered alert %s: %w", path, err)
	}

	var delivered Candidate
	if err := json.Unmarshal(content, &delivered); err != nil {
		// Something else lives at this path; it is not evidence of a delivery.
		return "", nil
	}
	return delivered.Digest, nil
}

// digestOf binds an approval to the exact bytes of a summary.
func digestOf(summary intelligence.IntelligenceSummary) (string, error) {
	encoded, err := json.Marshal(summary)
	if err != nil {
		return "", fmt.Errorf("encode the summary for approval: %w", err)
	}

	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// writeAtomically writes through a temporary file in the destination directory
// and renames it, so a reader never sees a half-written alert.
func writeAtomically(path string, candidate Candidate) error {
	encoded, err := json.MarshalIndent(candidate, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the alert: %w", err)
	}
	encoded = append(encoded, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(path), ".alert-*")
	if err != nil {
		return fmt.Errorf("prepare the alert in %s: %w", filepath.Dir(path), err)
	}
	name := temporary.Name()
	defer func() {
		_ = os.Remove(name) // a no-op once the rename succeeded
	}()

	if _, err := temporary.Write(encoded); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write the alert: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("write the alert: %w", err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("deliver the alert to %s: %w", path, err)
	}

	return nil
}

func deliveredDigests(ledger string) (map[string]bool, error) {
	content, err := os.ReadFile(ledger)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, fmt.Errorf("read the delivery ledger %s: %w", ledger, err)
	}

	delivered := map[string]bool{}
	for _, line := range strings.Split(string(content), "\n") {
		if digest := strings.TrimSpace(line); digest != "" {
			delivered[digest] = true
		}
	}
	return delivered, nil
}

func appendLedger(ledger, digest string) error {
	file, err := os.OpenFile(ledger, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("record the delivery in %s: %w", ledger, err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := fmt.Fprintf(file, "%s\n", digest); err != nil {
		return fmt.Errorf("record the delivery in %s: %w", ledger, err)
	}
	return nil
}
