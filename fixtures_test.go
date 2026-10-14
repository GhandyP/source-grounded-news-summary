// The fixture suite is the executable form of this project's promise: for every
// realistic batch, a summary that cites a source nobody supplied, quotes a
// sentence nobody wrote, or hides the uncertainty of an undated source must be
// refused, and a grounded summary must be accepted.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/cli"
	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
	"github.com/GhandyP/source-grounded-news-summary/internal/summarizer"
)

type rejection struct {
	name    string
	summary intelligence.IntelligenceSummary
	want    string
}

type fixture struct {
	name string
	// articles is how many canonical articles the batch has after normalization.
	articles int
	// duplicates is how many duplicate source ids the whole batch records.
	duplicates int
	// usage marks a batch that must be refused before a model is constructed.
	usage bool
	// accept must pass the contract and reach the reader.
	accept intelligence.IntelligenceSummary
	// rejections must each fail the contract with the given problem text.
	rejections []rejection
}

func claim(text string, sources []string, confidence string, evidence ...intelligence.Quote) intelligence.Claim {
	return intelligence.Claim{Text: text, SourceIDs: sources, Evidence: evidence, Confidence: confidence}
}

func quote(sourceID, text string) intelligence.Quote {
	return intelligence.Quote{SourceID: sourceID, Quote: text}
}

func fixtures() []fixture {
	return []fixture{
		{
			name:     "repeated-claim.json",
			articles: 3,
			accept: intelligence.IntelligenceSummary{
				Claims: []intelligence.Claim{claim("The council approved the budget.",
					[]string{"wire", "daily"}, intelligence.ConfidenceMedium,
					quote("wire", "The city council approved the 2027 budget on Friday by a vote of 9 to 2."),
					quote("daily", "The city council approved the 2027 budget on Friday by a vote of 9 to 2"),
				)},
				Themes: []intelligence.Theme{{Text: "Budget approval", SourceIDs: []string{"wire", "column"}}},
				Gaps:   []intelligence.Gap{{Text: "No article states when the levy takes effect."}},
			},
			// A candidate that splits one claim across two entries is deliberately
			// absent here: Normalize merges it before Validate sees it, so the command
			// repairs it rather than refusing it. TestRepeatedClaimsAreMergedOnTheWayIn
			// pins that, and the strict reading is pinned in internal/intelligence.
			rejections: []rejection{
				{
					name: "an undated source cannot carry high confidence",
					summary: intelligence.IntelligenceSummary{Claims: []intelligence.Claim{
						claim("The council approved the budget.", []string{"wire", "daily"}, intelligence.ConfidenceHigh,
							quote("wire", "approved the 2027 budget on Friday"),
							quote("daily", "the council approved the 2027 budget on Friday")),
					}},
					want: "claims[0].confidence: unverified confidence on an undated source",
				},
				{
					name: "a quotation with words the article does not contain",
					summary: intelligence.IntelligenceSummary{Claims: []intelligence.Claim{
						claim("The council approved the budget unanimously.", []string{"wire"}, intelligence.ConfidenceHigh,
							quote("wire", "The council approved the budget unanimously")),
					}},
					want: "claims[0].evidence[0].quote: ungrounded quote",
				},
			},
		},
		{
			name:     "conflicting-claims.json",
			articles: 2,
			accept: intelligence.IntelligenceSummary{
				Claims: []intelligence.Claim{
					claim("The ministry said the bridge reopened on Monday.", []string{"ministry"}, intelligence.ConfidenceHigh,
						quote("ministry", "the bridge reopened on Monday")),
					claim("Local crews say the bridge remains closed to heavy vehicles.", []string{"local"}, intelligence.ConfidenceHigh,
						quote("local", "remains closed to heavy vehicles")),
				},
				Conflicts: []intelligence.Conflict{{
					Subject:   "Whether the bridge is open",
					Positions: []string{"reopened on Monday", "still diverted around the bridge"},
					SourceIDs: []string{"ministry", "local"},
				}},
			},
			rejections: []rejection{
				{
					name: "a conflict needs two sides",
					summary: intelligence.IntelligenceSummary{
						Claims: []intelligence.Claim{claim("The bridge reopened.", []string{"ministry"}, intelligence.ConfidenceHigh,
							quote("ministry", "the bridge reopened on Monday"))},
						Conflicts: []intelligence.Conflict{{
							Subject:   "Whether the bridge is open",
							Positions: []string{"open", "closed"},
							SourceIDs: []string{"ministry"},
						}},
					},
					want: "conflicts[0].source_ids: conflict needs at least two distinct sources",
				},
			},
		},
		{
			name:       "duplicate-articles.json",
			articles:   2,
			duplicates: 1,
			accept: intelligence.IntelligenceSummary{
				Claims: []intelligence.Claim{claim("The ferry service resumed on Saturday morning.",
					[]string{"wire", "syndicated"}, intelligence.ConfidenceHigh,
					quote("wire", "The ferry service resumed on Saturday morning"),
					quote("syndicated", "two weeks of repairs to the northern pier"),
				)},
				Gaps: []intelligence.Gap{{Text: "The batch records the wire copy twice, so it is one report, not two confirmations."}},
			},
			rejections: []rejection{
				{
					name: "a claim may not cite a source the batch never had",
					summary: intelligence.IntelligenceSummary{Claims: []intelligence.Claim{
						claim("The ferry resumed.", []string{"wire", "unknown-outlet"}, intelligence.ConfidenceHigh,
							quote("wire", "The ferry service resumed on Saturday morning")),
					}},
					want: `claims[0].source_ids: unknown source id "unknown-outlet"`,
				},
			},
		},
		{
			name:     "missing-dates.json",
			articles: 2,
			accept: intelligence.IntelligenceSummary{
				Claims: []intelligence.Claim{claim("Negotiators met again in the capital.",
					[]string{"undated-a"}, intelligence.ConfidenceMedium,
					quote("undated-a", "Negotiators met again in the capital"),
				)},
				Gaps: []intelligence.Gap{{Text: "Neither article carries a publication date, so nothing in this summary can be placed in time."}},
			},
			rejections: []rejection{
				{
					name: "an undated claim may not be high",
					summary: intelligence.IntelligenceSummary{Claims: []intelligence.Claim{
						claim("Negotiators met again in the capital.", []string{"undated-a"}, intelligence.ConfidenceHigh,
							quote("undated-a", "Negotiators met again in the capital")),
					}},
					want: "claims[0].confidence: unverified confidence on an undated source",
				},
			},
		},
		{
			name:     "paywall-snippet.json",
			articles: 1,
			accept: intelligence.IntelligenceSummary{
				Claims: []intelligence.Claim{claim("The investigation is available to subscribers.",
					[]string{"subscriber"}, intelligence.ConfidenceHigh,
					quote("subscriber", "This investigation into the port contract is available to subscribers."),
				)},
				Gaps: []intelligence.Gap{{Text: "The batch only carries a paywall notice, so the investigation's findings are not established."}},
			},
			rejections: []rejection{
				{
					name: "a snippet cannot support the story it hides",
					summary: intelligence.IntelligenceSummary{Claims: []intelligence.Claim{
						claim("The port contract was awarded without a tender.", []string{"subscriber"}, intelligence.ConfidenceMedium,
							quote("subscriber", "The port contract was awarded without a tender"),
						),
					}},
					want: "claims[0].evidence[0].quote: ungrounded quote",
				},
			},
		},
		{
			name:  "empty.json",
			usage: true,
			accept: intelligence.IntelligenceSummary{
				Claims: []intelligence.Claim{claim("Nothing to summarize.", []string{"blank"}, intelligence.ConfidenceLow,
					quote("blank", "Feed returned nothing"))},
			},
		},
	}
}

