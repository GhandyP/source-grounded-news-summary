// Package cli implements the source-grounded-news-summary command line.
//
// The command is a dry run by design: it prints a summary labelled as unverified
// intelligence and has no way to send, publish or alert anything.
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/GhandyP/source-grounded-news-summary/internal/alert"
	"github.com/GhandyP/source-grounded-news-summary/internal/feed"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
	"github.com/GhandyP/source-grounded-news-summary/internal/summarizer"
)

const (
	// ExitOK means a contract-checked summary was produced.
	ExitOK = 0
	// ExitFailure means the model, the network or the output stream failed.
	ExitFailure = 1
	// ExitUsage means the request could not be formed.
	ExitUsage = 2
	// ExitContractViolation means the summary violated the contract.
	ExitContractViolation = 3
	// ExitApprovalRequired means the alert candidate is ready and nobody approved it yet.
	ExitApprovalRequired = 4
	// ExitAlreadyDelivered means that exact alert was written before.
	ExitAlreadyDelivered = 5
)

// Banner labels every produced summary. It is written to standard error, so a
// machine reading JSON from standard output still sees the label.
const Banner = "unverified intelligence - human review required before any external action"

// Deps carries the collaborators Run needs, so tests can drive the command
// without a model. A zero Deps uses the live Gemini summarizer.
type Deps struct {
	NewSummarizer func(ctx context.Context, opts summarizer.Options) (summarizer.Summarizer, error)
}

