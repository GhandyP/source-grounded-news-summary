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
			engine, err := summarizer.NewGemini(ctx, opts)
			if err != nil {
				return nil, err
			}
			return engine, nil
		}
	}

	var inputPath string
	var outputFormat string
	var model string
	var timeout time.Duration
	var verbose bool

	flags := flag.NewFlagSet("source-grounded-news-summary", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		fmt.Fprintln(stderr, "Summarize a batch of articles without letting the summary invent a source.")
		flags.PrintDefaults()
	}
	flags.StringVar(&inputPath, "input", "-", "news batch JSON path, or - for standard input")
	flags.StringVar(&outputFormat, "out", "json", "output format: json or text")
	flags.StringVar(&model, "model", "", "Gemini model name")
	flags.DurationVar(&timeout, "timeout", 60*time.Second, "summarization timeout; zero or negative disables the deadline")
	flags.BoolVar(&verbose, "verbose", false, "write diagnostics to standard error")

	articles := 0
	inputBytes := 0
	finish := func(code int, reason string) int {
		if verbose {
			fmt.Fprintf(stderr, "articles: %d\ninput bytes: %d\nexit reason: %s\n", articles, inputBytes, reason)
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

	input, err := readInput(inputPath, stdin)
	if err != nil {
		return fail(ExitUsage, err)
	}
	inputBytes = len(input)

	var batch news.NewsBatch
	if err := json.Unmarshal(input, &batch); err != nil {
		return fail(ExitUsage, fmt.Errorf("input is not a news batch: %v", err))
	}

	// An empty batch is a request that cannot be formed, so it fails before a
	// model is constructed or a key is needed.
	normalized := news.Normalize(batch)
	articles = len(normalized.Articles)
	if articles == 0 {
		return fail(ExitUsage, errors.New("input has no article text to summarize"))
	}

	runCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	engine, err := deps.NewSummarizer(runCtx, summarizer.Options{Model: model})
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
