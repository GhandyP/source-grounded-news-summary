// Package feed turns a fetched feed document into the articles a summary can be
// grounded in.
//
// Parsing is pure and offline: it takes the bytes someone else fetched and
// returns articles with the same identity rules the rest of the project uses, so
// the grounding rules apply to a feed exactly as they apply to a batch a caller
// supplied by hand.
package feed

// Format is the feed document format Parse recognised.
type Format string

const (
	// FormatRSS is an RSS 2.0 document.
	FormatRSS Format = "rss"
	// FormatAtom is an Atom document.
	FormatAtom Format = "atom"
)