func loadBatch(t *testing.T, name string) news.NewsBatch {
	t.Helper()

	encoded, err := os.ReadFile(filepath.Join("fixtures", name))
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", name, err)
	}

	var batch news.NewsBatch
	if err := json.Unmarshal(encoded, &batch); err != nil {
		t.Fatalf("fixture %s is not a news batch: %v", name, err)
	}
	return batch
}

func TestFixturesProduceTheExpectedBatch(t *testing.T) {
	for _, test := range fixtures() {
		t.Run(test.name, func(t *testing.T) {
			normalized := news.Normalize(loadBatch(t, test.name))

			duplicates := 0
			for _, article := range normalized.Articles {
				duplicates += len(article.DuplicateSourceIDs)
			}

			if len(normalized.Articles) != test.articles {
				t.Fatalf("normalized articles = %d, want %d", len(normalized.Articles), test.articles)
			}
			if duplicates != test.duplicates {
				t.Fatalf("recorded duplicates = %d, want %d", duplicates, test.duplicates)
			}
		})
	}
}

// runFixture drives the whole command line with a fake summarizer: no key, no
// network, and the same policy a live run would face.
func runFixture(t *testing.T, name string, candidate intelligence.IntelligenceSummary) (int, string, string, bool) {
	t.Helper()

	constructed := false
	deps := cli.Deps{
		NewSummarizer: func(context.Context, summarizer.Options) (summarizer.Summarizer, error) {
			constructed = true
			return summarizer.FakeFunc(func(context.Context, news.NewsBatch) (intelligence.IntelligenceSummary, error) {
				return candidate, nil
			}), nil
		},
	}

	var stdout, stderr bytes.Buffer
	code := cli.Run(context.Background(), []string{"-input", filepath.Join("fixtures", name)},
		strings.NewReader(""), &stdout, &stderr, deps)

	return code, stdout.String(), stderr.String(), constructed
}

