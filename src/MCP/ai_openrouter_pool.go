package MCP

import (
	"cuento-backend/src/Entities"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

const openRouterBaseURL = "https://openrouter.ai/api/v1"

// minContextLength maps our size enum to the minimum context window a model
// must have to qualify. Used as a proxy for model capability.
var minContextForSize = map[Entities.AIModelSize]int{
	Entities.AIModelSizeTiny:   4096,
	Entities.AIModelSizeSmall:  16384,
	Entities.AIModelSizeMedium: 65536,
	Entities.AIModelSizeLarge:  131072,
}

// providerPrefixForProtocol maps our protocol enum to the OpenRouter model ID
// prefix used to filter by underlying provider.
var providerPrefixForProtocol = map[Entities.AIModelProtocolType]string{
	Entities.AIModelProtocolOpenAI:    "openai/",
	Entities.AIModelProtocolAnthropic: "anthropic/",
	Entities.AIModelProtocolGemini:    "google/",
}

type openRouterModel struct {
	ID           string `json:"id"`
	ContextLen   int    `json:"context_length"`
	Architecture struct {
		Modality string `json:"modality"`
	} `json:"architecture"`
	Pricing struct {
		Prompt string `json:"prompt"`
	} `json:"pricing"`
}

type OpenRouterModelPool struct {
	apiKey      string
	useFreeOnly bool
}

func NewOpenRouterModelPool(apiKey string, useFreeOnly bool) *OpenRouterModelPool {
	return &OpenRouterModelPool{apiKey: apiKey, useFreeOnly: useFreeOnly}
}

func (p *OpenRouterModelPool) ClientForMinSize(minSize Entities.AIModelSize, modelType Entities.AIModelType) (AIClient, error) {
	return p.selectModel(minSize, modelType, "")
}

func (p *OpenRouterModelPool) ClientForMinSizeAndProtocol(minSize Entities.AIModelSize, modelType Entities.AIModelType, protocol Entities.AIModelProtocolType) (AIClient, error) {
	return p.selectModel(minSize, modelType, providerPrefixForProtocol[protocol])
}

func (p *OpenRouterModelPool) selectModel(minSize Entities.AIModelSize, modelType Entities.AIModelType, providerPrefix string) (AIClient, error) {
	models, err := p.fetchModels()
	if err != nil {
		return nil, fmt.Errorf("openrouter: failed to fetch models: %w", err)
	}

	minCtx := minContextForSize[minSize]

	var candidates []openRouterModel
	for _, m := range models {
		if m.ContextLen < minCtx {
			continue
		}
		if !modalityMatchesType(m.Architecture.Modality, modelType) {
			continue
		}
		if providerPrefix != "" && !strings.HasPrefix(m.ID, providerPrefix) {
			continue
		}
		if p.useFreeOnly && parsePrice(m.Pricing.Prompt) > 0 {
			continue
		}
		candidates = append(candidates, m)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("openrouter: no model available for size=%d type=%d prefix=%q", minSize, modelType, providerPrefix)
	}

	// Pick the cheapest qualifying model (lowest prompt price = smallest sufficient model).
	sort.Slice(candidates, func(i, j int) bool {
		return parsePrice(candidates[i].Pricing.Prompt) < parsePrice(candidates[j].Pricing.Prompt)
	})

	chosen := candidates[0]
	return NewOpenAICompatClient(p.apiKey, chosen.ID, openRouterBaseURL)
}

func (p *OpenRouterModelPool) fetchModels() ([]openRouterModel, error) {
	req, err := http.NewRequest(http.MethodGet, openRouterBaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openrouter /models returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []openRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

// modalityMatchesType checks whether a model's OpenRouter modality string
// matches the requested AIModelType (text output vs. image output).
func modalityMatchesType(modality string, modelType Entities.AIModelType) bool {
	switch modelType {
	case Entities.AIModelTypeText:
		// Modality ends with "->text", e.g. "text->text" or "text+image->text"
		return strings.HasSuffix(modality, "->text")
	case Entities.AIModelTypeImage:
		return strings.HasSuffix(modality, "->image")
	}
	return false
}

func parsePrice(s string) float64 {
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
