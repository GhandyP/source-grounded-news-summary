package summarizer

import (
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"google.golang.org/genai"
)

// TestSummarySchemaMatchesContract fails when the schema handed to the model and
// the structs the validator checks drift apart. A schema is a promise to the
// model; if the promise and the contract disagree, the model is being asked for
// something this project will then reject.
func TestSummarySchemaMatchesContract(t *testing.T) {
	schema := SummarySchema()

	if schema.Type != genai.TypeObject {
		t.Fatalf("schema type = %v, want an object", schema.Type)
	}

	// The contract carries citations but the schema must not: the project derives
	// them from the batch, and TestSummarySchemaOmitsDerivedCitations pins that.
	want := remove(jsonFieldNames(t, reflect.TypeOf(intelligence.IntelligenceSummary{})), "citations")
	got := sortedKeys(schema.Properties)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("summary schema properties = %v, want %v", got, want)
	}

	assertObjectFields(t, "claims", schema.Properties["claims"], reflect.TypeOf(intelligence.Claim{}))
	assertObjectFields(t, "evidence", schema.Properties["claims"].Items.Properties["evidence"], reflect.TypeOf(intelligence.Quote{}))
	assertObjectFields(t, "themes", schema.Properties["themes"], reflect.TypeOf(intelligence.Theme{}))
	assertObjectFields(t, "conflicts", schema.Properties["conflicts"], reflect.TypeOf(intelligence.Conflict{}))
	assertObjectFields(t, "gaps", schema.Properties["gaps"], reflect.TypeOf(intelligence.Gap{}))
}

func TestSummarySchemaOmitsDerivedCitations(t *testing.T) {
	if _, ok := SummarySchema().Properties["citations"]; ok {
		t.Fatal("citations must not be requested from the model: the project derives them from the batch")
	}
}

func TestExtractionSchemaMatchesTheExtractionContract(t *testing.T) {
	schema := ExtractionSchema()

	want := jsonFieldNames(t, reflect.TypeOf(Extraction{}))
	got := sortedKeys(schema.Properties)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("extraction schema properties = %v, want %v", got, want)
	}
	assertObjectFields(t, "claims", schema.Properties["claims"], reflect.TypeOf(intelligence.Claim{}))
}

func TestConfidenceSchemaPinsTheThreeValues(t *testing.T) {
	confidence := SummarySchema().Properties["claims"].Items.Properties["confidence"]

	if confidence.Type != genai.TypeString {
		t.Fatalf("confidence type = %v, want a string", confidence.Type)
	}
	want := []string{intelligence.ConfidenceHigh, intelligence.ConfidenceMedium, intelligence.ConfidenceLow}
	if !reflect.DeepEqual(confidence.Enum, want) {
		t.Fatalf("confidence enum = %v, want %v", confidence.Enum, want)
	}
}

func TestEveryObjectSchemaDeclaresItsRequiredFields(t *testing.T) {
	schema := SummarySchema()

	assertRequired(t, "summary", schema, true)
	assertRequired(t, "claims items", schema.Properties["claims"].Items, true)
	assertRequired(t, "evidence items", schema.Properties["claims"].Items.Properties["evidence"].Items, true)
	assertRequired(t, "themes items", schema.Properties["themes"].Items, true)
	assertRequired(t, "conflicts items", schema.Properties["conflicts"].Items, true)
	assertRequired(t, "gaps items", schema.Properties["gaps"].Items, true)
}

func assertObjectFields(t *testing.T, name string, schema *genai.Schema, contract reflect.Type) {
	t.Helper()

	if schema == nil {
		t.Fatalf("%s: schema is missing", name)
	}
	if schema.Type != genai.TypeArray {
		t.Fatalf("%s: type = %v, want an array", name, schema.Type)
	}
	want := jsonFieldNames(t, contract)
	got := sortedKeys(schema.Items.Properties)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s item properties = %v, want %v", name, got, want)
	}
}

func assertRequired(t *testing.T, name string, schema *genai.Schema, wantRequired bool) {
	t.Helper()

	got := append([]string(nil), schema.Required...)
	sort.Strings(got)
	want := sortedKeys(schema.Properties)
	if wantRequired && !reflect.DeepEqual(got, want) {
		t.Fatalf("%s required = %v, want every property %v", name, got, want)
	}
}

// jsonFieldNames reads the JSON tags of a contract struct, which are the names
// the model is asked for.
func jsonFieldNames(t *testing.T, contract reflect.Type) []string {
	t.Helper()

	names := make([]string, 0, contract.NumField())
	for i := 0; i < contract.NumField(); i++ {
		field := contract.Field(i)
		tag, ok := field.Tag.Lookup("json")
		if !ok || tag == "-" {
			continue
		}
		names = append(names, strings.Split(tag, ",")[0])
	}
	sort.Strings(names)
	return names
}

func sortedKeys(properties map[string]*genai.Schema) []string {
	keys := make([]string, 0, len(properties))
	for key := range properties {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func remove(values []string, drop string) []string {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if value != drop {
			kept = append(kept, value)
		}
	}
	return kept
}
