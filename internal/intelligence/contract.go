// Package intelligence holds the summary contract and the pure-Go rules that
// decide whether a candidate summary may be published.
//
// A schema can guarantee the shape of a summary; it cannot guarantee that the
// summary is anchored in the articles it claims to describe. Everything in this
// package exists to make that second half a deterministic, offline decision.
package intelligence

import (
	"errors"
	"fmt"
	"strings"
)

// Confidence values a claim may carry. They are part of the contract rather
// than a hint: Validate rejects anything else, and a claim over an undated
// article may not be high.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// Violation kinds. They exist so a caller can classify a failure with errors.Is
// instead of matching message text; the message itself stays stable for humans.
var (
	ErrEmptyBatch           = errors.New("empty batch")
	ErrEmptyText            = errors.New("empty text")
	ErrUngroundedSource     = errors.New("ungrounded source")
	ErrUngroundedQuote      = errors.New("ungrounded quote")
	ErrInvalidConfidence    = errors.New("invalid confidence")
	ErrUnverifiedConfidence = errors.New("unverified confidence")
	ErrInvalidConflict      = errors.New("invalid conflict")
	ErrRepeatedClaim        = errors.New("repeated claim")
)

// problem is one contract violation together with the path of the field that
// caused it and the kind it belongs to.
type problem struct {
	path   string
	reason string
	kind   error
}

func (p problem) Error() string { return p.path + ": " + p.reason }

func (p problem) Unwrap() error { return p.kind }

// Problems is every violation found in one candidate, in a stable order.
type Problems []problem

// Error lists every problem at once, so one run reports every reason instead of
// only the first.
func (p Problems) Error() string {
	reasons := make([]string, 0, len(p))
	for _, item := range p {
		reasons = append(reasons, item.Error())
	}
	if len(reasons) == 1 {
		return "1 problem: " + reasons[0]
	}
	return fmt.Sprintf("%d problems: %s", len(reasons), strings.Join(reasons, "; "))
}

// Unwrap exposes every problem, so errors.Is(problems, ErrUngroundedQuote) is
// true when any single problem has that kind.
func (p Problems) Unwrap() []error {
	kinds := make([]error, 0, len(p))
	for _, item := range p {
		kinds = append(kinds, item)
	}
	return kinds
}

// Quote is one verbatim span of one supplied article, copied by the model and
// checked against the article's own normalized text.
type Quote struct {
	SourceID string `json:"source_id"`
	Quote    string `json:"quote"`
}

// Claim is one factual statement the summary makes about the batch.
type Claim struct {
	Text string `json:"text"`
	// SourceIDs are the articles that support this claim. At least one is
	// required, and every id must exist in the batch.
	SourceIDs []string `json:"source_ids"`
	// Evidence is at least one quoted span, one per supporting article.
	Evidence []Quote `json:"evidence"`
	// Confidence is ConfidenceHigh, ConfidenceMedium or ConfidenceLow.
	Confidence string `json:"confidence"`
}

// Theme is a grouping of claims that the batch covers together.
type Theme struct {
	Text      string   `json:"text"`
	SourceIDs []string `json:"source_ids"`
}

// Conflict records that two or more articles disagree about one subject.
type Conflict struct {
	Subject string `json:"subject"`
	// Positions are the competing values, at least two of them.
	Positions []string `json:"positions"`
	// SourceIDs are the articles on each side, at least two of them.
	SourceIDs []string `json:"source_ids"`
}

// Gap records something material the batch does not establish. A gap may cite
// no article at all, because "no article says this" is itself the finding.
type Gap struct {
	Text      string   `json:"text"`
	SourceIDs []string `json:"source_ids"`
}

// Citation is the batch's own record of a source. It is assembled in Go from
// the supplied batch and never accepted from the model, so a fabricated
// citation or timestamp cannot reach the output.
type Citation struct {
	SourceID    string   `json:"source_id"`
	Title       string   `json:"title"`
	PublishedAt string   `json:"published_at"`
	DuplicateOf []string `json:"duplicate_of,omitempty"`
}

// IntelligenceSummary is the whole output contract.
type IntelligenceSummary struct {
	Claims    []Claim    `json:"claims"`
	Themes    []Theme    `json:"themes"`
	Conflicts []Conflict `json:"conflicts"`
	Gaps      []Gap      `json:"gaps"`
	// Citations is filled by Check from the batch. A candidate value is
	// discarded by Normalize.
	Citations []Citation `json:"citations"`
}