func TestFixturesAcceptGroundedSummaries(t *testing.T) {
	for _, test := range fixtures() {
		if test.usage {
			continue
		}
		t.Run(test.name, func(t *testing.T) {
			code, stdout, stderr, constructed := runFixture(t, test.name, test.accept)

			if code != cli.ExitOK {
				t.Fatalf("exit = %d, want %d; stderr: %s", code, cli.ExitOK, stderr)
			}
			if !constructed {
				t.Fatal("the summarizer was never constructed")
			}
			if !strings.Contains(stderr, cli.Banner) {
				t.Fatalf("stderr does not carry the review banner: %q", stderr)
			}

			var summary intelligence.IntelligenceSummary
			if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
				t.Fatalf("stdout is not a summary: %v\n%s", err, stdout)
			}
			if len(summary.Citations) == 0 {
				t.Fatal("the output carries no citations, so nothing is traceable")
			}
		})
	}
}

func TestFixturesRefuseInvention(t *testing.T) {
	for _, test := range fixtures() {
		for _, rejected := range test.rejections {
			t.Run(test.name+": "+rejected.name, func(t *testing.T) {
				code, stdout, stderr, _ := runFixture(t, test.name, rejected.summary)

				if code != cli.ExitContractViolation {
					t.Fatalf("exit = %d, want %d; stderr: %s", code, cli.ExitContractViolation, stderr)
				}
				if !strings.Contains(stderr, rejected.want) {
					t.Fatalf("stderr = %q, want it to contain %q", stderr, rejected.want)
				}
				if stdout != "" {
					t.Fatalf("a refused summary still reached standard output:\n%s", stdout)
				}
			})
		}
	}
}

func TestDuplicateArticlesAreVisibleInTheOutput(t *testing.T) {
	test := fixtures()[2]
	if test.name != "duplicate-articles.json" {
		t.Fatalf("the duplicate fixture moved: %s", test.name)
	}

	code, stdout, stderr, _ := runFixture(t, test.name, test.accept)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, cli.ExitOK, stderr)
	}

	var summary intelligence.IntelligenceSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("stdout is not a summary: %v", err)
	}
	if len(summary.Citations) != 2 {
		t.Fatalf("citations = %d, want 2 canonical articles", len(summary.Citations))
	}
	if got := summary.Citations[0].DuplicateOf; len(got) != 1 || got[0] != "syndicated" {
		t.Fatalf("citations[0].duplicate_of = %#v, want the syndicated copy recorded", got)
	}
}

func TestRepeatedClaimsAreMergedOnTheWayIn(t *testing.T) {
	split := intelligence.IntelligenceSummary{Claims: []intelligence.Claim{
		claim("The council approved the budget.", []string{"wire"}, intelligence.ConfidenceHigh,
			quote("wire", "approved the 2027 budget on Friday")),
		claim("THE COUNCIL   approved the budget.", []string{"daily"}, intelligence.ConfidenceLow,
			quote("daily", "debate that ran past midnight")),
	}}

	code, stdout, stderr, _ := runFixture(t, "repeated-claim.json", split)
	if code != cli.ExitOK {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, cli.ExitOK, stderr)
	}

	var summary intelligence.IntelligenceSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("stdout is not a summary: %v", err)
	}
	if len(summary.Claims) != 1 {
		t.Fatalf("claims = %d, want the two entries merged into one", len(summary.Claims))
	}
	if want := []string{"wire", "daily"}; !reflect.DeepEqual(summary.Claims[0].SourceIDs, want) {
		t.Fatalf("merged sources = %#v, want %#v", summary.Claims[0].SourceIDs, want)
	}
	if summary.Claims[0].Confidence != intelligence.ConfidenceLow {
		t.Fatalf("merged confidence = %q, want the weakest of the two %q", summary.Claims[0].Confidence, intelligence.ConfidenceLow)
	}
}

func TestEmptyFixtureIsRefusedBeforeTheModel(t *testing.T) {
	code, stdout, stderr, constructed := runFixture(t, "empty.json", fixtures()[5].accept)

	if code != cli.ExitUsage {
		t.Fatalf("exit = %d, want %d", code, cli.ExitUsage)
	}
	if !strings.Contains(stderr, "input has no article text to summarize") {
		t.Fatalf("stderr = %q, want the empty-batch message", stderr)
	}
	if constructed {
		t.Fatal("an empty batch reached the model")
	}
	if stdout != "" {
		t.Fatalf("an empty batch produced output:\n%s", stdout)
	}
}

// TestModuleHasNoOutboundSurface is the machine-checkable half of the dry-run
// promise: nothing in this module can send, publish or execute anything. The
// model call is the only thing that leaves the process, and it lives in one
// package that imports the ADK client, not an HTTP or SMTP surface of its own.
func TestModuleHasNoOutboundSurface(t *testing.T) {
	forbidden := map[string]string{
		"net/http": "an HTTP client",
		"net/smtp": "an SMTP client",
		"net/rpc":  "an RPC client",
		"os/exec":  "a process launcher",
	}

	scanned := 0
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "testdata") {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		scanned++

		for _, imported := range parsed.Imports {
			name := strings.Trim(imported.Path.Value, `"`)
			if what, ok := forbidden[name]; ok {
				t.Errorf("%s imports %s (%s); this project is a dry run and has no outbound surface", path, name, what)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if scanned < 15 {
		t.Fatalf("only %d Go files were scanned; the walk is not seeing the module", scanned)
	}
}
