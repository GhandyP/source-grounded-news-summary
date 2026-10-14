//go:build integration

// The live test is opt-in: it needs GOOGLE_API_KEY and network access, so the
// offline suite never depends on it. Run it with:
//
//	GOOGLE_API_KEY=... go test -tags=integration -run TestGeminiLive ./internal/summarizer/
package summarizer

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestGeminiLiveProducesAContractCheckedSummary(t *testing.T) {
	if strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")) == "" {
		t.Skip("GOOGLE_API_KEY is not set; the live Gemini path is not exercised")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	summarizer, err := New(ctx, Options{})
	if err != nil {
		t.Fatalf("NewGemini() error = %v", err)
	}

	got, err := summarizer.Summarize(ctx, batch())
	if err != nil {
		t.Fatalf("Summarize() error = %v", err)
	}

	if len(got.Claims) == 0 {
		t.Fatal("Summarize() returned no claims for a batch that states one")
	}
	if len(got.Citations) != len(batch().Articles) {
		t.Fatalf("Summarize() citations = %d, want %d", len(got.Citations), len(batch().Articles))
	}
}
