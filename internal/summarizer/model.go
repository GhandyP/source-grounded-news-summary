package summarizer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/v2/model"
	"google.golang.org/adk/v2/model/apigee"
	"google.golang.org/adk/v2/model/gemini"
	"google.golang.org/adk/v2/model/openaimodel"
	"google.golang.org/genai"
)

// The providers ADK v2.2.0 ships, and the only ones this project claims to build.
const (
	ProviderApigee = "apigee"
	ProviderGemini = "gemini"
	ProviderOpenAI = "openai"
)

// defaultModel applies when the provider is Gemini and nobody named a model.
// Other providers have no sensible default, so they are asked for one instead of
// being handed a Gemini model name that cannot work.
const defaultModel = "gemini-flash-latest"

// ModelFactory builds the model both agents share. It exists so a test can drive
// the whole pipeline with no key and no network, and so a caller can plug in any
// ADK model.LLM of their own without touching the pipeline.
type ModelFactory func(ctx context.Context, provider, modelName string) (model.LLM, error)

// SupportedProviders lists the providers this build can construct.
func SupportedProviders() []string {
	return []string{ProviderApigee, ProviderGemini, ProviderOpenAI}
}

// ResolveProvider chooses the provider: an explicit value, then
// ADK_MODEL_PROVIDER, then Gemini.
func ResolveProvider(explicit string) (string, error) {
	for _, candidate := range []string{explicit, os.Getenv("ADK_MODEL_PROVIDER")} {
		normalized := strings.ToLower(strings.TrimSpace(candidate))
		if normalized == "" {
			continue
		}
		for _, known := range SupportedProviders() {
			if normalized == known {
				return known, nil
			}
		}
		return "", fmt.Errorf("unknown provider %q: use one of %s",
			strings.TrimSpace(candidate), strings.Join(SupportedProviders(), ", "))
	}

	return ProviderGemini, nil
}

// resolveModelName chooses the model: an explicit value, then ADK_MODEL, then the
// Gemini default.
func resolveModelName(provider, explicit string) (string, error) {
	if name := strings.TrimSpace(explicit); name != "" {
		return name, nil
	}
	if name := strings.TrimSpace(os.Getenv("ADK_MODEL")); name != "" {
		return name, nil
	}
	if provider == ProviderGemini {
		return defaultModel, nil
	}

	return "", fmt.Errorf("provider %q needs a model name: pass -model or set ADK_MODEL", provider)
}

// BuildModel constructs the provider's model from that provider's own environment
// variables. A missing credential is reported by name here, rather than surfacing
// later as an opaque client failure.
func BuildModel(ctx context.Context, provider, modelName string) (model.LLM, error) {
	switch provider {
	case ProviderGemini:
		config := &genai.ClientConfig{}
		if apiKey := strings.TrimSpace(os.Getenv("GOOGLE_API_KEY")); apiKey != "" {
			config.APIKey = apiKey
		} else if useVertex() {
			config.Backend = genai.BackendVertexAI
		} else {
			return nil, errors.New("GOOGLE_API_KEY is not set (set GOOGLE_GENAI_USE_VERTEXAI=true to use Vertex AI credentials instead)")
		}
		return gemini.NewModel(ctx, modelName, config)

	case ProviderOpenAI:
		apiKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
		if apiKey == "" {
			return nil, errors.New("OPENAI_API_KEY is not set (an OpenAI-compatible endpoint may also need OPENAI_BASE_URL)")
		}
		return openaimodel.NewModel(ctx, modelName, &openaimodel.ClientConfig{
			APIKey:  apiKey,
			BaseURL: strings.TrimSpace(os.Getenv("OPENAI_BASE_URL")),
		})

	case ProviderApigee:
		proxy := strings.TrimSpace(os.Getenv("APIGEE_PROXY_URL"))
		if proxy == "" {
			return nil, errors.New("APIGEE_PROXY_URL is not set")
		}
		client, err := apigee.NewModel(ctx, modelName, apigee.WithProxyURL(proxy))
		if err != nil {
			return nil, err
		}
		return client, nil

	default:
		return nil, fmt.Errorf("unknown provider %q: use one of %s", provider, strings.Join(SupportedProviders(), ", "))
	}
}

func useVertex() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("GOOGLE_GENAI_USE_VERTEXAI"))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}
