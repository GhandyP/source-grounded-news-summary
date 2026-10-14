package intelligence

import (
	"strings"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// Normalize repairs what can be repaired without inventing a fact: it trims text,
// normalizes quotations the same way article text is normalized, drops empty
// source ids, folds a repeated claim into one claim carrying every supporting
// source id, and discards any citation the candidate carries, because citations
// are always derived from the batch in Go.
//
// It repairs nothing else. A claim citing a source the batch does not contain, a
// quotation nobody wrote, or a confidence the sources cannot support stays broken
// and Validate rejects it.
func Normalize(summary IntelligenceSummary) IntelligenceSummary {
	normalized := IntelligenceSummary{
		Claims:    make([]Claim, 0, len(summary.Claims)),
		Themes:    make([]Theme, 0, len(summary.Themes)),
		Conflicts: make([]Conflict, 0, len(summary.Conflicts)),
		Gaps:      make([]Gap, 0, len(summary.Gaps)),
	}

	merged := make(map[string]int, len(summary.Claims))
	for _, claim := range summary.Claims {
		claim.Text = news.NormalizeText(claim.Text)
		claim.SourceIDs = normalizeIDs(claim.SourceIDs)
		claim.Confidence = strings.ToLower(strings.TrimSpace(claim.Confidence))
		claim.Evidence = normalizeEvidence(claim.Evidence)

		if key := repeatKey(claim.Text); key != "" {
			if at, ok := merged[key]; ok {
				normalized.Claims[at] = mergeClaims(normalized.Claims[at], claim)
				continue
			}
			merged[key] = len(normalized.Claims)
		}
		normalized.Claims = append(normalized.Claims, claim)
	}

	for _, theme := range summary.Themes {
		normalized.Themes = append(normalized.Themes, Theme{
			Text:      news.NormalizeText(theme.Text),
			SourceIDs: normalizeIDs(theme.SourceIDs),
		})
	}

	for _, conflict := range summary.Conflicts {
		normalized.Conflicts = append(normalized.Conflicts, Conflict{
			Subject:   news.NormalizeText(conflict.Subject),
			Positions: normalizeIDs(conflict.Positions),
			SourceIDs: normalizeIDs(conflict.SourceIDs),
		})
	}

	for _, gap := range summary.Gaps {
		normalized.Gaps = append(normalized.Gaps, Gap{
			Text:      news.NormalizeText(gap.Text),
			SourceIDs: normalizeIDs(gap.SourceIDs),
		})
	}

	return normalized
}

// mergeClaims folds a later claim with the same text into the first one. The
// first text wins and the support is the union of both, so a claim two outlets
// reported is reported once with two sources instead of twice as if it were two
// independent findings.
//
// The merged confidence is the weakest of the two. A claim supported by both an
// outlet that reported it confidently and one that hedged cannot be published as
// the confident version, and merging must never create a claim that S8 would
// then reject.
func mergeClaims(first, later Claim) Claim {
	first.SourceIDs = unionIDs(first.SourceIDs, later.SourceIDs)
	first.Confidence = weakestConfidence(first.Confidence, later.Confidence)
	for _, evidence := range later.Evidence {
		if !hasEvidence(first.Evidence, evidence) {
			first.Evidence = append(first.Evidence, evidence)
		}
	}
	return first
}

func normalizeEvidence(evidence []Quote) []Quote {
	normalized := make([]Quote, 0, len(evidence))
	for _, item := range evidence {
		normalized = append(normalized, Quote{
			SourceID: strings.TrimSpace(item.SourceID),
			Quote:    news.NormalizeText(item.Quote),
		})
	}
	return normalized
}
