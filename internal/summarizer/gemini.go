package summarizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/GhandyP/source-grounded-news-summary/internal/intelligence"
	"github.com/GhandyP/source-grounded-news-summary/internal/news"
	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/agent/llmagent"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/runner"
	"google.golang.org/adk/v2/session"
	"google.golang.org/genai"
)

const (
	// defaultModel is the model the project is verified against.
	defaultModel = "gemini-flash-latest"
	// appName names the in-memory session a batch is summarized in.
	appName = "source-grounded-news-summary"
	// userID is fixed because a batch is one bounded run, not a conversation.
	userID = "source-grounded-news-summary"
)

const extractorInstruction = `Extract the claims the supplied articles support.
- Use only the articles in the batch. A claim must be something an article actually states.
- List each claim once. When several articles state the same claim, list it once with every source_id that states it and one evidence entry per source.
- Set evidence[].quote to the exact span of that article's text that supports the claim, copied character for character. Never paraphrase, shorten or repair a quotation.
- Set evidence[].source_id to the article the quotation comes from; it must also appear in the claim's source_ids.
- Set confidence to "high" only when a dated article states the claim plainly. Use "medium" when the article is undated or hedges, and "low" when the claim is implied rather than stated.
- Never invent a source id, a quotation, a date or a number.`

const synthesizerInstruction = `Write the intelligence summary for the supplied articles, using the claims extracted in the first stage.
- The extracted claims come from these same articles. Keep every claim the quotations support, and never add a claim no quotation supports.
- themes: subjects the batch covers together. Each theme needs at least one source_id from the batch.
- conflicts: statements where two or more articles disagree. Each conflict needs at least two distinct source_ids and at least two distinct positions.
- gaps: material things the batch does not establish, such as a missing date, a missing figure, or a claim only one source carries. A gap may carry no source_id at all.
- Every source_id must be one of the batch's source ids. Never invent one, and never invent a title or a date.
- Do not produce a citations list. The caller derives it from the batch itself.`

// Gemini summarizes a batch with two schema-constrained ADK agents that share one
// bounded session: an extractor that quotes, then a synthesizer that groups.
type Gemini struct {
	extractor   *runner.Runner
	synthesizer *runner.Runner
}

var (
	_ Summarizer = (*Gemini)(nil)

	sessionSequence uint64
)

