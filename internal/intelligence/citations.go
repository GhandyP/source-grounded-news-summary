package intelligence

import "github.com/GhandyP/source-grounded-news-summary/internal/news"

// Citations assembles the batch's own record of its sources, in batch order.
//
// It is derived in Go and never accepted from the model, so a citation, a title
// or a timestamp that no supplied article supports cannot reach the output. An
// undated article keeps PublishedUnknown, which is how a reader sees that part
// of the summary rests on an undated source.
func Citations(batch news.NewsBatch) []Citation {
	normalized := news.Normalize(batch)

	citations := make([]Citation, 0, len(normalized.Articles))
	for _, article := range normalized.Articles {
		citations = append(citations, Citation{
			SourceID:    article.SourceID,
			Title:       article.Title,
			PublishedAt: article.PublishedAt,
			DuplicateOf: append([]string(nil), article.DuplicateSourceIDs...),
		})
	}
	return citations
}
