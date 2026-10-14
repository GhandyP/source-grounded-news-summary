package intelligence

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// batchWithDuplicate adds a second source carrying byte-identical text, which is
// the case this project has to make visible instead of counting twice.
func batchWithDuplicate() news.NewsBatch {
	shared := "The council approved the budget on Friday."
	return news.NewsBatch{Articles: []news.Article{
		{SourceID: "wire", Title: "Wire report", PublishedAt: "2026-10-03", Text: shared},
		{SourceID: "syndicated", Title: "Syndicated copy", PublishedAt: "2026-10-05", Text: shared},
		{
			SourceID:    "daily",
			Title:       "Daily follow-up",
			PublishedAt: news.PublishedUnknown,
			Text:        "The council approved the budget on Friday, the mayor said.",
		},
	}}
}

func TestNormalizeMergesRepeatedClaims(t *testing.T) {
	summary := IntelligenceSummary{Claims: []Claim{
		{
			Text:       "  The council approved the budget. ",
			SourceIDs:  []string{"wire", "wire"},
			Evidence:   []Quote{{SourceID: "wire", Quote: "The council approved    the budget on Friday."}},
			Confidence: "HIGH",
		},
		{
			Text:       "the council   APPROVED the budget.",
			SourceIDs:  []string{"daily"},
			Evidence:   []Quote{{SourceID: "daily", Quote: "the mayor said"}},
			Confidence: ConfidenceLow,
		},
	}}

	got := Normalize(summary)

	if len(got.Claims) != 1 {
		t.Fatalf("Normalize() kept %d claims, want 1: %#v", len(got.Claims), got.Claims)
	}
	claim := got.Claims[0]
	if claim.Text != "The council approved the budget." {
		t.Fatalf("claim text = %q, want the first occurrence trimmed", claim.Text)
	}
	if want := []string{"wire", "daily"}; !reflect.DeepEqual(claim.SourceIDs, want) {
		t.Fatalf("claim sources = %#v, want the union %#v", claim.SourceIDs, want)
	}
	if claim.Confidence != ConfidenceLow {
		t.Fatalf("claim confidence = %q, want the weakest of the merged claims %q", claim.Confidence, ConfidenceLow)
	}
	wantEvidence := []Quote{
		{SourceID: "wire", Quote: "The council approved the budget on Friday."},
		{SourceID: "daily", Quote: "the mayor said"},
	}
	if !reflect.DeepEqual(claim.Evidence, wantEvidence) {
		t.Fatalf("claim evidence = %#v, want %#v", claim.Evidence, wantEvidence)
	}
}

func TestNormalizeNeverRaisesConfidenceOnMerge(t *testing.T) {
	tests := []struct {
		name  string
		first string
		later string
		want  string
	}{
		{name: "high and medium stay medium", first: ConfidenceHigh, later: ConfidenceMedium, want: ConfidenceMedium},
		{name: "high and low stay low", first: ConfidenceHigh, later: ConfidenceLow, want: ConfidenceLow},
		{name: "medium is not raised by high", first: ConfidenceMedium, later: ConfidenceHigh, want: ConfidenceMedium},
		{name: "low is not raised by medium", first: ConfidenceLow, later: ConfidenceMedium, want: ConfidenceLow},
		{name: "an unrecognised value is not laundered", first: "certain", later: ConfidenceLow, want: "certain"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := IntelligenceSummary{Claims: []Claim{
				{
					Text:       "The council approved the budget.",
					SourceIDs:  []string{"wire"},
					Evidence:   []Quote{{SourceID: "wire", Quote: "approved the budget on Friday"}},
					Confidence: test.first,
				},
				{
					Text:       "  the council   approved the budget. ",
					SourceIDs:  []string{"daily"},
					Evidence:   []Quote{{SourceID: "daily", Quote: "the mayor said"}},
					Confidence: test.later,
				},
			}}

			got := Normalize(summary)

			if len(got.Claims) != 1 {
				t.Fatalf("Normalize() kept %d claims, want 1", len(got.Claims))
			}
			if got.Claims[0].Confidence != test.want {
				t.Fatalf("merged confidence = %q, want %q", got.Claims[0].Confidence, test.want)
			}
		})
	}
}

