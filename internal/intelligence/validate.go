package intelligence

import (
	"fmt"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// Validate reports every way in which summary fails the contract against batch.
// It returns nil when the summary may be published, and Problems otherwise.
//
// Validate never mutates its input and is strict by design: it rejects what
// Normalize would repair, so a caller decides which repairs are acceptable
// instead of having them applied silently.
func Validate(summary IntelligenceSummary, batch news.NewsBatch) error {
	normalized := news.Normalize(batch)
	if len(normalized.Articles) == 0 {
		return Problems{{path: "batch", reason: "batch has no usable article text", kind: ErrEmptyBatch}}
	}

	// Every source id the caller supplied resolves, including the ids recorded as
	// duplicates: a wire copy printed by three outlets is one article, and a claim
	// citing the third outlet is citing a source the caller really did supply.
	// Grounding still happens against the canonical article's text.
	sources := make(map[string]news.Article, len(normalized.Articles))
	for _, article := range normalized.Articles {
		if _, ok := sources[article.SourceID]; !ok {
			sources[article.SourceID] = article
		}
		for _, duplicate := range article.DuplicateSourceIDs {
			if _, ok := sources[duplicate]; !ok {
				sources[duplicate] = article
			}
		}
	}

	var problems Problems
	problems = append(problems, validateClaims(summary.Claims, sources)...)
	problems = append(problems, validateThemes(summary.Themes, sources)...)
	problems = append(problems, validateConflicts(summary.Conflicts, sources)...)
	problems = append(problems, validateGaps(summary.Gaps, sources)...)

	if len(problems) == 0 {
		return nil
	}
	return problems
}

func validateClaims(claims []Claim, sources map[string]news.Article) Problems {
	var problems Problems
	seen := make(map[string]int, len(claims))

	for i, claim := range claims {
		path := fmt.Sprintf("claims[%d]", i)
		text := news.NormalizeText(claim.Text)

		if text == "" {
			problems = append(problems, problem{path: path + ".text", reason: "text must not be empty", kind: ErrEmptyText})
		}

		if len(claim.SourceIDs) == 0 {
			problems = append(problems, problem{path: path + ".source_ids", reason: "claim needs at least one source id", kind: ErrUngroundedSource})
		}
		for _, id := range claim.SourceIDs {
			if _, ok := sources[id]; !ok {
				problems = append(problems, problem{path: path + ".source_ids", reason: fmt.Sprintf("unknown source id %q", id), kind: ErrUngroundedSource})
			}
		}

		if len(claim.Evidence) == 0 {
			problems = append(problems, problem{path: path + ".evidence", reason: "claim needs at least one quoted evidence", kind: ErrUngroundedQuote})
		}
		for j, evidence := range claim.Evidence {
			evidencePath := fmt.Sprintf("%s.evidence[%d]", path, j)
			article, ok := sources[evidence.SourceID]
			if !ok {
				problems = append(problems, problem{path: evidencePath + ".source_id", reason: fmt.Sprintf("unknown source id %q", evidence.SourceID), kind: ErrUngroundedSource})
				continue
			}
			if !grounded(evidence.Quote, article.Text) {
				problems = append(problems, problem{path: evidencePath + ".quote", reason: "ungrounded quote", kind: ErrUngroundedQuote})
				continue
			}
			if len(claim.SourceIDs) > 0 && !contains(claim.SourceIDs, evidence.SourceID) {
				problems = append(problems, problem{path: evidencePath + ".source_id", reason: "quote source is not cited by the claim", kind: ErrUngroundedSource})
			}
		}

		switch claim.Confidence {
		case ConfidenceHigh, ConfidenceMedium, ConfidenceLow:
			if claim.Confidence == ConfidenceHigh && anyUndated(claim.SourceIDs, sources) {
				problems = append(problems, problem{path: path + ".confidence", reason: "unverified confidence on an undated source", kind: ErrUnverifiedConfidence})
			}
		default:
			problems = append(problems, problem{path: path + ".confidence", reason: "confidence must be high, medium or low", kind: ErrInvalidConfidence})
		}

		key := repeatKey(text)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			problems = append(problems, problem{path: path, reason: "repeated claim not merged", kind: ErrRepeatedClaim})
			continue
		}
		seen[key] = i
	}

	return problems
}

func validateThemes(themes []Theme, sources map[string]news.Article) Problems {
	var problems Problems

	for i, theme := range themes {
		path := fmt.Sprintf("themes[%d]", i)

		if news.NormalizeText(theme.Text) == "" {
			problems = append(problems, problem{path: path + ".text", reason: "text must not be empty", kind: ErrEmptyText})
		}
		if len(normalizeIDs(theme.SourceIDs)) == 0 {
			problems = append(problems, problem{path: path + ".source_ids", reason: "theme needs at least one source id", kind: ErrUngroundedSource})
		}
		problems = append(problems, unknownSourceProblems(path+".source_ids", theme.SourceIDs, sources)...)
	}

	return problems
}

func validateConflicts(conflicts []Conflict, sources map[string]news.Article) Problems {
	var problems Problems

	for i, conflict := range conflicts {
		path := fmt.Sprintf("conflicts[%d]", i)

		if news.NormalizeText(conflict.Subject) == "" {
			problems = append(problems, problem{path: path + ".subject", reason: "text must not be empty", kind: ErrEmptyText})
		}
		if len(normalizeIDs(conflict.SourceIDs)) < 2 {
			problems = append(problems, problem{path: path + ".source_ids", reason: "conflict needs at least two distinct sources", kind: ErrInvalidConflict})
		}
		problems = append(problems, unknownSourceProblems(path+".source_ids", conflict.SourceIDs, sources)...)
		if distinctNonEmpty(conflict.Positions) < 2 {
			problems = append(problems, problem{path: path + ".positions", reason: "conflict needs at least two distinct positions", kind: ErrInvalidConflict})
		}
	}

	return problems
}

func validateGaps(gaps []Gap, sources map[string]news.Article) Problems {
	var problems Problems

	for i, gap := range gaps {
		path := fmt.Sprintf("gaps[%d]", i)

		if news.NormalizeText(gap.Text) == "" {
			problems = append(problems, problem{path: path + ".text", reason: "text must not be empty", kind: ErrEmptyText})
		}
		// A gap may cite no article: "no article says this" is itself the finding.
		problems = append(problems, unknownSourceProblems(path+".source_ids", gap.SourceIDs, sources)...)
	}

	return problems
}

func unknownSourceProblems(path string, ids []string, sources map[string]news.Article) Problems {
	var problems Problems
	for _, id := range ids {
		if _, ok := sources[id]; !ok {
			problems = append(problems, problem{path: path, reason: fmt.Sprintf("unknown source id %q", id), kind: ErrUngroundedSource})
		}
	}
	return problems
}
