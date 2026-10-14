package summarizer

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

func TestNewWithoutCredentialsIsUnavailable(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "  ")
	t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "")

	_, err := New(context.Background(), Options{})

	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("errors.Is(%v, ErrModelUnavailable) = false, want true", err)
	}
}

func TestDecodeIntoAcceptsADecodedMap(t *testing.T) {
	// ADK may hand back either a map or a value already decoded into the target
	// type, so the seam decodes through JSON.
	output := map[string]any{
		"claims": []any{
			map[string]any{
				"text":       "The council approved the budget.",
				"source_ids": []any{"wire"},
				"evidence":   []any{map[string]any{"source_id": "wire", "quote": "approved the budget on Friday"}},
				"confidence": "high",
			},
		},
	}

	var extraction Extraction
	if err := decodeInto(output, &extraction); err != nil {
		t.Fatalf("decodeInto() error = %v", err)
	}

	if len(extraction.Claims) != 1 {
		t.Fatalf("decoded %d claims, want 1", len(extraction.Claims))
	}
	if extraction.Claims[0].Confidence != intelligence.ConfidenceHigh {
		t.Fatalf("decoded confidence = %q", extraction.Claims[0].Confidence)
	}
}

func TestDecodeIntoRejectsNilAndUnusableOutput(t *testing.T) {
	var target Extraction

	if err := decodeInto(nil, &target); !errors.Is(err, ErrModelOutput) {
		t.Fatalf("decodeInto(nil) = %v, want ErrModelOutput", err)
	}
	if err := decodeInto(make(chan int), &target); !errors.Is(err, ErrModelOutput) {
		t.Fatalf("decodeInto(chan) = %v, want ErrModelOutput", err)
	}
	if err := decodeInto([]byte("{"), &target); !errors.Is(err, ErrModelOutput) {
		t.Fatalf("decodeInto(bad JSON) = %v, want ErrModelOutput", err)
	}
}

func TestMergeStagesKeepsClaimsTheSynthesizerDropped(t *testing.T) {
	extraction := Extraction{Claims: candidate().Claims}

	merged := mergeStages(extraction, intelligence.IntelligenceSummary{
		Gaps: []intelligence.Gap{{Text: "No article states the vote margin."}},
	})

	if len(merged.Claims) != 1 {
		t.Fatalf("merged claims = %d, want the extracted claim to survive", len(merged.Claims))
	}
	if len(merged.Gaps) != 1 {
		t.Fatalf("merged gaps = %d, want the synthesizer's gaps kept", len(merged.Gaps))
	}
}

func TestMergeStagesPrefersSynthesizerClaims(t *testing.T) {
	extraction := Extraction{Claims: candidate().Claims}
	refined := candidate()
	refined.Claims[0].Text = "The council approved the budget on Friday, with two abstentions."

	merged := mergeStages(extraction, refined)

	if merged.Claims[0].Text != refined.Claims[0].Text {
		t.Fatalf("merged claim text = %q, want the synthesizer's %q", merged.Claims[0].Text, refined.Claims[0].Text)
	}
}

func TestBatchPromptCarriesTheNormalizedBatch(t *testing.T) {
	raw := news.NewsBatch{Articles: []news.Article{{
		SourceID:    "wire",
		Title:       "Wire  report",
		PublishedAt: "2026-10-03",
		Text:        "The council\tapproved   the budget.",
	}}}

	prompt, err := batchPrompt(raw)
	if err != nil {
		t.Fatalf("batchPrompt() error = %v", err)
	}

	if !strings.Contains(prompt, `"text":"The council approved the budget."`) {
		t.Fatalf("prompt does not carry the normalized text:\n%s", prompt)
	}
	if strings.Contains(prompt, "\t") {
		t.Fatalf("prompt carries a tab from the raw input:\n%s", prompt)
	}
}

func TestSynthesisPromptCarriesTheExtractedClaims(t *testing.T) {
	extraction := Extraction{Claims: candidate().Claims}

	prompt, err := synthesisPrompt("batch here", extraction)
	if err != nil {
		t.Fatalf("synthesisPrompt() error = %v", err)
	}

	if !strings.Contains(prompt, "batch here") {
		t.Fatalf("prompt dropped the batch:\n%s", prompt)
	}
	if !strings.Contains(prompt, "The council approved the budget.") {
		t.Fatalf("prompt does not carry the extracted claim:\n%s", prompt)
	}
}

func TestSessionIDsAreUnique(t *testing.T) {
	first := newSessionID()
	second := newSessionID()

	if first == second {
		t.Fatalf("newSessionID() returned %q twice; both stages would share a foreign session", first)
	}
	if first == "" {
		t.Fatal("newSessionID() returned an empty id")
	}
}