// Run executes the command and returns the process exit code.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, deps Deps) int {
	if ctx == nil {
		ctx = context.Background()
	}
	if stdin == nil {
		stdin = strings.NewReader("")
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	if deps.NewSummarizer == nil {
		// The adapter keeps a failed construction from becoming a non-nil
		// interface holding a nil pointer.
		deps.NewSummarizer = func(ctx context.Context, opts summarizer.Options) (summarizer.Summarizer, error) {
			engine, err := summarizer.New(ctx, opts)
			if err != nil {
				return nil, err
			}
			return engine, nil
		}
	}

	var inputPath string
	var feedsPath string
	var alertPath string
	var approval string
	var outputFormat string
	var provider string
	var model string
	var timeout time.Duration
	var verbose bool

	flags := flag.NewFlagSet("source-grounded-news-summary", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Summarize a batch of articles without letting the summary invent a source.")
		flags.PrintDefaults()
	}
	flags.StringVar(&inputPath, "input", "-", "news batch JSON path, or - for standard input; ignored when -feeds is used without it")
	flags.StringVar(&feedsPath, "feeds", "", "path to a file listing feed urls, one per line")
	flags.StringVar(&alertPath, "alert", "", "write the checked summary to this path once its digest is approved")
	flags.StringVar(&approval, "approve", "", "the digest a previous run printed; approves exactly that alert")
	flags.StringVar(&outputFormat, "out", "json", "output format: json or text")
	flags.StringVar(&provider, "provider", "", "model provider: apigee, gemini or openai (default gemini)")
	flags.StringVar(&model, "model", "", "model name for the selected provider")
	flags.DurationVar(&timeout, "timeout", 60*time.Second, "fetch and summarization timeout; zero or negative disables the deadline")
	flags.BoolVar(&verbose, "verbose", false, "write diagnostics to standard error")

	articles := 0
	inputBytes := 0
	fetched := 0
	finish := func(code int, reason string) int {
		if verbose {
			fmt.Fprintf(stderr, "articles: %d\ninput bytes: %d\nfeeds: %d\nexit reason: %s\n", articles, inputBytes, fetched, reason)
		}
		return code
	}
	fail := func(code int, err error) int {
		if err != nil {
			fmt.Fprintln(stderr, err)
			return finish(code, err.Error())
		}
		return finish(code, "operation failed")
	}

	if err := flags.Parse(args); err != nil {
		return fail(ExitUsage, err)
	}
	if positional := flags.Args(); len(positional) > 0 {
		return fail(ExitUsage, fmt.Errorf("unexpected argument %q: the batch is read from -input", positional[0]))
	}
	switch outputFormat {
	case "json", "text":
	default:
		return fail(ExitUsage, fmt.Errorf("unknown output format %q: use json or text", outputFormat))
	}
	if strings.TrimSpace(approval) != "" && strings.TrimSpace(alertPath) == "" {
		return fail(ExitUsage, errors.New("-approve was given without -alert, so there is nothing to approve"))
	}

	explicit := make(map[string]bool, len(args))
	flags.Visit(func(set *flag.Flag) { explicit[set.Name] = true })

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var batch news.NewsBatch

	// Standard input is read only when the caller asked for it. A run driven by
	// -feeds alone must never block on a pipe nobody is writing to.
	if feedsPath == "" || explicit["input"] {
		input, err := readInput(inputPath, stdin)
		if err != nil {
			return fail(ExitUsage, err)
		}
		inputBytes = len(input)

		if err := json.Unmarshal(input, &batch); err != nil {
			return fail(ExitUsage, fmt.Errorf("input is not a news batch: %v", err))
		}
	}

	if feedsPath != "" {
		urls, err := readFeedList(feedsPath)
		if err != nil {
			return fail(ExitUsage, err)
		}
		if len(urls) == 0 {
			return fail(ExitUsage, fmt.Errorf("the feed list %s has no urls", feedsPath))
		}
		fetched = len(urls)

		// A feed that could not be read is reported by url and never hidden: a
		// summary that silently covers three of five feeds would overstate how
		// much of the story it saw.
		for _, result := range (feed.Client{}).FetchAll(runCtx, urls) {
			if result.Err != nil {
				fmt.Fprintf(stderr, "feed %s: %v\n", result.URL, result.Err)
				continue
			}
			batch.Articles = append(batch.Articles, result.Articles...)
		}
	}

	// An empty batch is a request that cannot be formed, so it fails before a
	// model is constructed or a key is needed.
	normalized := news.Normalize(batch)
	articles = len(normalized.Articles)
	if articles == 0 {
		return fail(ExitUsage, errors.New("input has no article text to summarize"))
	}

	engine, err := deps.NewSummarizer(runCtx, summarizer.Options{Provider: provider, Model: model})
	if err != nil {
		return fail(ExitFailure, err)
	}

	candidate, err := engine.Summarize(runCtx, normalized)
	if err != nil {
		if errors.Is(err, summarizer.ErrContractViolation) {
			return fail(ExitContractViolation, err)
		}
		return fail(ExitFailure, err)
	}

	// The contract is re-run here even though Summarize already checked, because
	// this is the trust boundary: any implementation, including a fake, has to
	// pass it to reach a reader.
	checked, err := summarizer.Check(candidate, normalized)
	if err != nil {
		return fail(ExitContractViolation, err)
	}

	fmt.Fprintln(stderr, Banner)
	if err := render(stdout, outputFormat, checked); err != nil {
		return fail(ExitFailure, err)
	}

	// An alert is the one artefact meant to leave the machine, so it is written
	// only when the human approves the digest of exactly this content. Without an
	// approval the run prints the digest, writes nothing, and says so with its own
	// exit code instead of pretending the delivery happened.
	if alertPath != "" {
		candidate, err := alert.New(checked)
		if err != nil {
			return fail(ExitUsage, err)
		}

		if strings.TrimSpace(approval) == "" {
			fmt.Fprintf(stderr, "alert approval required for %s\napprove exactly this summary with -approve %s\n",
				alertPath, candidate.Digest)
			return finish(ExitApprovalRequired, "alert awaiting approval")
		}

		if err := alert.Write(alertPath, candidate, approval); err != nil {
			if errors.Is(err, alert.ErrAlreadyDelivered) {
				return fail(ExitAlreadyDelivered, err)
			}
			return fail(ExitUsage, err)
		}
		fmt.Fprintf(stderr, "alert written to %s\n", alertPath)
	}

	return finish(ExitOK, "summary produced")
}

func readInput(path string, stdin io.Reader) ([]byte, error) {
	if path == "" || path == "-" {
		data, err := io.ReadAll(stdin)
		if err != nil {
			return nil, fmt.Errorf("read standard input: %v", err)
		}
		return data, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", path, err)
	}
	return data, nil
}

func readFeedList(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", path, err)
	}
	defer func() {
		_ = file.Close()
	}()

	urls, err := feed.ReadFeedURLs(file)
	if err != nil {
		return nil, fmt.Errorf("read %s: %v", path, err)
	}
	return urls, nil
}
