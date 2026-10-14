package summarizer

import (
	"context"
	"errors"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// FakeFunc adapts a function to the Summarizer seam, so the CLI, the policy and
// the fixture suite can run every rule without a model, a key or a network.
type FakeFunc func(ctx context.Context, batch news.NewsBatch) (intelligence.IntelligenceSummary, error)

// Summarize calls the wrapped function.
func (f FakeFunc) Summarize(ctx context.Context, batch news.NewsBatch) (intelligence.IntelligenceSummary, error) {
	if f == nil {
		return intelligence.IntelligenceSummary{}, errors.New("the fake summarizer has no implementation")
	}
	return f(ctx, batch)
}