func TestNormalizeDoesNotMergeDistinctClaims(t *testing.T) {
	summary := IntelligenceSummary{Claims: []Claim{
		groundedClaim(),
		{
			Text:       "The mayor did not attend.",
			SourceIDs:  []string{"daily"},
			Evidence:   []Quote{{SourceID: "daily", Quote: "the mayor said"}},
			Confidence: ConfidenceMedium,
		},
	}}

	got := Normalize(summary)

	if len(got.Claims) != 2 {
		t.Fatalf("Normalize() kept %d claims, want 2", len(got.Claims))
	}
}

func TestNormalizeDropsEmptySourceIDsAndDuplicates(t *testing.T) {
	summary := validSummary()
	summary.Themes[0].SourceIDs = []string{" wire ", "", "wire", "daily"}

	got := Normalize(summary)

	want := []string{"wire", "daily"}
	if !reflect.DeepEqual(got.Themes[0].SourceIDs, want) {
		t.Fatalf("theme sources = %#v, want %#v", got.Themes[0].SourceIDs, want)
	}
}

func TestNormalizeDiscardsModelCitations(t *testing.T) {
	summary := validSummary()
	summary.Citations = []Citation{{
		SourceID:    "invented",
		Title:       "Invented",
		PublishedAt: "2099-01-01",
	}}

	got := Normalize(summary)

	if len(got.Citations) != 0 {
		t.Fatalf("Normalize() kept %d model citations, want 0: %#v", len(got.Citations), got.Citations)
	}
}

func TestCitationsFollowBatchOrder(t *testing.T) {
	batch := batchWithDuplicate()

	got := Citations(batch)

	want := []Citation{
		{SourceID: "wire", Title: "Wire report", PublishedAt: "2026-10-03", DuplicateOf: []string{"syndicated"}},
		{SourceID: "daily", Title: "Daily follow-up", PublishedAt: news.PublishedUnknown},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Citations() = %#v, want %#v", got, want)
	}
}

func TestCheckAttachesDerivedCitations(t *testing.T) {
	batch := batchWithDuplicate()

	got, err := Check(validSummary(), batch)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}

	if want := Citations(batch); !reflect.DeepEqual(got.Citations, want) {
		t.Fatalf("Check() citations = %#v, want the batch-derived %#v", got.Citations, want)
	}
	if got.Citations[0].SourceID != "wire" || len(got.Citations[0].DuplicateOf) != 1 {
		t.Fatalf("Check() lost the duplicate: %#v", got.Citations[0])
	}
}

func TestCheckMergesRepeatedClaimsBeforeValidating(t *testing.T) {
	summary := validSummary()
	summary.Claims = append(summary.Claims, Claim{
		Text:       "THE COUNCIL approved   the budget.",
		SourceIDs:  []string{"daily"},
		Evidence:   []Quote{{SourceID: "daily", Quote: "the mayor said"}},
		Confidence: ConfidenceMedium,
	})

	got, err := Check(summary, batch())
	if err != nil {
		t.Fatalf("Check() error = %v, want the repeated claim repaired by Normalize", err)
	}
	if len(got.Claims) != 1 {
		t.Fatalf("Check() returned %d claims, want 1", len(got.Claims))
	}
	if want := []string{"wire", "daily"}; !reflect.DeepEqual(got.Claims[0].SourceIDs, want) {
		t.Fatalf("Check() claim sources = %#v, want %#v", got.Claims[0].SourceIDs, want)
	}
}

func TestCheckRejectsUnknownSource(t *testing.T) {
	summary := validSummary()
	summary.Claims[0].SourceIDs = []string{"wire", "s9"}

	_, err := Check(summary, batch())
	if err == nil {
		t.Fatal("Check() = nil, want a contract violation")
	}
	if !errors.Is(err, ErrUngroundedSource) {
		t.Fatalf("errors.Is(%v, ErrUngroundedSource) = false, want true", err)
	}
}

func TestCheckRejectsAnEmptyBatch(t *testing.T) {
	empty := news.NewsBatch{Articles: []news.Article{
		{SourceID: "blank", Title: "Blank", PublishedAt: news.PublishedUnknown, Text: "  "},
	}}

	_, err := Check(validSummary(), empty)
	if !errors.Is(err, ErrEmptyBatch) {
		t.Fatalf("errors.Is(%v, ErrEmptyBatch) = false, want true", err)
	}
}
