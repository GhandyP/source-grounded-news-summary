package intelligence

import (
	"errors"
	"reflect"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// batch is the fixed corpus the rule tests run against: a dated wire story, an
// undated follow-up that repeats part of it, and a column whose text contains
// the boundary traps this project exists to catch.
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
		{
			SourceID:    "column",
			Title:       "Column",
			PublishedAt: "2026-10-04",
			Text:        "The archive label is X2026-10-03Z and the annual report is late.",
		},
	}}
}

// groundedClaim is the smallest claim that satisfies every rule: one source that
// exists and is dated, one quotation copied out of it, and a matching confidence.
func groundedClaim() Claim {
	return Claim{
		Text:       "The council approved the budget.",
		SourceIDs:  []string{"wire"},
		Evidence:   []Quote{{SourceID: "wire", Quote: "The council approved the budget on Friday."}},
		Confidence: ConfidenceHigh,
	}
}

func validSummary() IntelligenceSummary {
	return IntelligenceSummary{
		Claims: []Claim{groundedClaim()},
		Themes: []Theme{{Text: "Budget approval", SourceIDs: []string{"wire", "daily"}}},
		Conflicts: []Conflict{{
			Subject:   "Budget timing",
			Positions: []string{"Friday", "not yet decided"},
			SourceIDs: []string{"wire", "daily"},
		}},
		Gaps: []Gap{{Text: "No article states the final vote margin."}},
	}
}

