package feed

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestClientFetchesAndParsesAFeed(t *testing.T) {
	document := load(t, "rss.xml")
	var (
		gotUserAgent string
		gotMethod    string
		calls        int
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotUserAgent = r.UserAgent()
		gotMethod = r.Method
		w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
		_, _ = w.Write(document)
	}))
	defer server.Close()

	articles, format, err := Client{}.Fetch(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}

	if format != FormatRSS {
		t.Fatalf("format = %q, want %q", format, FormatRSS)
	}
	if len(articles) != 2 {
		t.Fatalf("Fetch() returned %d articles, want 2", len(articles))
	}
	if gotMethod != http.MethodGet {
		t.Fatalf("method = %q, want GET", gotMethod)
	}
	if calls != 1 {
		t.Fatalf("the feed was requested %d times, want once", calls)
	}
	if !strings.Contains(gotUserAgent, "source-grounded-news-summary") {
		t.Fatalf("user agent = %q, want it to name the project", gotUserAgent)
	}
}

func TestClientReportsANonSuccessStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no feed here", http.StatusNotFound)
	}))
	defer server.Close()

	articles, _, err := Client{}.Fetch(context.Background(), server.URL)

	if err == nil {
		t.Fatal("Fetch() = nil error, want the 404 reported")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("error = %q, want the status in it", err)
	}
	if len(articles) != 0 {
		t.Fatalf("a failed fetch returned %d articles", len(articles))
	}
}

func TestClientRejectsAnOversizedFeedRatherThanTruncatingIt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><rss version="2.0"><channel><title>Huge</title>`))
		padding := strings.Repeat("x", 4096)
		for i := 0; i < 40; i++ {
			fmt.Fprintf(w, "<item><title>item %d</title><description>%s</description></item>", i, padding)
		}
		_, _ = w.Write([]byte(`</channel></rss>`))
	}))
	defer server.Close()

	articles, _, err := Client{MaxBytes: 8192}.Fetch(context.Background(), server.URL)

	if err == nil {
		t.Fatal("an oversized feed must be rejected: truncating it would let the grounding rules pass on half an article")
	}
	if !strings.Contains(err.Error(), "8192") {
		t.Fatalf("error = %q, want the limit in it", err)
	}
	if len(articles) != 0 {
		t.Fatalf("a rejected feed returned %d articles", len(articles))
	}
}

func TestClientGivesUpOnAFeedThatNeverAnswers(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)

	started := time.Now()
	_, _, err := Client{HTTPClient: &http.Client{Timeout: 150 * time.Millisecond}}.Fetch(context.Background(), server.URL)

	if err == nil {
		t.Fatal("Fetch() = nil error, want the timeout reported")
	}
	if elapsed := time.Since(started); elapsed > 10*time.Second {
		t.Fatalf("Fetch() took %s; the timeout was not respected", elapsed)
	}
}

func TestFetchAllReportsEveryFeedInOrder(t *testing.T) {
	document := load(t, "rss.xml")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write(document)
		case "/broken":
			_, _ = w.Write([]byte("this is not a feed"))
		default:
			http.Error(w, "gone", http.StatusGone)
		}
	}))
	defer server.Close()

	urls := []string{server.URL + "/ok", server.URL + "/missing", server.URL + "/broken"}
	results := Client{}.FetchAll(context.Background(), urls)

	if len(results) != len(urls) {
		t.Fatalf("FetchAll() returned %d results for %d feeds; a failed feed must be reported, never dropped", len(results), len(urls))
	}
	for i, result := range results {
		if result.URL != urls[i] {
			t.Fatalf("results[%d].URL = %q, want %q so a failure can be attributed", i, result.URL, urls[i])
		}
	}

	if results[0].Err != nil {
		t.Fatalf("results[0].Err = %v, want the healthy feed to succeed", results[0].Err)
	}
	if len(results[0].Articles) != 2 || results[0].Format != FormatRSS {
		t.Fatalf("results[0] = %#v, want two parsed articles", results[0])
	}
	for _, i := range []int{1, 2} {
		if results[i].Err == nil {
			t.Fatalf("results[%d].Err = nil, want the failure reported", i)
		}
		if len(results[i].Articles) != 0 {
			t.Fatalf("results[%d] carried %d articles despite failing", i, len(results[i].Articles))
		}
	}
}

func TestReadFeedURLs(t *testing.T) {
	input := "# feeds for the morning run\r\n" +
		"\r\n" +
		"https://a.example/feed.xml\r\n" +
		"  https://b.example/atom.xml  \n" +
		"https://a.example/feed.xml\n" +
		"# a trailing comment\n"

	urls, err := ReadFeedURLs(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadFeedURLs() error = %v", err)
	}

	want := []string{"https://a.example/feed.xml", "https://b.example/atom.xml"}
	if !reflect.DeepEqual(urls, want) {
		t.Fatalf("ReadFeedURLs() = %#v, want %#v", urls, want)
	}
}

func TestReadFeedURLsAcceptsAnEmptyList(t *testing.T) {
	urls, err := ReadFeedURLs(strings.NewReader("\n# nothing but comments\n\n"))
	if err != nil {
		t.Fatalf("ReadFeedURLs() error = %v", err)
	}
	if len(urls) != 0 {
		t.Fatalf("ReadFeedURLs() = %#v, want no urls", urls)
	}
}
