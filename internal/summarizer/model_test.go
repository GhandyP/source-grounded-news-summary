package summarizer

import (
	"context"
	"errors"
	"iter"
	"reflect"
	"strings"
	"testing"

	"google.golang.org/adk/v2/model"
)

// stubModel stands in for a provider so provider-neutrality can be tested
// without a key, a client or a network.
type stubModel struct{ name string }

func (m stubModel) Name() string { return m.name }

func (m stubModel) GenerateContent(context.Context, *model.LLMRequest, bool) iter.Seq2[*model.LLMResponse, error] {
	return func(func(*model.LLMResponse, error) bool) {}
}

func TestSupportedProvidersAreTheOnesADKShips(t *testing.T) {
	want := []string{ProviderApigee, ProviderGemini, ProviderOpenAI}

	if got := SupportedProviders(); !reflect.DeepEqual(got, want) {
		t.Fatalf("SupportedProviders() = %v, want %v", got, want)
	}
}

func TestResolveProvider(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		env      string
		want     string
	}{
		{name: "defaults to gemini", want: ProviderGemini},
		{name: "from the environment", env: "openai", want: ProviderOpenAI},
		{name: "the flag wins", explicit: "apigee", env: "openai", want: ProviderApigee},
		{name: "a blank flag falls through", explicit: "   ", env: "openai", want: ProviderOpenAI},
		{name: "case and spaces are forgiven", explicit: " OpenAI ", want: ProviderOpenAI},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ADK_MODEL_PROVIDER", test.env)

			got, err := ResolveProvider(test.explicit)
			if err != nil {
				t.Fatalf("ResolveProvider(%q) error = %v", test.explicit, err)
			}
			if got != test.want {
				t.Fatalf("ResolveProvider(%q) = %q, want %q", test.explicit, got, test.want)
			}
		})
	}
}

func TestResolveProviderListsWhatItSupports(t *testing.T) {
	t.Setenv("ADK_MODEL_PROVIDER", "")

	_, err := ResolveProvider("gpt")

	if err == nil {
		t.Fatal("ResolveProvider() = nil error, want an unknown provider refused")
	}
	for _, provider := range SupportedProviders() {
		if !strings.Contains(err.Error(), provider) {
			t.Fatalf("error = %q, want it to offer %q", err, provider)
		}
	}
}

func TestResolveModelName(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		explicit string
		env      string
		want     string
		wantErr  bool
	}{
		{name: "gemini has a default", provider: ProviderGemini, want: defaultModel},
		{name: "the flag wins", provider: ProviderGemini, explicit: "gemini-2.5-pro", env: "gemini-from-env", want: "gemini-2.5-pro"},
		{name: "the environment is next", provider: ProviderGemini, env: "gemini-from-env", want: "gemini-from-env"},
		{name: "openai needs a name", provider: ProviderOpenAI, wantErr: true},
		{name: "apigee needs a name", provider: ProviderApigee, wantErr: true},
		{name: "openai accepts an explicit name", provider: ProviderOpenAI, explicit: "gpt-5.4", want: "gpt-5.4"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("ADK_MODEL", test.env)

			got, err := resolveModelName(test.provider, test.explicit)
			if test.wantErr {
				if err == nil {
					t.Fatalf("resolveModelName(%q, %q) = %q and no error, want a refusal", test.provider, test.explicit, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveModelName(%q, %q) error = %v", test.provider, test.explicit, err)
			}
			if got != test.want {
				t.Fatalf("resolveModelName(%q, %q) = %q, want %q", test.provider, test.explicit, got, test.want)
			}
		})
	}
}

func TestNewBuildsTheSamePipelineForEveryProvider(t *testing.T) {
	for _, provider := range SupportedProviders() {
		t.Run(provider, func(t *testing.T) {
			var gotProvider, gotModel string

			engine, err := New(context.Background(), Options{
				Provider: provider,
				Model:    "some-model",
				NewModel: func(_ context.Context, chosen, name string) (model.LLM, error) {
					gotProvider, gotModel = chosen, name
					return stubModel{name: name}, nil
				},
			})
			if err != nil {
				t.Fatalf("New(provider=%s) error = %v", provider, err)
			}

			if engine == nil || engine.extractor == nil || engine.synthesizer == nil {
				t.Fatalf("New(provider=%s) = %#v, want both agents built", provider, engine)
			}
			if gotProvider != provider {
				t.Fatalf("the factory was asked for %q, want %q", gotProvider, provider)
			}
			if gotModel != "some-model" {
				t.Fatalf("the factory was asked for model %q, want the requested one", gotModel)
			}
		})
	}
}

func TestNewReportsTheProviderFailure(t *testing.T) {
	_, err := New(context.Background(), Options{
		Provider: ProviderGemini,
		NewModel: func(context.Context, string, string) (model.LLM, error) {
			return nil, errors.New("the provider said no")
		},
	})

	if err == nil {
		t.Fatal("New() = nil error, want the provider failure surfaced")
	}
	if !errors.Is(err, ErrModelUnavailable) {
		t.Fatalf("errors.Is(%v, ErrModelUnavailable) = false, want true", err)
	}
	if !strings.Contains(err.Error(), "the provider said no") {
		t.Fatalf("error = %q, want the underlying reason kept", err)
	}
}

func TestBuildModelNamesTheCredentialItNeeds(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		variable string
		clear    []string
	}{
		{name: "gemini", provider: ProviderGemini, variable: "GOOGLE_API_KEY", clear: []string{"GOOGLE_API_KEY", "GOOGLE_GENAI_USE_VERTEXAI"}},
		{name: "openai", provider: ProviderOpenAI, variable: "OPENAI_API_KEY", clear: []string{"OPENAI_API_KEY"}},
		{name: "apigee", provider: ProviderApigee, variable: "APIGEE_PROXY_URL", clear: []string{"APIGEE_PROXY_URL"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, variable := range test.clear {
				t.Setenv(variable, "")
			}

			_, err := BuildModel(context.Background(), test.provider, "some-model")

			if err == nil {
				t.Fatalf("BuildModel(%s) = nil error with no credentials, want a refusal", test.provider)
			}
			if !strings.Contains(err.Error(), test.variable) {
				t.Fatalf("error = %q, want it to name %s", err, test.variable)
			}
		})
	}
}

func TestBuildModelRefusesAnUnknownProvider(t *testing.T) {
	_, err := BuildModel(context.Background(), "some-vendor", "some-model")

	if err == nil {
		t.Fatal("BuildModel(some-vendor) = nil error, want a refusal")
	}
	if !strings.Contains(err.Error(), "some-vendor") {
		t.Fatalf("error = %q, want the rejected provider named", err)
	}
}