func TestValidateRules(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*IntelligenceSummary)
		want   []string
		kind   error
	}{
		{
			name:   "grounded summary passes",
			mutate: func(*IntelligenceSummary) {},
		},
		{
			name:   "claim text may not be empty",
			mutate: func(s *IntelligenceSummary) { s.Claims[0].Text = "   " },
			want:   []string{"claims[0].text: text must not be empty"},
			kind:   ErrEmptyText,
		},
		{
			name:   "claim needs at least one source",
			mutate: func(s *IntelligenceSummary) { s.Claims[0].SourceIDs = nil },
			want:   []string{"claims[0].source_ids: claim needs at least one source id"},
			kind:   ErrUngroundedSource,
		},
		{
			name:   "claim may not cite an unknown source",
			mutate: func(s *IntelligenceSummary) { s.Claims[0].SourceIDs = []string{"wire", "s9"} },
			want:   []string{`claims[0].source_ids: unknown source id "s9"`},
			kind:   ErrUngroundedSource,
		},
		{
			name:   "claim needs at least one quotation",
			mutate: func(s *IntelligenceSummary) { s.Claims[0].Evidence = nil },
			want:   []string{"claims[0].evidence: claim needs at least one quoted evidence"},
			kind:   ErrUngroundedQuote,
		},
		{
			name: "evidence may not cite an unknown source",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].Evidence = []Quote{{SourceID: "s9", Quote: "anything"}}
			},
			want: []string{`claims[0].evidence[0].source_id: unknown source id "s9"`},
			kind: ErrUngroundedSource,
		},
		{
			name: "evidence source must be cited by the claim",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].Evidence = []Quote{{SourceID: "daily", Quote: "the mayor said"}}
			},
			want: []string{"claims[0].evidence[0].source_id: quote source is not cited by the claim"},
			kind: ErrUngroundedSource,
		},
		{
			name: "invented quotation is rejected",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].Evidence = []Quote{{SourceID: "wire", Quote: "The council rejected the budget."}}
			},
			want: []string{"claims[0].evidence[0].quote: ungrounded quote"},
			kind: ErrUngroundedQuote,
		},
		{
			name: "an empty quotation is not grounded",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].Evidence = []Quote{{SourceID: "wire", Quote: "   "}}
			},
			want: []string{"claims[0].evidence[0].quote: ungrounded quote"},
			kind: ErrUngroundedQuote,
		},
		{
			name: "a name inside a longer word is not grounded",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].SourceIDs = []string{"column"}
				s.Claims[0].Evidence = []Quote{{SourceID: "column", Quote: "Ann"}}
			},
			want: []string{"claims[0].evidence[0].quote: ungrounded quote"},
			kind: ErrUngroundedQuote,
		},
		{
			name: "a date inside an identifier is not grounded",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].SourceIDs = []string{"column"}
				s.Claims[0].Evidence = []Quote{{SourceID: "column", Quote: "2026-10-03"}}
			},
			want: []string{"claims[0].evidence[0].quote: ungrounded quote"},
			kind: ErrUngroundedQuote,
		},
		{
			name: "a whole word inside the article is grounded",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].SourceIDs = []string{"column"}
				s.Claims[0].Evidence = []Quote{{SourceID: "column", Quote: "annual report"}}
			},
		},
		{
			name:   "confidence must be one of the three values",
			mutate: func(s *IntelligenceSummary) { s.Claims[0].Confidence = "certain" },
			want:   []string{"claims[0].confidence: confidence must be high, medium or low"},
			kind:   ErrInvalidConfidence,
		},
		{
			name: "an undated source cannot carry high confidence",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].SourceIDs = []string{"daily"}
				s.Claims[0].Evidence = []Quote{{SourceID: "daily", Quote: "the mayor said"}}
			},
			want: []string{"claims[0].confidence: unverified confidence on an undated source"},
			kind: ErrUnverifiedConfidence,
		},
		{
			name: "an undated source may carry medium confidence",
			mutate: func(s *IntelligenceSummary) {
				s.Claims[0].SourceIDs = []string{"daily"}
				s.Claims[0].Evidence = []Quote{{SourceID: "daily", Quote: "the mayor said"}}
				s.Claims[0].Confidence = ConfidenceMedium
			},
		},
		{
			name:   "theme text may not be empty",
			mutate: func(s *IntelligenceSummary) { s.Themes[0].Text = "" },
			want:   []string{"themes[0].text: text must not be empty"},
			kind:   ErrEmptyText,
		},
		{
			name:   "theme needs at least one source",
			mutate: func(s *IntelligenceSummary) { s.Themes[0].SourceIDs = nil },
			want:   []string{"themes[0].source_ids: theme needs at least one source id"},
			kind:   ErrUngroundedSource,
		},
		{
			name:   "theme may not cite an unknown source",
			mutate: func(s *IntelligenceSummary) { s.Themes[0].SourceIDs = []string{"wire", "s9"} },
			want:   []string{`themes[0].source_ids: unknown source id "s9"`},
			kind:   ErrUngroundedSource,
		},
		{
			name:   "conflict subject may not be empty",
			mutate: func(s *IntelligenceSummary) { s.Conflicts[0].Subject = "  " },
			want:   []string{"conflicts[0].subject: text must not be empty"},
			kind:   ErrEmptyText,
		},
		{
			name:   "conflict needs two sources",
			mutate: func(s *IntelligenceSummary) { s.Conflicts[0].SourceIDs = []string{"wire"} },
			want:   []string{"conflicts[0].source_ids: conflict needs at least two distinct sources"},
			kind:   ErrInvalidConflict,
		},
		{
			name:   "conflict needs two distinct positions",
			mutate: func(s *IntelligenceSummary) { s.Conflicts[0].Positions = []string{"Friday"} },
			want:   []string{"conflicts[0].positions: conflict needs at least two distinct positions"},
			kind:   ErrInvalidConflict,
		},
		{
			name: "conflict positions must differ",
			mutate: func(s *IntelligenceSummary) {
				s.Conflicts[0].Positions = []string{"Friday", "Friday"}
			},
			want: []string{"conflicts[0].positions: conflict needs at least two distinct positions"},
			kind: ErrInvalidConflict,
		},
		{
			name:   "conflict may not cite an unknown source",
			mutate: func(s *IntelligenceSummary) { s.Conflicts[0].SourceIDs = []string{"wire", "s9"} },
			want:   []string{`conflicts[0].source_ids: unknown source id "s9"`},
			kind:   ErrUngroundedSource,
		},
		{
			name:   "gap text may not be empty",
			mutate: func(s *IntelligenceSummary) { s.Gaps[0].Text = "\n" },
			want:   []string{"gaps[0].text: text must not be empty"},
			kind:   ErrEmptyText,
		},
		{
			name:   "a batch-wide gap needs no source",
			mutate: func(s *IntelligenceSummary) { s.Gaps[0].SourceIDs = nil },
		},
		{
			name:   "gap may not cite an unknown source",
			mutate: func(s *IntelligenceSummary) { s.Gaps[0].SourceIDs = []string{"s9"} },
			want:   []string{`gaps[0].source_ids: unknown source id "s9"`},
			kind:   ErrUngroundedSource,
		},
		{
			name: "a repeated claim must be merged",
			mutate: func(s *IntelligenceSummary) {
				s.Claims = append(s.Claims, Claim{
					Text:       "the council   APPROVED the budget.",
					SourceIDs:  []string{"daily"},
					Evidence:   []Quote{{SourceID: "daily", Quote: "the mayor said"}},
					Confidence: ConfidenceMedium,
				})
			},
			want: []string{"claims[1]: repeated claim not merged"},
			kind: ErrRepeatedClaim,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			summary := validSummary()
			test.mutate(&summary)

			err := Validate(summary, batch())

			if len(test.want) == 0 {
				if err != nil {
					t.Fatalf("Validate() = %v, want nil", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() = nil, want %v", test.want)
			}
			var problems Problems
			if !errors.As(err, &problems) {
				t.Fatalf("Validate() error type = %T, want Problems", err)
			}
			got := make([]string, 0, len(problems))
			for _, item := range problems {
				got = append(got, item.Error())
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("Validate() problems =\n%s\nwant\n%s", got, test.want)
			}
			if test.kind != nil && !errors.Is(err, test.kind) {
				t.Fatalf("errors.Is(%v, %v) = false, want true", err, test.kind)
			}
		})
	}
}

