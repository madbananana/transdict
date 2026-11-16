package translate

import (
	"strings"
	"testing"

	"transdict/internal/settings"
)

func TestModelInfoNamesTheModelAndItsChoices(t *testing.T) {
	p := settings.Provider{Kind: "openai", Model: "gpt-4o"}
	model, choices := ModelInfo(p)
	if model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", model)
	}
	if len(choices) != 3 || choices[0] != "gpt-4o-mini" {
		t.Errorf("choices = %v, want the shipped OpenAI list", choices)
	}
}

func TestModelInfoFallsBackToTheKindsShippedModel(t *testing.T) {
	model, choices := ModelInfo(settings.Provider{Kind: "claude"})
	if model != "claude-sonnet-4-5" {
		t.Errorf("model = %q, want the shipped Claude default", model)
	}
	if len(choices) == 0 || !strings.Contains(strings.Join(choices, " "), model) {
		t.Errorf("choices = %v, want the default among them", choices)
	}
}

func TestModelInfoReadsSupportedModels(t *testing.T) {
	p := settings.Provider{Kind: "openai", Model: "my-saved-model",
		Extra: map[string]string{settings.FieldSupportedModels: "a, b，c , a,"}}
	model, choices := ModelInfo(p)
	want := []string{"my-saved-model", "a", "b", "c"}
	if model != "my-saved-model" || strings.Join(choices, ",") != strings.Join(want, ",") {
		t.Errorf("model/choices = %q %v, want %q %v", model, choices, model, want)
	}
}

func TestModelInfoSkipsPlainTranslators(t *testing.T) {
	model, choices := ModelInfo(settings.Provider{Kind: "google"})
	if model != "" || len(choices) != 0 {
		t.Errorf("ModelInfo(google) = %q %v, want no chip", model, choices)
	}
}

func TestModelInfoWithoutDefaultsStillNamesTheSavedModel(t *testing.T) {
	p := settings.Provider{Kind: "custom", Model: "local-model"}
	model, choices := ModelInfo(p)
	if model != "local-model" || len(choices) != 1 || choices[0] != "local-model" {
		t.Errorf("ModelInfo(custom) = %q %v, want a one-item menu", model, choices)
	}
}
