# PLAN — `source-grounded-news-summary`

Status: **implemented slice = plan steps 1-4.** Step 5 (ingestion and the alert path) is deliberately not authorized; see "Out of scope".

## The problem

A structured-output schema guarantees shape, not truth. A news summarizer that answers with
`{"claims":[...]}` can still cite a source that does not exist, quote a sentence nobody wrote,
attach a claim to an undated article, hide that two outlets repeated the same wire copy, or flatten
two outlets that disagree into one confident sentence. This project adds the missing half: the
contract is enforced in pure Go, offline, before any of it reaches a reader.

## Shape

```text
NewsBatch{articles[{source_id,title,published_at,text}]}
      │
      ├─ news.Normalize        collapse whitespace, drop empty text, collapse duplicate articles
      │
      ├─ claim extractor       ADK agent #1, OutputSchema = claims + evidence + confidence
      │
      ├─ synthesis agent       ADK agent #2, OutputSchema = the full summary
      │
      └─ intelligence.Check    normalize → validate S5-S10 → citations assembled from the batch
                               │
                               └─ unverified intelligence; human review required
```

Both ADK agents share one in-memory bounded session. The order is decided in Go, not by model
transfer, so it is testable and cannot silently change.

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
- Duplicate detection is **exact after normalization** (whitespace collapse). Two wire stories with a
  changed byline are not duplicates.
- Conflict detection is model-reported and only structurally validated: the Go layer proves a
  conflict cites two real sources with two positions; it cannot prove they truly disagree.
- The confidence rule is a floor, not a calibration. It stops a claim over an undated article from
  being labelled `high`; it does not measure how likely a claim is to be true.
- No ingestion, no fetching, no alerting: this project only consumes a `NewsBatch` it is given.
- The live Gemini path requires `GOOGLE_API_KEY` and is out of the offline verification.

## Out of scope (not authorized)

RSS or article ingestion, canonicalization across outlets, paywall handling, real-time monitoring,
and any alert or notification path. The case plan gates them behind "after citation checks pass".

## Provenance

Extracted from cookbook case 18 (`~/Documents/18-news-and-intelligence-summary`, `README.md` and
`main.go` preserved in the first commit). The case's `main.go` imported the cookbook's
`internal/adkhelper`, which is unimportable from another module and unpublished upstream, so the
standalone copy owns its own wiring.
