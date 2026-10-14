// Command source-grounded-news-summary summarizes a supplied batch of news
// articles into claims that are grounded in the batch, and refuses a summary
// that cites a source nobody supplied or quotes a sentence nobody wrote.
//
// The command is a dry run: it prints a summary labelled as unverified
// intelligence and sends nothing anywhere.
package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "source-grounded-news-summary: the command line is not wired yet")
	os.Exit(2)
}
