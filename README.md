# 18 — News and intelligence summaries

## Goal

Summarize supplied news text while preserving source labels, separating repeated claims, and showing uncertainty.

## Run the current prototype

From this case directory:

```bash
export GOOGLE_API_KEY="your-key"
go run .
```

`internal/adkhelper.Run` uses ADK Go v2.2.0, Gemini, and `gemini-flash-latest`; set `ADK_MODEL` to override the model.

Sample prompt:

```text
Summarize these two article excerpts and call out their overlapping claim.
```

## What exists now

- `main.go (exists)` gives article excerpts and instructions to one focused Gemini ADK agent.
- The response is untyped text; source IDs, claim boundaries, publication times, and confidence are not preserved reliably.
- This is intentionally text-only: it does not ingest RSS/news feeds, fetch articles, deduplicate sources, or publish alerts.

## Missing elements

- **Contract:** Define `NewsBatch{articles[{sourceID,title,publishedAt,text}]}` -> `IntelligenceSummary{claims[], themes[], conflicts[], gaps[], citations[]}`.
- **ADK workflow:** Add a claim-extraction sub-agent and synthesis agent with schemas and a batch session; retrieval remains read-only.
- **Domain boundary:** Add RSS/article ingestion, canonicalization, duplicate detection, source metadata, and citation adapters.
- **Fixtures and validation:** Cover duplicate articles, conflicting claims, missing dates, paywall snippets, and empty feeds with source-link tests.
- **Safety:** Label summaries as unverified intelligence and require human review before external alerts or business decisions.

## Target structure

```text
18-news-and-intelligence-summary/
├── main.go (exists)
├── README.md (exists)
├── workflow.go (next)
├── schema.go (next)
├── adapters/news_feed.go (next)
├── testdata/articles.json (next)
└── main_test.go (next)
```

## Implementation order

1. Define article, claim, citation, conflict, and gap schemas.
2. Normalize source identity and deduplicate an in-memory batch.
3. Add extraction/synthesis agents and a bounded session.
4. Test repeated, conflicting, stale, and incomplete sources.
5. Add feed ingestion and a reviewed alert path after citation checks pass.

## Done when

- [ ] Every factual claim links to one or more source IDs and timestamps.
- [ ] Conflicts, missing sources, and duplicate articles are visible.
- [ ] Fixtures cover feed and article edge cases with repeatable results.
- [ ] Alerts and downstream decisions require human review.

## Next improvement

Create a source-labeled claim schema and deduplicate two supplied excerpts before adding RSS ingestion.
