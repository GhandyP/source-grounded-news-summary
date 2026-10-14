package news

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestNormalizeTextCollapsesWhitespace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "only whitespace", in: " \t\n ", want: ""},
		{name: "single space kept", in: "a b", want: "a b"},
		{name: "runs collapse", in: "a   b", want: "a b"},
		{name: "tabs and newlines collapse", in: "a\t\n\tb", want: "a b"},
		{name: "leading and trailing trimmed", in: "\n  a b  \n", want: "a b"},
		{name: "unicode space collapses", in: "a\u00a0\u2003b", want: "a b"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := NormalizeText(test.in); got != test.want {
				t.Fatalf("NormalizeText(%q) = %q, want %q", test.in, got, test.want)
			}
		})
	}
}

func TestValidDate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "real date", in: "2026-10-03", want: true},
		{name: "unknown sentinel", in: PublishedUnknown, want: true},
		{name: "impossible day", in: "2026-02-31", want: false},
		{name: "impossible month", in: "2026-13-01", want: false},
		{name: "empty", in: "", want: false},
		{name: "local format", in: "03/10/2026", want: false},
		{name: "unpadded day", in: "2026-1-03", want: false},
		{name: "timestamp", in: "2026-10-03T09:00:00Z", want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := ValidDate(test.in); got != test.want {
				t.Fatalf("ValidDate(%q) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestNormalizeDropsArticlesWithoutText(t *testing.T) {
	batch := NewsBatch{Articles: []Article{
		{SourceID: "kept", Title: "Kept", PublishedAt: "2026-10-03", Text: "Real text."},
		{SourceID: "empty", Title: "Empty", PublishedAt: "2026-10-03", Text: ""},
		{SourceID: "blank", Title: "Blank", PublishedAt: "2026-10-03", Text: " \n\t "},
	}}

	got := Normalize(batch)

	want := []Article{{
		SourceID:    "kept",
		Title:       "Kept",
		PublishedAt: "2026-10-03",
		Text:        "Real text.",
	}}
	if !reflect.DeepEqual(got.Articles, want) {
		t.Fatalf("Normalize().Articles = %#v, want %#v", got.Articles, want)
	}
}

func TestNormalizeCollapsesIdenticalText(t *testing.T) {
	wire := "The council approved the budget on Friday."
	batch := NewsBatch{Articles: []Article{
		{SourceID: "wire", Title: "Wire", PublishedAt: "2026-10-03", Text: wire},
		{SourceID: "syndicated", Title: "Syndicated", PublishedAt: "2026-10-04", Text: "  " + wire + "\n"},
		{SourceID: "other", Title: "Other", PublishedAt: "2026-10-05", Text: "A different story."},
		{SourceID: "third-copy", Title: "Third copy", PublishedAt: "2026-10-06", Text: wire},
	}}

	got := Normalize(batch)

	if len(got.Articles) != 2 {
		t.Fatalf("Normalize() kept %d articles, want 2: %#v", len(got.Articles), got.Articles)
	}
	if got.Articles[0].SourceID != "wire" {
		t.Fatalf("canonical article = %q, want the first occurrence %q", got.Articles[0].SourceID, "wire")
	}
	wantDuplicates := []string{"syndicated", "third-copy"}
	if !reflect.DeepEqual(got.Articles[0].DuplicateSourceIDs, wantDuplicates) {
		t.Fatalf("DuplicateSourceIDs = %#v, want %#v", got.Articles[0].DuplicateSourceIDs, wantDuplicates)
	}
	if got.Articles[1].SourceID != "other" || len(got.Articles[1].DuplicateSourceIDs) != 0 {
		t.Fatalf("distinct text must not collapse: %#v", got.Articles[1])
	}
}

func TestNormalizeIsIdempotent(t *testing.T) {
	wire := "The council approved the budget on Friday."
	batch := NewsBatch{Articles: []Article{
		{SourceID: "wire", Title: "Wire", PublishedAt: "2026-10-03", Text: wire},
		{SourceID: "syndicated", Title: "Syndicated", PublishedAt: "2026-10-04", Text: wire},
		{SourceID: "blank", Title: "Blank", PublishedAt: PublishedUnknown, Text: "  "},
	}}

	once := Normalize(batch)
	twice := Normalize(once)

	if !reflect.DeepEqual(once, twice) {
		t.Fatalf("Normalize is not idempotent:\nonce  = %#v\ntwice = %#v", once, twice)
	}
}

func TestArticleJSONTags(t *testing.T) {
	encoded := []byte(`{"articles":[{"source_id":"wire","title":"Wire","published_at":"2026-10-03","text":"Body.","duplicate_source_ids":["syndicated"]}]}`)

	var batch NewsBatch
	if err := json.Unmarshal(encoded, &batch); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := []Article{{
		SourceID:           "wire",
		Title:              "Wire",
		PublishedAt:        "2026-10-03",
		Text:               "Body.",
		DuplicateSourceIDs: []string{"syndicated"},
	}}
	if !reflect.DeepEqual(batch.Articles, want) {
		t.Fatalf("decoded Articles = %#v, want %#v", batch.Articles, want)
	}

	roundTrip, err := json.Marshal(Article{SourceID: "wire", Title: "Wire", PublishedAt: "2026-10-03", Text: "Body."})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(roundTrip) != `{"source_id":"wire","title":"Wire","published_at":"2026-10-03","text":"Body."}` {
		t.Fatalf("Marshal() = %s, want the documented tags and no empty duplicate_source_ids", roundTrip)
	}
}
