package main

import (
	"context"
	"log"
	"os"

	"github.com/GhandyP/adk-go-enterprise-cookbook/internal/adkhelper"
)

func main() {
	if err := adkhelper.Run(
		context.Background(),
		os.Args[1:],
		"case_18_news_and_intelligence_summary",
		"A text-only starting point for news and intelligence summaries.",
		"Summarize articles or news text supplied by the user. Deduplicate repeated claims, keep source labels supplied in the text, and distinguish facts from uncertainty; do not browse.",
	); err != nil {
		log.Fatal(err)
	}
}
