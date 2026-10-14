package news

import (
	"strings"
	"time"
	"unicode"
)

// NormalizeText collapses every run of Unicode whitespace to a single space and
// trims the result. It is the only normalization this project applies, so the
// text handed to a model and the text used to check a quotation are always the
// same bytes.
func NormalizeText(text string) string {
	return strings.Join(strings.FieldsFunc(text, unicode.IsSpace), " ")
}

// ValidDate reports whether date is PublishedUnknown or a real YYYY-MM-DD
// calendar date. A syntactically plausible but impossible date such as
// 2026-02-31 is not valid: the calendar, not the pattern, decides.
func ValidDate(date string) bool {
	if date == PublishedUnknown {
		return true
	}
	_, err := time.Parse(time.DateOnly, date)
	return err == nil
}

// Normalize returns the batch this project is allowed to reason about: text
// collapsed, articles without text dropped, and articles whose normalized text is
// identical collapsed into the first occurrence, with every other source id
// recorded on that canonical article.
//
// Normalize is idempotent, so a caller can apply it defensively without losing
// the duplicate record a previous pass already found.
func Normalize(batch NewsBatch) NewsBatch {
	normalized := NewsBatch{Articles: make([]Article, 0, len(batch.Articles))}
	canonical := make(map[string]int, len(batch.Articles))

	for _, article := range batch.Articles {
		known := append([]string(nil), article.DuplicateSourceIDs...)

		article.Title = NormalizeText(article.Title)
		article.Text = NormalizeText(article.Text)
		article.DuplicateSourceIDs = nil

		if article.Text == "" {
			// An article with no text is not evidence of anything.
			continue
		}
		if at, ok := canonical[article.Text]; ok {
			// A second copy is not a second article: it is one more source for
			// the same text, and hiding it would overstate how many outlets
			// reported the story.
			duplicates := append([]string(nil), normalized.Articles[at].DuplicateSourceIDs...)
			duplicates = append(duplicates, article.SourceID)
			duplicates = append(duplicates, known...)
			normalized.Articles[at].DuplicateSourceIDs = uniqueIDs(duplicates)
			continue
		}

		article.DuplicateSourceIDs = uniqueIDs(known)
		canonical[article.Text] = len(normalized.Articles)
		normalized.Articles = append(normalized.Articles, article)
	}

	return normalized
}

// uniqueIDs drops empty ids and repeats while keeping first-seen order. It
// returns nil for an empty result so a normalized value compares equal to a
// value that never carried duplicates.
func uniqueIDs(ids []string) []string {
	var unique []string
	for _, id := range ids {
		if id == "" || contains(unique, id) {
			continue
		}
		unique = append(unique, id)
	}
	return unique
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
