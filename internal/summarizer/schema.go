package summarizer

import (
	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"google.golang.org/genai"
)

// ExtractionSchema is the contract handed to the first agent: the claims it found
// and the quotation supporting each one.
func ExtractionSchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"claims": claimsSchema(),
		},
		Required: []string{"claims"},
	}
}

// SummarySchema is the contract handed to the second agent.
//
// It deliberately has no citations field: the citation index is derived from the
// batch in Go, so asking the model for it would invite a fabricated source or
// timestamp into the output.
func SummarySchema() *genai.Schema {
	return &genai.Schema{
		Type: genai.TypeObject,
		Properties: map[string]*genai.Schema{
			"claims": claimsSchema(),
			"themes": {
				Type:        genai.TypeArray,
				Description: "Subjects the batch covers together, each grounded in at least one source id.",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"text":       {Type: genai.TypeString, Description: "The subject several articles share."},
						"source_ids": sourceIDsSchema("The articles that carry this subject."),
					},
					Required: []string{"text", "source_ids"},
				},
			},
			"conflicts": {
				Type:        genai.TypeArray,
				Description: "Subjects where two or more articles disagree.",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"subject": {Type: genai.TypeString, Description: "What the articles disagree about."},
						"positions": {
							Type:        genai.TypeArray,
							Description: "The competing statements, at least two, each stated by one of the cited articles.",
							Items:       &genai.Schema{Type: genai.TypeString},
						},
						"source_ids": sourceIDsSchema("The articles on each side, at least two distinct ones."),
					},
					Required: []string{"subject", "positions", "source_ids"},
				},
			},
			"gaps": {
				Type:        genai.TypeArray,
				Description: "Material facts the batch does not establish, such as a missing date or a single-source claim.",
				Items: &genai.Schema{
					Type: genai.TypeObject,
					Properties: map[string]*genai.Schema{
						"text":       {Type: genai.TypeString, Description: "What the batch does not establish."},
						"source_ids": sourceIDsSchema("The articles concerned, which may be empty for a batch-wide gap."),
					},
					Required: []string{"text", "source_ids"},
				},
			},
		},
		Required: []string{"claims", "themes", "conflicts", "gaps"},
	}
}

func claimsSchema() *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeArray,
		Description: "Factual statements the batch supports. A claim stated by several articles is one entry listing every source.",
		Items: &genai.Schema{
			Type: genai.TypeObject,
			Properties: map[string]*genai.Schema{
				"text": {Type: genai.TypeString, Description: "The statement the articles make."},
				"source_ids": sourceIDsSchema(
					"Every article that states this claim. At least one, and only ids present in the batch.",
				),
				"evidence": {
					Type:        genai.TypeArray,
					Description: "One entry per cited article.",
					Items: &genai.Schema{
						Type: genai.TypeObject,
						Properties: map[string]*genai.Schema{
							"source_id": {Type: genai.TypeString, Description: "The article the quotation comes from."},
							"quote": {
								Type: genai.TypeString,
								Description: "The exact span of that article's text supporting the claim, copied character for " +
									"character. Never paraphrase a quotation.",
							},
						},
						Required: []string{"source_id", "quote"},
					},
				},
				"confidence": {
					Type: genai.TypeString,
					Description: "How well the batch supports the claim. \"high\" only when a dated article states it " +
						"plainly; \"medium\" when the article is undated or hedges; \"low\" when the claim is implied.",
					Enum: []string{
						intelligence.ConfidenceHigh,
						intelligence.ConfidenceMedium,
						intelligence.ConfidenceLow,
					},
				},
			},
			Required: []string{"text", "source_ids", "evidence", "confidence"},
		},
	}
}

func sourceIDsSchema(description string) *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeArray,
		Description: description,
		Items:       &genai.Schema{Type: genai.TypeString},
	}
}
