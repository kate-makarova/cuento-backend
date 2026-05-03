package MCP

import (
	"context"
	"fmt"

	"google.golang.org/genai"
)

type GeminiClient struct {
	client *genai.Client
	model  string
}

func NewGeminiClient(apiKey string, model string) (*GeminiClient, error) {
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		return nil, err
	}
	return &GeminiClient{client: client, model: model}, nil
}

func (g *GeminiClient) Chat(ctx context.Context, history []ChatMessage) (string, error) {
	contents := make([]*genai.Content, len(history))
	for i, msg := range history {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		contents[i] = &genai.Content{
			Role:  role,
			Parts: []*genai.Part{{Text: msg.Content}},
		}
	}

	result, err := g.client.Models.GenerateContent(ctx, g.model, contents, nil)
	if err != nil {
		return "", err
	}

	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		return result.Candidates[0].Content.Parts[0].Text, nil
	}
	return "", fmt.Errorf("empty response from Gemini")
}
