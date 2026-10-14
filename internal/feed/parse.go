package feed

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"html"
	"net/url"
	"strings"
	"time"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// document covers both feed shapes this package reads. A document is treated as
// RSS or Atom according to its root element, so a feed that carries the wrong
// namespace is still read by its structure rather than rejected on a technicality.
type document struct {
	XMLName xml.Name
	Channel struct {
		Title string `xml:"title"`
		Items []item `xml:"item"`
	} `xml:"channel"`
	Entries []entry `xml:"entry"`
}

type item struct {
	Title          string `xml:"title"`
	Link           string `xml:"link"`
	GUID           string `xml:"guid"`
	PubDate        string `xml:"pubDate"`
	Description    string `xml:"description"`
	ContentEncoded string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
}

type entry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Links     []atomLink `xml:"link"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

func (e entry) linkHref() string {
	for _, link := range e.Links {
		if link.Rel == "" || link.Rel == "alternate" {
			return link.Href
		}
	}
	if len(e.Links) > 0 {
		return e.Links[0].Href
	}
	return ""
}

// Parse converts one feed document into the articles it carries.
//
// An entry with no body becomes an article with no text, which the batch
// normalization drops: deciding what counts as evidence stays in one place.
// An entry with no usable date becomes an undated article rather than a guessed
// one, so a claim resting on it cannot be published as high confidence.
func Parse(feedURL string, raw []byte) ([]news.Article, Format, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, "", fmt.Errorf("feed %s: the document is empty", label(feedURL))
	}

	var parsed document
	if err := xml.Unmarshal(raw, &parsed); err != nil {
		return nil, "", fmt.Errorf("feed %s: cannot read the document: %w", label(feedURL), err)
	}

	switch parsed.XMLName.Local {
	case "rss":
		return rssArticles(feedURL, parsed), FormatRSS, nil
	case "feed":
		return atomArticles(feedURL, parsed), FormatAtom, nil
	default:
		return nil, "", fmt.Errorf("feed %s: the document is neither RSS nor Atom (root element %q)",
			label(feedURL), parsed.XMLName.Local)
	}
}

func rssArticles(feedURL string, parsed document) []news.Article {
	articles := make([]news.Article, 0, len(parsed.Channel.Items))

	for i, item := range parsed.Channel.Items {
		identity := firstNonEmpty(item.GUID, item.Link, plainText(item.Title), fmt.Sprintf("item-%d", i))
		articles = append(articles, news.Article{
			SourceID:    sourceID(feedURL, identity),
			Title:       plainText(item.Title),
			PublishedAt: publishedAt(item.PubDate),
			Text:        plainText(firstNonEmpty(item.ContentEncoded, item.Description)),
		})
	}

	return articles
}

func atomArticles(feedURL string, parsed document) []news.Article {
	articles := make([]news.Article, 0, len(parsed.Entries))

	for i, entry := range parsed.Entries {
		identity := firstNonEmpty(entry.ID, entry.linkHref(), plainText(entry.Title), fmt.Sprintf("entry-%d", i))
		articles = append(articles, news.Article{
			SourceID:    sourceID(feedURL, identity),
			Title:       plainText(entry.Title),
			PublishedAt: publishedAt(firstNonEmpty(entry.Published, entry.Updated)),
			Text:        plainText(firstNonEmpty(entry.Content, entry.Summary)),
		})
	}

	return articles
}

// sourceID derives a citation id from the feed's host and the entry's own
// identity, so the same entry parsed twice is the same source and the same story
// syndicated into two feeds of one host is one source, not two confirmations.
func sourceID(feedURL, identity string) string {
	sum := sha256.Sum256([]byte(identity))
	return hostOf(feedURL) + ":" + hex.EncodeToString(sum[:])[:12]
}

func hostOf(feedURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(feedURL))
	if err != nil || parsed.Host == "" {
		return "feed"
	}
	return parsed.Host
}

func label(feedURL string) string {
	if trimmed := strings.TrimSpace(feedURL); trimmed != "" {
		return trimmed
	}
	return "with no url"
}

// publishedAt keeps the project's rule: a real calendar date, or the explicit
// sentinel that makes the uncertainty visible downstream.
func publishedAt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return news.PublishedUnknown
	}
	parsed, ok := parseDate(value)
	if !ok {
		return news.PublishedUnknown
	}
	return parsed.Format(time.DateOnly)
}

func parseDate(value string) (time.Time, bool) {
	layouts := []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.DateOnly,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}

	// RFC 822 and RFC 1123 allow a single-digit day ("Mon, 5 Oct 2026"), which
	// Go's own layouts reject, and real feeds do produce it. Retrying with the
	// day zero-padded accepts the date without guessing anything.
	if padded, ok := padDay(value); ok {
		for _, layout := range []string{time.RFC1123Z, time.RFC1123, time.RFC822Z, time.RFC822} {
			if parsed, err := time.Parse(layout, padded); err == nil {
				return parsed, true
			}
		}
	}

	return time.Time{}, false
}

func padDay(value string) (string, bool) {
	comma := strings.Index(value, ", ")
	if comma < 0 {
		return value, false
	}
	rest := value[comma+2:]
	if len(rest) < 2 {
		return value, false
	}
	if rest[0] < '0' || rest[0] > '9' {
		return value, false
	}
	if rest[1] >= '0' && rest[1] <= '9' {
		return value, false
	}
	return value[:comma+2] + "0" + rest, true
}

// plainText turns an entry body into the text a summary may quote: markup
// removed, entities decoded, whitespace collapsed. A feed that ships HTML must
// not force the model to quote markup the article does not contain as prose.
func plainText(value string) string {
	if strings.Contains(value, "<") {
		value = stripTags(value)
	}
	return news.NormalizeText(html.UnescapeString(value))
}

func stripTags(value string) string {
	var out strings.Builder
	inTag := false

	for _, r := range value {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
			out.WriteRune(' ')
		case !inTag:
			out.WriteRune(r)
		}
	}

	return out.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
