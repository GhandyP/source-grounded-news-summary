package intelligence

import "github.com/GhandyP/source-grounded-news-summary/internal/news"

// Check normalizes a candidate, enforces the contract, and attaches the
// batch-derived citations. It returns the normalized summary on success.
//
// Check is the single place where the publish policy runs, so it applies to
// whatever produced the candidate: a model, a test fixture or a caller's own
// code. There is no bypass.
func Check(candidate IntelligenceSummary, batch news.NewsBatch) (IntelligenceSummary, error) {
	if len(news.Normalize(batch).Articles) == 0 {
		return IntelligenceSummary{}, Problems{{
			path:   "batch",
			reason: "batch has no usable article text",
			kind:   ErrEmptyBatch,
		}}
	}

	normalized := Normalize(candidate)
	if err := Validate(normalized, batch); err != nil {
		return IntelligenceSummary{}, err
	}

	normalized.Citations = Citations(batch)
	return normalized, nil
}
