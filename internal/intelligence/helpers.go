package intelligence

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// grounded reports whether quote appears in text delimited on both sides by the
// string edge or by a character that is neither a letter nor a digit.
//
// The boundary matters: without it an owner such as "Ann" would be satisfied by
// "annual", and a date such as "2026-10-03" by the identifier "X2026-10-03Z" —
// which is exactly the invention this project exists to catch.
func grounded(quote, text string) bool {
	quote = news.NormalizeText(quote)
	if quote == "" || text == "" {
		return false
	}

	for from := 0; from+len(quote) <= len(text); {
		at := strings.Index(text[from:], quote)
		if at < 0 {
			return false
		}
		start := from + at
		end := start + len(quote)
		if delimited(text, start, end) {
			return true
		}
		from = start + 1
	}
	return false
}

func delimited(text string, start, end int) bool {
	if start > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:start])
		if unicode.IsLetter(before) || unicode.IsDigit(before) {
			return false
		}
	}
	if end < len(text) {
		after, _ := utf8.DecodeRuneInString(text[end:])
		if unicode.IsLetter(after) || unicode.IsDigit(after) {
			return false
		}
	}
	return true
}

// repeatKey identifies two claims that say the same thing, so a claim supported
// by several articles is one claim with several sources instead of two claims
// that look independent.
func repeatKey(text string) string {
	return strings.ToLower(news.NormalizeText(text))
}

// anyUndated reports whether any cited article has no established date. Such a
// claim may be reported, but not as high confidence.
func anyUndated(ids []string, sources map[string]news.Article) bool {
	for _, id := range ids {
		if article, ok := sources[id]; ok && article.PublishedAt == news.PublishedUnknown {
			return true
		}
	}
	return false
}

// normalizeIDs trims ids, drops empty ones and removes repeats while keeping
// first-seen order.
func normalizeIDs(ids []string) []string {
	var normalized []string
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || contains(normalized, id) {
			continue
		}
		normalized = append(normalized, id)
	}
	return normalized
}

func unionIDs(first, second []string) []string {
	return normalizeIDs(append(append([]string(nil), first...), second...))
}

// confidenceRank orders the three contract values so a merge can never raise
// confidence. An unrecognised value has no rank, which keeps it visible to
// Validate instead of being quietly repaired.
func confidenceRank(value string) (int, bool) {
	switch value {
	case ConfidenceLow:
		return 0, true
	case ConfidenceMedium:
		return 1, true
	case ConfidenceHigh:
		return 2, true
	default:
		return 0, false
	}
}

// weakestConfidence returns the less certain of two recognised values. A claim
// assembled from several reports cannot be more certain than its least certain
// member, and a claim that also rests on an undated source is not high at all.
func weakestConfidence(first, later string) string {
	rank, ok := confidenceRank(first)
	if !ok {
		return first
	}
	other, ok := confidenceRank(later)
	if !ok || other >= rank {
		return first
	}
	return later
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func hasEvidence(evidence []Quote, want Quote) bool {
	for _, item := range evidence {
		if item == want {
			return true
		}
	}
	return false
}

// distinctNonEmpty counts distinct values after trimming, which is how a
// conflict is decided: two sides, not the same side written twice.
func distinctNonEmpty(values []string) int {
	return len(normalizeIDs(values))
}
