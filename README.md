# source-grounded-news-summary

A Go CLI that turns news articles — supplied as a batch, read from RSS and Atom feeds, or both — into
claims, themes, conflicts and gaps, and refuses a summary that cites a source nobody supplied, quotes
a sentence nobody wrote, or hides that a claim rests on an undated article.

[![CI](https://github.com/GhandyP/source-grounded-news-summary/actions/workflows/ci.yml/badge.svg)](https://github.com/GhandyP/source-grounded-news-summary/actions/workflows/ci.yml)

## The idea

A structured-output schema guarantees shape, not truth. A model asked for `{"claims": [...]}` will
happily return a claim that cites `source_id: "s9"` when the batch has three articles, quote a
sentence nobody wrote, attach a statement to an undated article and publish it as `high` confidence,
and turn one wire story printed by three outlets into three independent confirmations.

This project adds the missing half: the contract is enforced in pure Go, offline, before a reader ever
sees the summary. Two agents do the reading — a claim extractor that must quote what it cited, and a
synthesizer that groups those claims into themes, conflicts and gaps — and everything they return
passes through one policy function. Nothing is sent anywhere: the command prints a labelled summary,
and the only file it ever writes is an alert that a human approved by digest. The model is a seam as
well: Gemini by default, any other provider ADK ships, or your own `model.LLM`.

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

The first two commands need no key and no network. The summarizing commands construct a model, so they
need that provider's credentials:

| Provider | `-provider` | Credentials | Default model |
| --- | --- | --- | --- |
| Gemini | `gemini` (default) | `GOOGLE_API_KEY`, or `GOOGLE_GENAI_USE_VERTEXAI=true` for Vertex AI credentials | `gemini-flash-latest` |
| OpenAI-compatible | `openai` | `OPENAI_API_KEY`, optionally `OPENAI_BASE_URL` | none: pass `-model` |
| Apigee | `apigee` | `APIGEE_PROXY_URL` | none: pass `-model` |

`ADK_MODEL_PROVIDER` and `ADK_MODEL` set the same two things by environment, and `-provider`/`-model`
win over them.

### Reading feeds

```bash
printf 'https://example.com/feed.xml\n' > feeds.txt
go run . -feeds feeds.txt
go run . -input fixtures/repeated-claim.json -feeds feeds.txt   # merge both
```

The list is one URL per line; blank lines and `#` comments are ignored and duplicates collapse. A
ready-made example lives in `fixtures/feeds.txt`, and a test keeps that file's format honest. Each
feed is fetched with one GET, a 30 second timeout, a 5 MiB cap, and a user agent naming the project. A
feed that cannot be read is reported by URL on standard error and never silently skipped; if no feed
yields an article the run fails before a model exists.

### Delivering an alert

```bash
go run . -feeds feeds.txt -alert out/alert.json          # prints a digest, writes nothing, exits 4
go run . -feeds feeds.txt -alert out/alert.json -approve <digest>
```

The alert file is the one artefact meant to leave the machine, so it is gated: the first command prints
the digest of exactly what would be written and exits 4; the second writes it atomically and records
the digest in `out/alert.json.approved`, so approving the same alert twice is refused (exit 5) instead
of sending it twice. The destination file is compared as well, so deleting the ledger does not reopen
the gate, and `Write` re-checks that the summary carries citations rather than trusting whatever built
the candidate: an alert can only be built from a summary that already passed the contract.

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
ok  	github.com/GhandyP/source-grounded-news-summary	2.173s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/alert	1.094s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/cli	1.246s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/feed	1.348s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/intelligence	1.034s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/news	1.017s
ok  	github.com/GhandyP/source-grounded-news-summary/internal/summarizer	1.074s
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

**Not verified:** no live model run happened, because no provider credential was available in the environment
this was recorded in. Exit 0 through the real binary and the `//go:build integration` test were therefore
never executed, and this README claims nothing about the summary a live model would produce. Exit 4 and
exit 5 are covered by tests that drive the command with a fake summarizer; they were **not** exercised as
processes either, because reaching them needs a checked summary and that needs a model. CI runs exactly
the offline commands above on every push.

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

## Flags

| Flag | Default | Meaning |
| --- | --- | --- |
| `-input` | `-` | News batch JSON path, or `-` for standard input. Read only when given explicitly, so a `-feeds` run never waits on a pipe nobody is writing to. |
| `-feeds` | none | Path to a file listing feed URLs, one per line. |
| `-out` | `json` | `json` or `text`. |
| `-provider` | `gemini` | `apigee`, `gemini` or `openai`. Beats `ADK_MODEL_PROVIDER`. |
| `-model` | provider default | Model name. Beats `ADK_MODEL`. |
| `-alert` | none | Write the checked summary to this path once its digest is approved. |
| `-approve` | none | The digest a previous run printed; approves exactly that content. |
| `-timeout` | `60s` | Bounds feed fetching and summarization together; zero or negative disables the deadline. |
| `-verbose` | `false` | Writes article count, feed count and the exit reason to standard error. |

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | A contract-checked summary was produced, and the alert was written if one was approved. |
| 1 | The model, the network or the output stream failed. |
| 2 | The request could not be formed: unknown flag, unknown `-out`, missing file, invalid JSON, a batch with no usable article text, an unusable feed list, an approval that does not match, or `-approve` without `-alert`. |
| 3 | The summary violated the contract. Nothing is printed to standard output. |
| 4 | The alert candidate is ready and unapproved: the digest was printed and nothing was written. |
| 5 | That exact alert was delivered before. |

Standard error always carries `unverified intelligence - human review required before any external
action` when a summary is produced, so a machine consuming JSON on standard output still sees the
label.

## What this project does not do

- No scheduler and no daemon: you run it, it reads the feeds you listed, and it exits.
- No destination beyond a file you name, and no unapproved alert. It never sends mail, never posts,
  never calls a webhook. The fetch surface is GET-only and lives in one package; a test fails the suite
  if any other linked package imports an HTTP, SMTP, RPC or process-launching client.
- No repeat delivery from this machine: the same digest is refused twice, both from the delivery ledger
  and from the digest already written at the destination.
- No article extraction: the feed's own body is used, with markup stripped. It does not fetch the linked
  page or defeat a paywall, so a snippet is summarized as the snippet it is.
- Grounding is textual, not semantic. A quotation that genuinely appears in the article is accepted
  even when the model attached it to the wrong claim; entailment is not checked.
- Duplicate detection is exact after normalization. Two wire stories with a changed byline are not
  duplicates.
- Conflict detection is model-reported and only structurally validated: the Go layer proves a conflict
  cites two real sources with two distinct positions, not that they truly disagree.
- The confidence rule is a floor, not a calibration.
- A feed larger than the cap is rejected rather than truncated, so an oversized feed produces no summary
  instead of a summary of half an article.
- The alert digest binds to the exact JSON of a summary: reformatting or re-running may produce a new
  digest, and that is the point, because the approval has to be about the content being written.
- Repeat detection is **local evidence**: the ledger beside the alert plus the digest already present at
  the destination. Copying a delivered alert to another path, or running on another machine, starts a
  fresh gate, because nothing on this machine can know about that other one. Deleting the destination as
  well as the ledger reopens it, which is why the two are checked together rather than either alone.

## Provenance

Extracted from cookbook case 18 (`~/Documents/18-news-and-intelligence-summary`). The case's `main.go`
imported the cookbook's `internal/adkhelper`, which Go forbids importing from another module and which
is unpublished upstream, so this standalone copy owns its own wiring. Both original files are preserved
in the first commit of this repository.
