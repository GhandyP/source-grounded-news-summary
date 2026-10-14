package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
	"github.com/GhandyP/source-grounded-news-summary/internal/summarizer"
)

const rssDocument = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0"><channel><title>Wire</title>
  <item>
    <title>Council approves budget</title>
    <guid>news-budget</guid>
    <pubDate>Fri, 03 Oct 2026 09:12:00 +0200</pubDate>
    <description>The council approved the budget on Friday.</description>
  </item>
  <item>
    <title>Ferry resumes</title>
    <guid>news-ferry</guid>
    <pubDate>Mon, 5 Oct 2026 08:05:00 +0000</pubDate>
    <description>The ferry service resumed on Saturday morning.</description>
  </item>
</channel></rss>`

// groundedDeps plays the part of the model honestly: it quotes each article in
// full and never claims high confidence for an undated one.
func groundedDeps(calls *int) Deps {
	return Deps{
		NewSummarizer: func(context.Context, summarizer.Options) (summarizer.Summarizer, error) {
			*calls++
			return summarizer.FakeFunc(func(_ context.Context, batch news.NewsBatch) (intelligence.IntelligenceSummary, error) {
				summary := intelligence.IntelligenceSummary{}
				for _, article := range batch.Articles {
					confidence := intelligence.ConfidenceHigh
					if article.PublishedAt == news.PublishedUnknown {
						confidence = intelligence.ConfidenceMedium
					}
					summary.Claims = append(summary.Claims, intelligence.Claim{
						Text:       "The batch states: " + article.Title,
						SourceIDs:  []string{article.SourceID},
						Evidence:   []intelligence.Quote{{SourceID: article.SourceID, Quote: article.Text}},
						Confidence: confidence,
					})
				}
				return summary, nil
			}), nil
		},
	}
}

func writeFile(t *testing.T, name, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", name, err)
	}
	return path
}

func rssServer(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feed.xml":
			w.Header().Set("Content-Type", "application/rss+xml")
			_, _ = w.Write([]byte(rssDocument))
		default:
			http.Error(w, "gone", http.StatusGone)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRunSummarizesFromFeeds(t *testing.T) {
	server := rssServer(t)
	calls := 0
	list := writeFile(t, "feeds.txt", "# morning run\n"+server.URL+"/feed.xml\n")

	code, stdout, stderr := run(t, []string{"-feeds", list}, "", groundedDeps(&calls))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}
	if calls != 1 {
		t.Fatalf("the summarizer was constructed %d times, want 1", calls)
	}
	if !strings.Contains(stderr, Banner) {
		t.Fatalf("stderr is missing the review banner: %q", stderr)
	}
	if strings.Contains(stderr, "feed ") {
		t.Fatalf("a healthy run reported a feed failure: %q", stderr)
	}

	var summary intelligence.IntelligenceSummary
	if err := json.Unmarshal([]byte(stdout), &summary); err != nil {
		t.Fatalf("stdout is not a summary: %v", err)
	}
	if len(summary.Claims) != 2 {
		t.Fatalf("claims = %d, want one per feed entry", len(summary.Claims))
	}
	if len(summary.Citations) != 2 {
		t.Fatalf("citations = %d, want the two entries", len(summary.Citations))
	}
	for _, citation := range summary.Citations {
		if citation.PublishedAt == "" {
			t.Fatalf("citation %#v lost its date", citation)
		}
	}
}

func TestRunReportsFeedFailuresButStillSummarizes(t *testing.T) {
	server := rssServer(t)
	calls := 0
	list := writeFile(t, "feeds.txt", server.URL+"/feed.xml\n"+server.URL+"/missing.xml\n")

	code, stdout, stderr := run(t, []string{"-feeds", list}, "", groundedDeps(&calls))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; a readable feed still produces a summary", code, ExitOK)
	}
	if !strings.Contains(stderr, server.URL+"/missing.xml") || !strings.Contains(stderr, "410") {
		t.Fatalf("stderr does not attribute the failure to its url: %q", stderr)
	}
	if !strings.Contains(stdout, `"claims"`) {
		t.Fatalf("stdout is missing the summary: %s", stdout)
	}
}

func TestRunFailsWhenNoFeedYieldsAnArticle(t *testing.T) {
	server := rssServer(t)
	calls := 0
	list := writeFile(t, "feeds.txt", server.URL+"/missing.xml\n")

	code, stdout, stderr := run(t, []string{"-feeds", list}, "", groundedDeps(&calls))

	if code != ExitUsage {
		t.Fatalf("Run() = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(stderr, "input has no article text to summarize") {
		t.Fatalf("stderr = %q, want the empty-batch message", stderr)
	}
	if calls != 0 {
		t.Fatalf("a run with no readable feed reached the model (%d calls)", calls)
	}
	if stdout != "" {
		t.Fatalf("a refused run wrote to stdout:\n%s", stdout)
	}
}

func TestRunMergesFeedsWithASuppliedBatch(t *testing.T) {
	server := rssServer(t)
	list := writeFile(t, "feeds.txt", server.URL+"/feed.xml\n")
	path := writeBatch(t, batchJSON)

	var seen news.NewsBatch
	deps := Deps{
		NewSummarizer: func(context.Context, summarizer.Options) (summarizer.Summarizer, error) {
			return summarizer.FakeFunc(func(_ context.Context, batch news.NewsBatch) (intelligence.IntelligenceSummary, error) {
				seen = batch
				return intelligence.IntelligenceSummary{Claims: []intelligence.Claim{{
					Text:       "ok",
					SourceIDs:  []string{batch.Articles[0].SourceID},
					Evidence:   []intelligence.Quote{{SourceID: batch.Articles[0].SourceID, Quote: batch.Articles[0].Text}},
					Confidence: intelligence.ConfidenceMedium,
				}}}, nil
			}), nil
		},
	}

	code, _, stderr := run(t, []string{"-input", path, "-feeds", list}, "", deps)

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitOK, stderr)
	}

	// The feed's budget entry is byte-identical to the supplied wire article, so
	// the batch collapses them: one story with two source ids, not two articles.
	// The merged batch is normalized, which is which is why the summarizer sees
	// three canonical articles and not four.
	if len(seen.Articles) != 3 {
		sources := make([]string, 0, len(seen.Articles))
		for _, article := range seen.Articles {
			sources = append(sources, article.SourceID)
		}
		t.Fatalf("the summarizer saw %d articles %v, want the 2 supplied plus 2 fetched with the identical one merged", len(seen.Articles), sources)
	}

	duplicates := 0
	fetchedSeen := false
	for _, article := range seen.Articles {
		duplicates += len(article.DuplicateSourceIDs)
		if strings.HasPrefix(article.SourceID, "127.0.0.1") {
			fetchedSeen = true
		}
	}
	if duplicates != 1 {
		t.Fatalf("duplicates = %d, want the feed entry identical to the supplied article recorded as its duplicate", duplicates)
	}
	if !fetchedSeen {
		t.Fatal("no article came from the feed, so the merge did not happen")
	}
}

func TestRunRejectsUnusableFeedLists(t *testing.T) {
	tests := []struct {
		name string
		args func(t *testing.T) []string
		want string
	}{
		{
			name: "missing file",
			args: func(t *testing.T) []string {
				return []string{"-feeds", filepath.Join(t.TempDir(), "absent.txt")}
			},
			want: "read ",
		},
		{
			name: "no urls",
			args: func(t *testing.T) []string {
				return []string{"-feeds", writeFile(t, "feeds.txt", "# nothing here\n\n")}
			},
			want: "no urls",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			calls := 0

			code, _, stderr := run(t, test.args(t), "", groundedDeps(&calls))

			if code != ExitUsage {
				t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitUsage, stderr)
			}
			if !strings.Contains(stderr, test.want) {
				t.Fatalf("stderr = %q, want it to contain %q", stderr, test.want)
			}
			if calls != 0 {
				t.Fatalf("an unusable feed list reached the model (%d calls)", calls)
			}
		})
	}
}

func TestRunDoesNotReadStandardInputWhenOnlyFeedsAreGiven(t *testing.T) {
	// A blocking pipe would hang the command if -input were read anyway.
	server := rssServer(t)
	list := writeFile(t, "feeds.txt", server.URL+"/feed.xml\n")

	var stdout, stderr bytes.Buffer
	code := Run(context.Background(), []string{"-feeds", list}, blockingReader{}, &stdout, &stderr, groundedDeps(new(int)))

	if code != ExitOK {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitOK, stderr.String())
	}
}

type blockingReader struct{}

func (blockingReader) Read([]byte) (int, error) {
	select {} // blocks forever; the command must never touch standard input here
}

func TestFetchFeedsDefaultsToTheRealClient(t *testing.T) {
	// A zero Deps must not panic: the feed list is read by the real client, which
	// reaches a server the test owns rather than the public internet.
	t.Setenv("GOOGLE_API_KEY", "")
	server := rssServer(t)
	list := writeFile(t, "feeds.txt", server.URL+"/missing.xml\n")

	code, _, stderr := run(t, []string{"-feeds", list}, "", Deps{})

	if code != ExitUsage {
		t.Fatalf("Run() = %d, want %d; stderr: %s", code, ExitUsage, stderr)
	}
	if !strings.Contains(stderr, "410") {
		t.Fatalf("stderr = %q, want the real fetch failure reported", stderr)
	}
}