func TestValidateReportsEveryProblemInStableOrder(t *testing.T) {
	summary := validSummary()
	summary.Claims[0].Text = "  "
	summary.Claims[0].SourceIDs = []string{"s9"}
	summary.Claims[0].Evidence = []Quote{{SourceID: "wire", Quote: "invented"}}
	summary.Claims[0].Confidence = "certain"
	summary.Themes[0].SourceIDs = []string{"s9"}
	summary.Conflicts[0].Positions = []string{"only one side"}
	summary.Gaps[0].Text = ""

	want := []string{
		"claims[0].text: text must not be empty",
		`claims[0].source_ids: unknown source id "s9"`,
		"claims[0].evidence[0].quote: ungrounded quote",
		"claims[0].confidence: confidence must be high, medium or low",
		`themes[0].source_ids: unknown source id "s9"`,
		"conflicts[0].positions: conflict needs at least two distinct positions",
		"gaps[0].text: text must not be empty",
	}

	err := Validate(summary, batch())
	if err == nil {
		t.Fatalf("Validate() = nil, want %d problems", len(want))
	}

	var problems Problems
	if !errors.As(err, &problems) {
		t.Fatalf("Validate() error type = %T, want Problems", err)
	}
	got := make([]string, 0, len(problems))
	for _, item := range problems {
		got = append(got, item.Error())
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Validate() problems =\n%s\nwant\n%s", got, want)
	}
	if len(want) != len(problems) {
		t.Fatalf("Validate() reported %d problems, want %d", len(problems), len(want))
	}
}

func TestValidateRejectsAnEmptyBatch(t *testing.T) {
	empty := news.NewsBatch{Articles: []news.Article{
		{SourceID: "blank", Title: "Blank", PublishedAt: "2026-10-03", Text: " \n\t "},
	}}

	err := Validate(validSummary(), empty)
	if err == nil {
		t.Fatal("Validate() = nil, want an empty-batch problem")
	}
	if !errors.Is(err, ErrEmptyBatch) {
		t.Fatalf("errors.Is(%v, ErrEmptyBatch) = false, want true", err)
	}
	if got, want := err.Error(), "1 problem: batch: batch has no usable article text"; got != want {
		t.Fatalf("Validate() error = %q, want %q", got, want)
	}
}

func TestValidateDoesNotMutateItsInput(t *testing.T) {
	summary := validSummary()
	summary.Claims[0].Text = "  The council approved the budget.  "
	summary.Themes[0].SourceIDs = []string{"wire", "wire", " "}
	before := cloneSummary(summary)

	_ = Validate(summary, batch())

	if !reflect.DeepEqual(summary, before) {
		t.Fatalf("Validate() mutated its input:\ngot  %#v\nwant %#v", summary, before)
	}
}

func cloneSummary(summary IntelligenceSummary) IntelligenceSummary {
	clone := summary
	clone.Claims = make([]Claim, len(summary.Claims))
	for i, claim := range summary.Claims {
		clone.Claims[i] = claim
		clone.Claims[i].SourceIDs = append([]string(nil), claim.SourceIDs...)
		clone.Claims[i].Evidence = append([]Quote(nil), claim.Evidence...)
	}
	clone.Themes = make([]Theme, len(summary.Themes))
	for i, theme := range summary.Themes {
		clone.Themes[i] = theme
		clone.Themes[i].SourceIDs = append([]string(nil), theme.SourceIDs...)
	}
	clone.Conflicts = make([]Conflict, len(summary.Conflicts))
	for i, conflict := range summary.Conflicts {
		clone.Conflicts[i] = conflict
		clone.Conflicts[i].Positions = append([]string(nil), conflict.Positions...)
		clone.Conflicts[i].SourceIDs = append([]string(nil), conflict.SourceIDs...)
	}
	clone.Gaps = make([]Gap, len(summary.Gaps))
	for i, gap := range summary.Gaps {
		clone.Gaps[i] = gap
		clone.Gaps[i].SourceIDs = append([]string(nil), gap.SourceIDs...)
	}
	clone.Citations = append([]Citation(nil), summary.Citations...)
	return clone
}
