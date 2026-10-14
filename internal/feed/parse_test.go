package feed

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

func load(t *testing.T, name string) []byte {
	t.Helper()

	document, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}
	return document
}

func TestParseRSS(t *testing.T) {
	articles, format, err := Parse("https://example.com/rss.xml", load(t, "rss.xml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if format != FormatRSS {
		t.Fatalf("format = %q, want %q", format, FormatRSS)
	}

	want := []struct {
		title     string
		published string
		text      string
	}{
		{
			title:     "Council approves budget",
			published: "2026-10-03",
			text:      "The council approved the budget on Friday.",
		},
		{
			// The unpadded day "Mon, 5 Oct 2026" is a valid RFC 1123 date that
			// Go's own layout rejects until the day is zero-padded, and the body
			// arrives in content:encoded rather than description.
			title:     "Ferry resumes",
			published: "2026-10-05",
			text:      "The ferry service resumed on Saturday.",
		},
	}

	if len(articles) != len(want) {
		t.Fatalf("Parse() returned %d articles, want %d: %#v", len(articles), len(want), articles)
	}
	for i, expected := range want {
		if articles[i].Title != expected.title {
			t.Errorf("articles[%d].Title = %q, want %q", i, articles[i].Title, expected.title)
		}
		if articles[i].PublishedAt != expected.published {
			t.Errorf("articles[%d].PublishedAt = %q, want %q", i, articles[i].PublishedAt, expected.published)
		}
		if articles[i].Text != expected.text {
			t.Errorf("articles[%d].Text = %q, want %q", i, articles[i].Text, expected.text)
		}
	}
}

func TestParseAtom(t *testing.T) {
	articles, format, err := Parse("https://example.com/atom.xml", load(t, "atom.xml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if format != FormatAtom {
		t.Fatalf("format = %q, want %q", format, FormatAtom)
	}
	if len(articles) != 2 {
		t.Fatalf("Parse() returned %d articles, want 2", len(articles))
	}

	if got := articles[0]; got.Title != "Bridge reopens" || got.PublishedAt != "2026-09-28" ||
		got.Text != "The ministry said the bridge reopened on Monday." {
		t.Fatalf("articles[0] = %#v", got)
	}
	// An entry with no published and no updated date is undated, which the
	// contract turns into a visible uncertainty rather than a guessed date.
	if got := articles[1]; got.PublishedAt != news.PublishedUnknown ||
		got.Text != "Negotiators met again in the capital." {
		t.Fatalf("articles[1] = %#v", got)
	}
}

func TestParseTreatsMissingOrUnparseableDatesAsUnknown(t *testing.T) {
	articles, _, err := Parse("https://example.com/undated.xml", load(t, "missing-dates.xml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(articles) != 2 {
		t.Fatalf("Parse() returned %d articles, want 2", len(articles))
	}

	for i, article := range articles {
		if article.PublishedAt != news.PublishedUnknown {
			t.Fatalf("articles[%d].PublishedAt = %q, want %q", i, article.PublishedAt, news.PublishedUnknown)
		}
		if article.Text == "" {
			t.Fatalf("articles[%d].Text is empty; an undated article is still an article", i)
		}
	}
}

func TestParseEmptyFeed(t *testing.T) {
	articles, format, err := Parse("https://example.com/empty.xml", load(t, "empty.xml"))
	if err != nil {
		t.Fatalf("Parse() error = %v, want an empty feed to parse cleanly", err)
	}
	if format != FormatRSS {
		t.Fatalf("format = %q, want %q", format, FormatRSS)
	}
	if len(articles) != 0 {
		t.Fatalf("Parse() returned %d articles, want none", len(articles))
	}
}

func TestParseRejectsDocumentsItCannotRead(t *testing.T) {
	tests := []struct {
		name     string
		document string
	}{
		{name: "truncated XML", document: string(load(t, "malformed.xml"))},
		{name: "not XML at all", document: "<html><body>subscribe to read</body></html>"},
		{name: "empty document", document: ""},
		{name: "JSON instead of a feed", document: `{"articles":[]}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			articles, _, err := Parse("https://example.com/feed.xml", []byte(test.document))
			if err == nil {
				t.Fatalf("Parse() = %d articles and no error, want a refusal", len(articles))
			}
			if len(articles) != 0 {
				t.Fatalf("Parse() returned %d articles alongside the error", len(articles))
			}
		})
	}
}

func TestParseAssignsStableDistinctSourceIDs(t *testing.T) {
	document := load(t, "rss.xml")

	first, _, err := Parse("https://example.com/rss.xml", document)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	second, _, err := Parse("https://example.com/rss.xml", document)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("parsing the same document twice produced different articles:\n%#v\n%#v", first, second)
	}

	seen := map[string]bool{}
	for i, article := range first {
		if article.SourceID == "" {
			t.Fatalf("articles[%d] has no source id, so nothing can cite it", i)
		}
		if !strings.HasPrefix(article.SourceID, "example.com") {
			t.Fatalf("articles[%d].SourceID = %q, want it derived from the feed host", i, article.SourceID)
		}
		if seen[article.SourceID] {
			t.Fatalf("articles[%d].SourceID = %q is a duplicate inside one feed", i, article.SourceID)
		}
		seen[article.SourceID] = true
	}
}

func TestParseGivesTheSameStoryInTwoFeedsTheSameSourceID(t *testing.T) {
	// Two feeds on the same host carrying the same guid are the same story, so
	// the batch's duplicate rule can collapse them instead of counting two
	// confirmations.
	document := []byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Wire</title>
		<item><title>Ferry resumes</title><guid>shared-guid</guid><description>One sentence.</description></item>
	</channel></rss>`)
	other := []byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Syndicated</title>
		<item><title>Ferry resumes</title><guid>shared-guid</guid><description>One sentence.</description></item>
	</channel></rss>`)

	fromWire, _, err := Parse("https://example.com/a.xml", document)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	fromOther, _, err := Parse("https://example.com/b.xml", other)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if fromWire[0].SourceID != fromOther[0].SourceID {
		t.Fatalf("the same guid produced %q and %q, so a syndicated story would count twice",
			fromWire[0].SourceID, fromOther[0].SourceID)
	}
}

func TestParseArticlesSurviveNormalization(t *testing.T) {
	articles, _, err := Parse("https://example.com/rss.xml", load(t, "rss.xml"))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	normalized := news.Normalize(news.NewsBatch{Articles: articles})

	if len(normalized.Articles) != len(articles) {
		t.Fatalf("Normalize() kept %d of %d parsed articles", len(normalized.Articles), len(articles))
	}
	for i, article := range normalized.Articles {
		if article.Text == "" {
			t.Fatalf("normalized articles[%d] lost its text", i)
		}
	}
}