// NewGemini creates the two-stage workflow. The order is decided in Go, not by
// model transfer, so the pipeline cannot silently change shape at runtime.
func NewGemini(ctx context.Context, opts Options) (*Gemini, error) {
	apiKey := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY"))
	if apiKey == "" {
		return nil, fmt.Errorf("%w: GOOGLE_API_KEY is not set", ErrModelUnavailable)
	}

	model, err := gemini.NewModel(ctx, resolveModel(opts.Model), &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, fmt.Errorf("%w: create Gemini model: %v", ErrModelUnavailable, err)
	}

	extractor, err := llmagent.New(llmagent.Config{
		Name:         "claim_extractor",
		Description:  "Extracts the claims a batch of articles supports, each with a verbatim quotation.",
		Model:        model,
		Instruction:  extractorInstruction,
		OutputSchema: ExtractionSchema(),
		OutputKey:    "extracted_claims",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create extraction agent: %v", ErrModelUnavailable, err)
	}

	synthesizer, err := llmagent.New(llmagent.Config{
		Name:         "intelligence_synthesizer",
		Description:  "Synthesizes themes, conflicts and gaps from claims that were already extracted and quoted.",
		Model:        model,
		Instruction:  synthesizerInstruction,
		OutputSchema: SummarySchema(),
		OutputKey:    "intelligence_summary",
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create synthesis agent: %v", ErrModelUnavailable, err)
	}

	// One service, two runners: both stages of one batch live in the same
	// bounded session.
	sessions := session.InMemoryService()

	extractorRunner, err := runner.New(runner.Config{
		AppName:           appName,
		Agent:             extractor,
		SessionService:    sessions,
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create extraction runner: %v", ErrModelUnavailable, err)
	}

	synthesizerRunner, err := runner.New(runner.Config{
		AppName:           appName,
		Agent:             synthesizer,
		SessionService:    sessions,
		AutoCreateSession: true,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: create synthesis runner: %v", ErrModelUnavailable, err)
	}

	return &Gemini{extractor: extractorRunner, synthesizer: synthesizerRunner}, nil
}

// Summarize runs both stages and submits the result to the contract. The batch
// handed to the model is the normalized batch, which is also the text the
// contract checks quotations against.
func (g *Gemini) Summarize(ctx context.Context, batch news.NewsBatch) (intelligence.IntelligenceSummary, error) {
	if g == nil || g.extractor == nil || g.synthesizer == nil {
		return intelligence.IntelligenceSummary{}, fmt.Errorf("%w: the summarizer is not initialized", ErrModelUnavailable)
	}

	normalized := news.Normalize(batch)
	if len(normalized.Articles) == 0 {
		return intelligence.IntelligenceSummary{}, fmt.Errorf("cannot summarize a batch with no usable article text")
	}

	batchText, err := batchPrompt(normalized)
	if err != nil {
		return intelligence.IntelligenceSummary{}, err
	}

	sessionID := newSessionID()

	var extraction Extraction
	if err := runStage(ctx, g.extractor, sessionID, batchText, &extraction); err != nil {
		return intelligence.IntelligenceSummary{}, err
	}

	prompt, err := synthesisPrompt(batchText, extraction)
	if err != nil {
		return intelligence.IntelligenceSummary{}, err
	}

	var summary intelligence.IntelligenceSummary
	if err := runStage(ctx, g.synthesizer, sessionID, prompt, &summary); err != nil {
		return intelligence.IntelligenceSummary{}, err
	}

	return Check(mergeStages(extraction, summary), normalized)
}

// runStage runs one agent once and decodes the last structured output it emitted.
func runStage(ctx context.Context, r *runner.Runner, sessionID, prompt string, target any) error {
	msg := &genai.Content{
		Role:  genai.RoleUser,
		Parts: []*genai.Part{{Text: prompt}},
	}

	var output any
	for event, err := range r.Run(ctx, userID, sessionID, msg, agent.RunConfig{}) {
		if err != nil {
			return fmt.Errorf("%w: the ADK run failed: %v", ErrModelUnavailable, err)
		}
		if event != nil && event.Output != nil {
			output = event.Output
		}
	}

	return decodeInto(output, target)
}

// mergeStages combines both agents' output. The synthesizer owns the summary,
// but claims that were extracted and quoted once must not vanish in the second
// pass, so an empty claims list falls back to the extraction.
func mergeStages(extraction Extraction, summary intelligence.IntelligenceSummary) intelligence.IntelligenceSummary {
	if len(summary.Claims) == 0 {
		summary.Claims = extraction.Claims
	}
	return summary
}

// batchPrompt serializes the batch the way the model will read it: normalized and
// whitespace-collapsed, so a quotation the model returns can match the text the
// contract checks it against.
func batchPrompt(batch news.NewsBatch) (string, error) {
	encoded, err := encodeJSON(news.Normalize(batch))
	if err != nil {
		return "", fmt.Errorf("%w: encode the batch: %v", ErrModelOutput, err)
	}
	return "News batch as JSON:\n" + encoded, nil
}

// synthesisPrompt carries the stage one claims into stage two, so the synthesizer
// groups evidence that already has a quotation instead of recalling the batch.
func synthesisPrompt(batchText string, extraction Extraction) (string, error) {
	encoded, err := encodeJSON(extraction)
	if err != nil {
		return "", fmt.Errorf("%w: encode the extracted claims: %v", ErrModelOutput, err)
	}
	return batchText + "\n\nStage 1 extracted these claims as JSON:\n" + encoded, nil
}

// decodeInto decodes a model output into a contract type. ADK may hand back either
// a decoded map or a value already built from the schema, so the seam goes
// through JSON rather than guessing.
func decodeInto(output any, target any) error {
	if output == nil {
		return fmt.Errorf("%w: the model emitted no structured output", ErrModelOutput)
	}

	encoded, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("%w: encode the model output: %v", ErrModelOutput, err)
	}
	if err := json.Unmarshal(encoded, target); err != nil {
		return fmt.Errorf("%w: decode the model output: %v", ErrModelOutput, err)
	}
	return nil
}

// resolveModel chooses a non-blank explicit model, then ADK_MODEL, then the
// documented default.
func resolveModel(override string) string {
	if model := strings.TrimSpace(override); model != "" {
		return model
	}
	if model := strings.TrimSpace(os.Getenv("ADK_MODEL")); model != "" {
		return model
	}
	return defaultModel
}

// newSessionID gives every batch its own session, so two runs never share state.
func newSessionID() string {
	return fmt.Sprintf("batch-%d", atomic.AddUint64(&sessionSequence, 1))
}

func encodeJSON(value any) (string, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimRight(buffer.String(), "\n"), nil
}
