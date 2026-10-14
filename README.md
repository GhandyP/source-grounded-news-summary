# source-grounded-news-summary

A Go CLI that turns a supplied batch of news articles into claims, themes, conflicts and gaps — and
refuses a summary that cites a source nobody supplied, quotes a sentence nobody wrote, or hides that
a claim rests on an undated article.

[![CI](https://github.com/GhandyP/source-grounded-news-summary/actions/workflows/ci.yml/badge.svg)](https://github.com/GhandyP/source-grounded-news-summary/actions/workflows/ci.yml)

## The idea

A structured-output schema guarantees shape, not truth. A model asked for `{"claims": [...]}` will
happily return a claim that cites `source_id: "s9"` when the batch has three articles, quote a
sentence nobody wrote, attach a statement to an undated article and publish it as `high` confidence,
and turn one wire story printed by three outlets into three independent confirmations.

This project adds the missing half: the contract is enforced in pure Go, offline, before a reader ever
sees the summary. Two ADK agents do the reading — a claim extractor that must quote what it cited, and
a synthesizer that groups those claims into themes, conflicts and gaps — and everything they return
passes through one policy function. Nothing in the module can send, publish or alert: the command is a
dry run that labels its own output as unverified intelligence.

## Quickstart

The module uses the Go version declared in `go.mod`.

```bash
go build ./...
go test ./...

export GOOGLE_API_KEY="your-key"
go run . -input fixtures/repeated-claim.json
cat fixtures/repeated-claim.json | go run . -input -
go run . -input fixtures/repeated-claim.json -out text
```

The first two commands need no key and no network. The three summarizing commands construct the live
Gemini summarizer (`GOOGLE_API_KEY`), and the first build downloads the ADK and Gemini modules if they
are not already in the module cache.

### Verification recorded for this checkout

Run in the module directory with `GOPROXY=off` and no `GOOGLE_API_KEY`, so nothing below touched the
network:

```bash
GOPROXY=off go build ./...
GOPROXY=off go vet ./...
GOPROXY=off go vet -tags=integration ./...
GOPROXY=off go test -race -count=1 ./...
gofmt -l .
```

All five exit 0, `gofmt -l .` prints nothing, and the test run prints:

```text
ok  	github.com/GhandyP/source-grounded-news-summary	1.177s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/cli	1.147s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/intelligence	1.088s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/news	1.022s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/summarizer	1.043s
```

Exit codes were checked through the compiled binary rather than `go run`, which reports a child's
non-zero status as 1:

| Command | Exit | Output |
| --- | --- | --- |
| `./source-grounded-news-summary -input fixtures/empty.json` | 2 | `input has no article text to summarize` |
| `./source-grounded-news-summary -nope` | 2 | `flag provided but not defined: -nope` |
| `./source-grounded-news-summary extra` | 2 | `unexpected argument "extra": the batch is read from -input` |
| `./source-grounded-news-summary -input fixtures/repeated-claim.json -out yaml` | 2 | `unknown output format "yaml": use json or text` |
| `./source-grounded-news-summary -input fixtures/absent.json` | 2 | `read fixtures/absent.json: open fixtures/absent.json: no such file or directory` |
| `env -u GOOGLE_API_KEY ./source-grounded-news-summary -input fixtures/repeated-claim.json` | 1 | `the model is unavailable: GOOGLE_API_KEY is not set` |

**Not verified:** no live model run happened, because no `GOOGLE_API_KEY` was available. Exit 0 through
the real binary and the `//go:build integration` test were therefore never executed, and this README
claims nothing about the summary a live model would produce. CI runs exactly the offline commands above
on every push.

## What the Go layer refuses to publish

| Rule | What it requires | Where it looks |
| --- | --- | --- |
| Source grounding | Every `source_id` in a claim, theme, conflict, gap or evidence entry exists in the batch, and a claim cites at least one. A duplicated article's ids resolve to the canonical article. | The normalized batch only. |
| Quote grounding | Every quotation is non-empty and a boundary-aware substring of the cited article's own normalized text; every occurrence is checked. | The cited article's text only. |
| Uncertainty | `confidence` is `high`, `medium` or `low`, and a claim citing an undated article cannot be `high`. | The claim plus its cited articles. |
| Conflicts | A conflict cites at least two distinct sources and carries at least two distinct positions. | The conflict itself plus the batch. |
| Repeats | A claim several articles state is one claim carrying every supporting source id, with the weakest confidence of the merged claims. | The claim list. |
| Citations | `citations[]` is built in Go from the batch and never taken from the model, so a fabricated citation or timestamp cannot be published. | The batch. |

Boundary-aware means the match must be delimited on both sides by the string edge or by a character
that is neither a letter nor a digit: `Ann` is not satisfied by `annual`, and `2026-10-03` is not
satisfied by `X2026-10-03Z`. Duplicate detection is exact after whitespace collapse, so one wire story
printed three times is one article with three source ids, not three confirmations.

## Contracts

Input — a `NewsBatch`:

```json
{"articles":[{"source_id":"wire","title":"Council approves budget","published_at":"2026-10-03","text":"…"}]}
```

`published_at` is a real `YYYY-MM-DD` calendar date (`2026-02-31` is rejected) or the literal
`unknown`.

Output — an `IntelligenceSummary`:

```json
{
  "claims": [{"text": "…", "source_ids": ["wire"], "evidence": [{"source_id": "wire", "quote": "…"}], "confidence": "medium"}],
  "themes": [{"text": "…", "source_ids": ["wire", "daily"]}],
  "conflicts": [{"subject": "…", "positions": ["…", "…"], "source_ids": ["ministry", "local"]}],
  "gaps": [{"text": "…", "source_ids": ["wire"]}],
  "citations": [{"source_id": "wire", "title": "…", "published_at": "…", "duplicate_of": ["syndicated"]}]
}
```

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | A contract-checked summary was produced. |
| 1 | The model, the network or the output stream failed. |
| 2 | The request could not be formed: unknown flag, unknown `-out`, missing file, invalid JSON, or a batch with no usable article text. |
| 3 | The summary violated the contract. Nothing is printed to standard output. |

Standard error always carries `unverified intelligence - human review required before any external
action` when a summary is produced, so a machine consuming JSON on standard output still sees the
label.

## What this project does not do

- No ingestion: it reads a batch it is given. There is no RSS reader, no fetcher, no scheduler.
- No alerting: there is no notifier, sender or publisher, and a test fails if the module ever imports
  an HTTP client, an SMTP client, an RPC client or a process launcher.
- Grounding is textual, not semantic. A quotation that genuinely appears in the article is accepted
  even when the model attached it to the wrong claim; entailment is not checked.
- Duplicate detection is exact after normalization. Two wire stories with a changed byline are not
  duplicates.
- Conflict detection is model-reported and only structurally validated: the Go layer proves a conflict
  cites two real sources with two distinct positions, not that they truly disagree.
- The confidence rule is a floor, not a calibration.

## Provenance

Extracted from cookbook case 18 (`~/Documents/18-news-and-intelligence-summary`). The case's `main.go`
imported the cookbook's `internal/adkhelper`, which Go forbids importing from another module and which
is unpublished upstream, so this standalone copy owns its own wiring. Both original files are preserved
in the first commit of this repository.
