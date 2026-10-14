// Package summarizer produces a candidate summary for a batch of articles and
// submits it to the contract before anyone sees it.
//
// The package owns the seam: the live ADK workflow, a test fake and the CLI all
// pass through the same policy, so a summary cannot be published by taking a
// different route.
package summarizer

import (
	"context"
	"errors"
	"fmt"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// Summarizer produces a candidate summary for a batch. The candidate is never
// trusted: Check enforces the contract on whatever comes back.
type Summarizer interface {
	Summarize(ctx context.Context, batch news.NewsBatch) (intelligence.IntelligenceSummary, error)
}

// Options configures a Summarizer implementation.
type Options struct {
	// Provider selects the model provider: apigee, gemini or openai. Empty means
	// the ADK_MODEL_PROVIDER environment variable, and then gemini.
	Provider string
	// Model overrides the model name. Empty means the ADK_MODEL environment
	// variable, and then the provider's documented default when it has one.
	Model string
	// NewModel overrides how the model is built. A caller may plug in any ADK
	// model.LLM this way; leaving it empty uses the provider's own environment
	// variables.
	NewModel ModelFactory
}

var (
	// ErrModelUnavailable means no usable model could be created or reached.
	ErrModelUnavailable = errors.New("the model is unavailable")
	// ErrModelOutput means the model produced nothing that could be decoded.
	ErrModelOutput = errors.New("the model returned no usable structured output")
	// ErrContractViolation means a candidate summary failed the contract.
	ErrContractViolation = errors.New("the model output violated the summary contract")
)

// Extraction is what the first agent returns: the claims it found, each one with
// the quotation that supports it.
type Extraction struct {
	Claims []intelligence.Claim `json:"claims"`
}

// Check runs the publish policy over a candidate: normalize what can be repaired,
// enforce the contract, and attach the citations derived from the batch.
//
// It is the single place where the policy runs, so it applies to every Summarizer,
// including a fake used in a test.
func Check(candidate intelligence.IntelligenceSummary, batch news.NewsBatch) (intelligence.IntelligenceSummary, error) {
	checked, err := intelligence.Check(candidate, batch)
	if err != nil {
		return intelligence.IntelligenceSummary{}, fmt.Errorf("%w: %w", ErrContractViolation, err)
	}
	return checked, nil
}
