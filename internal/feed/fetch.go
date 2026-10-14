package feed

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/GhandyP/source-grounded-news-summary/internal/news"
)

const (
	// MaxFeedBytes is the largest feed this project reads. A larger feed is
	// rejected rather than truncated: half an article would let a quotation be
	// checked against text nobody ever supplied in full.
	MaxFeedBytes int64 = 5 << 20
	// defaultTimeout bounds one feed request.
	defaultTimeout = 30 * time.Second
	// defaultUserAgent names the project, so a feed operator can see who asked.
	defaultUserAgent = "source-grounded-news-summary/1.0 (+https://github.com/GhandyP/source-grounded-news-summary)"
)

// Client fetches feeds with bounded resources.
type Client struct {
	// HTTPClient overrides the default client, whose own timeout applies.
	HTTPClient *http.Client
	// MaxBytes overrides MaxFeedBytes when greater than zero.
	MaxBytes int64
	// UserAgent overrides the default user agent when non-blank.
	UserAgent string
}

// Result is one feed's outcome. A failure keeps its URL and its error instead of
// vanishing, so a run can report exactly which feeds it could not read.
type Result struct {
	URL      string
	Format   Format
	Articles []news.Article
	Err      error
}

// Fetch retrieves one feed and parses it.
//
// Only a GET is ever issued, the response is size-capped, and a non-2xx status
// is an error rather than a body to parse.
func (c Client) Fetch(ctx context.Context, feedURL string) ([]news.Article, Format, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("feed %s: cannot form the request: %w", label(feedURL), err)
	}
	request.Header.Set("User-Agent", c.userAgent())
	request.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8")

	response, err := c.httpClient().Do(request)
	if err != nil {
		return nil, "", fmt.Errorf("feed %s: %w", label(feedURL), err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		return nil, "", fmt.Errorf("feed %s: HTTP %s", label(feedURL), response.Status)
	}

	limit := c.maxBytes()
	body, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", fmt.Errorf("feed %s: read the response: %w", label(feedURL), err)
	}
	if int64(len(body)) > limit {
		return nil, "", fmt.Errorf("feed %s: the feed is larger than %d bytes and was rejected rather than truncated",
			label(feedURL), limit)
	}

	return Parse(feedURL, body)
}

// FetchAll fetches every feed in order and reports each outcome. One failure
// never stops the others; the caller decides what a partial run means.
func (c Client) FetchAll(ctx context.Context, urls []string) []Result {
	results := make([]Result, 0, len(urls))

	for _, feedURL := range urls {
		articles, format, err := c.Fetch(ctx, feedURL)
		results = append(results, Result{URL: feedURL, Format: format, Articles: articles, Err: err})
	}

	return results
}

// ReadFeedURLs reads a URL list: one per line, blank lines and # comments
// ignored, duplicates collapsed in first-seen order.
func ReadFeedURLs(source io.Reader) ([]string, error) {
	if source == nil {
		return nil, nil
	}

	var urls []string
	seen := make(map[string]bool)

	scanner := bufio.NewScanner(source)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		urls = append(urls, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read the feed list: %w", err)
	}

	return urls, nil
}

func (c Client) httpClient() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: defaultTimeout}
}

func (c Client) maxBytes() int64 {
	if c.MaxBytes > 0 {
		return c.MaxBytes
	}
	return MaxFeedBytes
}

func (c Client) userAgent() string {
	if agent := strings.TrimSpace(c.UserAgent); agent != "" {
		return agent
	}
	return defaultUserAgent
}
