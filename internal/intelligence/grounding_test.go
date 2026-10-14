package intelligence

import (
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

// The corpus mixes scripts, combining marks and the separators a naive boundary
// check gets wrong: a hyphen or an underscore is not a letter, a combining acute
// arrives as its own rune, and a soft hyphen is invisible in a terminal.
var groundingCorpus = []string{
	"The council approved the budget on Friday.",
	"annual report",
	"X2026-10-03Z",
	"9 to 2",
	"café",
	"cafe\u0301",
	"a\u0301vila",
	"Ann",
	"Ñandú",
	"ζήτα",
	"日本",
	"foo-bar_baz",
	"a\u00adb",
	"2026-02-31",
	"works",
}

var groundingSeparators = []string{
	"", " ", ".", "-", "_", "(", ")", ",", "\n", "\u00a0", "\u0301", "Z", "9", "'",
}

// TestGroundedNeverAcceptsAnAbsentQuote is the property this whole project rests
// on: when grounded reports that a quotation is supported, that quotation must
// really be present in the text. A validator that accepts a sentence nobody wrote
// makes every other rule pointless.
func TestGroundedNeverAcceptsAnAbsentQuote(t *testing.T) {
	cases := 0

	for _, text := range groundingCorpus {
		normalized := news.NormalizeText(text)

		for _, quote := range groundingCorpus {
			for _, separator := range groundingSeparators {
				candidate := quote + separator
				cases++

				if !grounded(candidate, normalized) {
					continue
				}

				folded := news.NormalizeText(candidate)
				if !strings.Contains(normalized, folded) {
					t.Fatalf("grounded(%q, %q) = true, but %q is absent from the text", candidate, normalized, folded)
				}
			}
		}
	}

	if cases < 1000 {
		t.Fatalf("only %d combinations were exercised; the corpus is too thin to prove anything", cases)
	}
}

// TestGroundedAcceptsEveryDelimitedOccurrence is the other half: no false
// negatives, so a model that quoted correctly is never refused.
func TestGroundedAcceptsEveryDelimitedOccurrence(t *testing.T) {
	text := news.NormalizeText("The council approved the budget on Friday. The budget rose.")

	supported := []string{
		"The council approved the budget",
		"budget",
		"on Friday",
		"The budget rose.",
		"Friday. The budget",
	}
	for _, quote := range supported {
		if !grounded(quote, text) {
			t.Fatalf("grounded(%q, %q) = false, want the quotation accepted", quote, text)
		}
	}
}

// A hyphen or an underscore really is a delimiter under the documented rule, so
// "report" inside "annual-report" is accepted on purpose: grounding is textual,
// and the README states that limit rather than hiding it.
func TestGroundedRejectsWordsAndIdentifiersThatMerelyContainTheValue(t *testing.T) {
	tests := []struct {
		name  string
		quote string
		text  string
	}{
		{name: "a name inside a longer word", quote: "Ann", text: "annual report"},
		{name: "a date inside an identifier", quote: "2026-10-03", text: "X2026-10-03Z"},
		{name: "a digit inside a year", quote: "9", text: "1990"},
		{name: "an empty quotation", quote: "", text: "annual report"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if grounded(test.quote, test.text) {
				t.Fatalf("grounded(%q, %q) = true, want false", test.quote, test.text)
			}
		})
	}
}
