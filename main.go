// Command source-grounded-news-summary summarizes a batch of news articles into
// claims that are anchored in those articles.
//
// Every quotation must exist in the article it cites, every cited source must be
// one the caller supplied, and a claim that rests on an undated article cannot be
// published as high confidence. The command is a dry run: it prints a summary
// labelled as unverified intelligence and sends nothing anywhere.
package main

import (
	"context"
	"os"

	"github.com/GhandyP/source-grounded-news-summary/internal/cli"
)

func main() {
	os.Exit(cli.Run(context.Background(), os.Args[1:], os.Stdin, os.Stdout, os.Stderr, cli.Deps{}))
}
