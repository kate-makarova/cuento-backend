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

func (g *GeminiClient) Chat(ctx context.Context, history []ChatMessage, systemInstruction string) (string, error) {
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

	cfg := &genai.GenerateContentConfig{
		Tools: buildGeminiTools(),
	}
	if systemInstruction != "" {
		cfg.SystemInstruction = &genai.Content{
			Parts: []*genai.Part{{Text: systemInstruction}},
		}
	}

	// Tool-calling loop: keep going until Gemini returns a text response
	const maxRounds = 10
	for round := 0; round < maxRounds; round++ {
		result, err := g.client.Models.GenerateContent(ctx, g.model, contents, cfg)
		if err != nil {
			return "", err
		}

		if len(result.Candidates) == 0 {
			return "", fmt.Errorf("empty response from Gemini (no candidates)")
		}

		candidate := result.Candidates[0]
		if candidate.Content == nil {
			return "", fmt.Errorf("empty response from Gemini (finish_reason=%v)", candidate.FinishReason)
		}

		// Check if this turn has any function calls
		var functionCalls []*genai.Part
		var textParts []*genai.Part
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil {
				functionCalls = append(functionCalls, part)
			} else if part.Text != "" {
				textParts = append(textParts, part)
			}
		}

		// No function calls — return the text response
		if len(functionCalls) == 0 {
			if len(textParts) > 0 {
				return textParts[0].Text, nil
			}
			return "", fmt.Errorf("empty response from Gemini (finish_reason=%v)", candidate.FinishReason)
		}

		// Append model's function call turn to contents
		contents = append(contents, candidate.Content)

		// Execute each function call and collect responses
		responseParts := make([]*genai.Part, 0, len(functionCalls))
		for _, part := range functionCalls {
			fc := part.FunctionCall
			executor, ok := findExecutor(fc.Name)
			if !ok {
				responseParts = append(responseParts, genai.NewPartFromFunctionResponse(fc.Name, map[string]any{
					"error": fmt.Sprintf("unknown tool: %s", fc.Name),
				}))
				continue
			}

			output, err := executor(ctx, fc.Args)
			if err != nil {
				responseParts = append(responseParts, genai.NewPartFromFunctionResponse(fc.Name, map[string]any{
					"error": err.Error(),
				}))
			} else {
				responseParts = append(responseParts, genai.NewPartFromFunctionResponse(fc.Name, map[string]any{
					"result": output,
				}))
			}
		}

		// Append tool results as a user turn
		contents = append(contents, &genai.Content{
			Role:  "user",
			Parts: responseParts,
		})
	}

	return "", fmt.Errorf("exceeded maximum tool call rounds")
}
