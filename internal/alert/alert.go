// Package alert prepares a summary that already passed the contract to become an
// alert file, and writes it only when a human approves the exact digest of what
// will be written.
//
// An alert is the one artefact of this project that is meant to leave the machine,
// so the gate is explicit rather than implicit: nothing is written without an
// approval that matches the digest of the candidate, and the same digest is never
// delivered twice.
package alert

import (
	"errors"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
)

var (
	// ErrUncheckedSummary means the summary carries no citations, so it never
	// passed Check: an alert may not be built from a candidate nobody validated.
	ErrUncheckedSummary = errors.New("the summary never passed the contract")
	// ErrEmptyPath means no alert destination was given.
	ErrEmptyPath = errors.New("no alert path was given")
	// ErrApprovalMismatch means the approval does not match the candidate digest.
	ErrApprovalMismatch = errors.New("the approval does not match the alert digest")
	// ErrAlreadyDelivered means this digest was approved and written before.
	ErrAlreadyDelivered = errors.New("this alert was already delivered")
	// ErrTamperedCandidate means the candidate's digest does not describe its summary.
	ErrTamperedCandidate = errors.New("the candidate does not match its own digest")
)

// Candidate is a summary that passed the contract together with the digest a
// human approves before anything is written.
type Candidate struct {
	Summary intelligence.IntelligenceSummary `json:"summary"`
	Digest  string                           `json:"digest"`
}
