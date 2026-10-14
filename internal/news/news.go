// Package news models a supplied batch of news articles and normalizes it into
// the only text this project treats as evidence.
//
// Nothing here knows about a model. The batch a caller supplies is also the
// batch a summary is validated against, so normalization has to be the single
// function that produces both the text handed to the model and the text used to
// check a quotation.
package news

// PublishedUnknown marks an article whose publication date is not established.
// It is a value, not an omission: an undated article is something the summary
// has to show, because a claim over it cannot be published as high confidence.
const PublishedUnknown = "unknown"

// Article is one supplied article. Its Text, after NormalizeText, is the only
// evidence this project accepts.
type Article struct {
	// SourceID identifies where the article came from. A summary may only cite
	// a source id that appears in the batch.
	SourceID string `json:"source_id"`
	// Title is the article headline as supplied.
	Title string `json:"title"`
	// PublishedAt is a real YYYY-MM-DD date, or PublishedUnknown.
	PublishedAt string `json:"published_at"`
	// Text is the article body as supplied.
	Text string `json:"text"`
	// DuplicateSourceIDs lists the source ids whose normalized text is identical
	// to this article's, in input order. Normalize fills it in; a caller never
	// has to supply it.
	DuplicateSourceIDs []string `json:"duplicate_source_ids,omitempty"`
}

// NewsBatch is the whole supplied corpus.
type NewsBatch struct {
	Articles []Article `json:"articles"`
}
