# PLAN — `source-grounded-news-summary`

Status: **plan steps 1-4 implemented, then step 5 (feed ingestion and a reviewed alert path)
implemented on explicit user authorization**, plus a provider-neutral model seam. The promise changed
with step 5, and this document says so.

## The problem

A structured-output schema guarantees shape, not truth. A news summarizer that answers with
`{"claims":[...]}` can still cite a source that does not exist, quote a sentence nobody wrote,
attach a claim to an undated article, hide that two outlets repeated the same wire copy, or flatten
two outlets that disagree into one confident sentence. This project adds the missing half: the
contract is enforced in pure Go, offline, before any of it reaches a reader.

## Shape

```text
feeds.txt ──> feed.Client  one GET per url, 30s timeout, 5 MiB cap, user agent
                  │
                  └─> feed.Parse  RSS <item> and Atom <entry> ──┐
                                                                ├──> NewsBatch
batch JSON via -input ──────────────────────────────────────────┘
      │
      ├─ news.Normalize        collapse whitespace, drop empty text, collapse duplicate articles
      │
      ├─ claim extractor       agent #1, OutputSchema = claims + evidence + confidence
      │
      ├─ synthesis agent       agent #2, OutputSchema = the full summary
      │
      └─ intelligence.Check    normalize → validate S5-S10 → citations assembled from the batch
                               │
                               ├─> stdout: unverified intelligence; human review required
                               │
                               └─> alert gate: sha256 of the summary → human approval → atomic
                                   write plus a delivery ledger that refuses a repeat
```

Both agents share one in-memory bounded session. The order is decided in Go, not by model transfer, so
it is testable and cannot silently change. The model itself is a seam: `BuildModel` constructs the
provider's `model.LLM` from that provider's own environment variables, and a caller may inject any
model instead.

## Rules the Go validator enforces

| Rule | What it requires | Where it looks |
| --- | --- | --- |
| S6 source grounding | Every `source_id` in a claim, theme, conflict, gap or evidence exists in the batch, and a claim cites at least one. | The normalized batch only. |
| S7 quote grounding | Every quote is non-empty and a boundary-aware substring of the cited article's normalized text; every occurrence is checked. | The cited article's own normalized text. |
| S8 uncertainty | `confidence` is one of `high`/`medium`/`low`, and a claim citing an article with `published_at: unknown` cannot be `high`. | The claim plus its cited articles. |
| S9 conflicts | A conflict cites at least two distinct sources and carries at least two distinct non-empty positions. | The conflict itself plus the batch. |
| S10 repeats | The same claim text supported by several articles is one claim carrying every supporting `source_id`. | The claim list. |

`citations[]` is assembled in Go from the batch after validation; it is never accepted from the
model, so a fabricated citation or timestamp cannot be published.

## Known limits, stated rather than hidden

- Grounding is **textual, not semantic**. A quote that genuinely appears in the article is accepted
  even when the model attached it to the wrong claim. Entailment checking is not implemented.
- No ingestion beyond the feed body: the linked page is never fetched, so a paywall snippet is
  summarized as the snippet it is. Markup is stripped and entities decoded.
- Duplicate detection is **exact after normalization** (whitespace collapse). Two wire stories with a
  changed byline are not duplicates. A feed entry identical to a supplied article is collapsed into one
  story with two source ids, which is what keeps a syndicated wire copy from reading as two sources.
- An oversized feed is rejected, not truncated: a summary of half an article is worse than no summary.
- The alert digest binds to the exact JSON of one summary, so any reformatting produces a new digest.
  That is deliberate: an approval has to be about the bytes being written, and a repeat of the same
  digest is refused rather than delivered twice.
- The approval is a file digest, not a signature: it proves a human approved this content on this
  machine, not who that human was.
- Conflict detection is model-reported and only structurally validated: the Go layer proves a
  conflict cites two real sources with two positions; it cannot prove they truly disagree.
- The confidence rule is a floor, not a calibration. It stops a claim over an undated article from
  being labelled `high`; it does not measure how likely a claim is to be true.
- No ingestion, no fetching, no alerting: this project only consumes a `NewsBatch` it is given.
- The live Gemini path requires `GOOGLE_API_KEY` and is out of the offline verification.

## Out of scope

- No scheduler, daemon or watch mode: the command runs once, over the feeds a caller lists.
- No destination other than a file the caller names. There is no mail, webhook, message queue or
  publishing path, and no HTTP verb other than GET anywhere in the module.
- No fetching of the linked article, no paywall or cookie handling, and no canonicalization of the same
  story across different hosts.
- No risk scoring, prioritisation or routing of alerts: approval is a human reading a digest.

## Provenance

Extracted from cookbook case 18 (`~/Documents/18-news-and-intelligence-summary`, `README.md` and
`main.go` preserved in the first commit). The case's `main.go` imported the cookbook's
`internal/adkhelper`, which is unimportable from another module and unpublished upstream, so the
standalone copy owns its own wiring.
