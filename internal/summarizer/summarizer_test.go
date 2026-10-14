package summarizer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

func batch() news.NewsBatch {
	return news.NewsBatch{Articles: []news.Article{
		{
			SourceID:    "wire",
			Title:       "Wire report",
			PublishedAt: "2026-10-03",
			Text:        "The council approved the budget on Friday.",
		},
		{
			SourceID:    "daily",
			Title:       "Daily follow-up",
			PublishedAt: news.PublishedUnknown,
			Text:        "The council approved the budget on Friday, the mayor said.",
		},
	}}
}

func candidate() intelligence.IntelligenceSummary {
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

func TestFakeFuncSatisfiesTheSeam(t *testing.T) {
	var _ Summarizer = FakeFunc(nil)

	fake := FakeFunc(func(context.Context, news.NewsBatch) (intelligence.IntelligenceSummary, error) {
		return candidate(), nil
	})

	got, err := fake.Summarize(context.Background(), batch())
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}
	if !reflect.DeepEqual(got, candidate()) {
		t.Fatalf("Summarize() = %#v, want the fake's own value", got)
	}
}

func TestCheckAcceptsAGroundedCandidate(t *testing.T) {
	got, err := Check(candidate(), batch())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if want := intelligence.Citations(batch()); !reflect.DeepEqual(got.Citations, want) {
		t.Fatalf("Check() citations = %#v, want %#v", got.Citations, want)
	}
}

func TestCheckRejectsAnInventedQuotation(t *testing.T) {
	summary := candidate()
	summary.Claims[0].Evidence = []intelligence.Quote{{SourceID: "wire", Quote: "The council rejected the budget."}}

	_, err := Check(summary, batch())

	if err == nil {
		t.Fatal("Check() = nil, want a contract violation")
	}
	if !errors.Is(err, ErrContractViolation) {
		t.Fatalf("errors.Is(%v, ErrContractViolation) = false, want true", err)
	}
	if !errors.Is(err, intelligence.ErrUngroundedQuote) {
		t.Fatalf("errors.Is(%v, intelligence.ErrUngroundedQuote) = false, want true", err)
	}
}

func TestCheckRejectsAnUnknownSource(t *testing.T) {
	summary := candidate()
	summary.Claims[0].SourceIDs = []string{"wire", "s9"}

	_, err := Check(summary, batch())

	if !errors.Is(err, intelligence.ErrUngroundedSource) {
		t.Fatalf("errors.Is(%v, intelligence.ErrUngroundedSource) = false, want true", err)
	}
}

func TestCheckRejectsAnEmptyBatch(t *testing.T) {
	empty := news.NewsBatch{Articles: []news.Article{
		{SourceID: "blank", Title: "Blank", PublishedAt: news.PublishedUnknown, Text: " \t "},
	}}

	_, err := Check(candidate(), empty)

	if !errors.Is(err, intelligence.ErrEmptyBatch) {
		t.Fatalf("errors.Is(%v, intelligence.ErrEmptyBatch) = false, want true", err)
	}
}

func TestCheckRepairsWhatNormalizeCanRepair(t *testing.T) {
	summary := candidate()
	summary.Claims = append(summary.Claims, intelligence.Claim{
		Text:       "THE COUNCIL   approved the budget.",
		SourceIDs:  []string{"daily"},
		Evidence:   []intelligence.Quote{{SourceID: "daily", Quote: "the mayor said"}},
		Confidence: intelligence.ConfidenceMedium,
	})

	got, err := Check(summary, batch())
	if err != nil {
		t.Fatalf("Check() error = %v, want the repeated claim merged", err)
	}
	if len(got.Claims) != 1 {
		t.Fatalf("Check() returned %d claims, want 1", len(got.Claims))
	}
	if want := []string{"wire", "daily"}; !reflect.DeepEqual(got.Claims[0].SourceIDs, want) {
		t.Fatalf("Check() sources = %#v, want %#v", got.Claims[0].SourceIDs, want)
	}
}
