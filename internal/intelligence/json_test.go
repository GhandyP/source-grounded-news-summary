package intelligence

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSummaryJSONTags(t *testing.T) {
	encoded := []byte(`{
		"claims": [{
			"text": "The council approved the budget.",
			"source_ids": ["wire", "daily"],
			"evidence": [{"source_id": "wire", "quote": "approved the budget on Friday"}],
			"confidence": "high"
		}],
		"themes": [{"text": "Budget approval", "source_ids": ["wire"]}],
		"conflicts": [{"subject": "Budget timing", "positions": ["Friday", "later"], "source_ids": ["wire", "daily"]}],
		"gaps": [{"text": "No article states the vote margin.", "source_ids": ["wire"]}],
		"citations": [{"source_id": "wire", "title": "Wire report", "published_at": "2026-10-03", "duplicate_of": ["syndicated"]}]
	}`)

	var summary IntelligenceSummary
	if err := json.Unmarshal(encoded, &summary); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}

	want := IntelligenceSummary{
		Claims: []Claim{{
			Text:       "The council approved the budget.",
			SourceIDs:  []string{"wire", "daily"},
			Evidence:   []Quote{{SourceID: "wire", Quote: "approved the budget on Friday"}},
			Confidence: "high",
		}},
		Themes: []Theme{{Text: "Budget approval", SourceIDs: []string{"wire"}}},
		Conflicts: []Conflict{{
			Subject:   "Budget timing",
			Positions: []string{"Friday", "later"},
			SourceIDs: []string{"wire", "daily"},
		}},
		Gaps:      []Gap{{Text: "No article states the vote margin.", SourceIDs: []string{"wire"}}},
		Citations: []Citation{{SourceID: "wire", Title: "Wire report", PublishedAt: "2026-10-03", DuplicateOf: []string{"syndicated"}}},
	}
	if !reflect.DeepEqual(summary, want) {
		t.Fatalf("decoded summary = %#v\nwant %#v", summary, want)
	}
}

func TestClaimMarshalUsesDocumentedTags(t *testing.T) {
	encoded, err := json.Marshal(groundedClaim())
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	want := `{"text":"The council approved the budget.","source_ids":["wire"],` +
		`"evidence":[{"source_id":"wire","quote":"The council approved the budget on Friday."}],` +
		`"confidence":"high"}`
	if string(encoded) != want {
		t.Fatalf("Marshal() = %s\nwant       %s", encoded, want)
	}
}

func TestCitationOmitsEmptyDuplicateOf(t *testing.T) {
	encoded, err := json.Marshal(Citation{SourceID: "wire", Title: "Wire", PublishedAt: "2026-10-03"})
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	want := `{"source_id":"wire","title":"Wire","published_at":"2026-10-03"}`
	if string(encoded) != want {
		t.Fatalf("Marshal() = %s, want %s", encoded, want)
	}
}
