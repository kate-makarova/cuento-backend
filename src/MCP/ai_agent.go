package MCP

import "context"

type AIClient interface {
	Chat(ctx context.Context, history []ChatMessage, systemInstruction string) (string, error)
}

type ChatMessage struct {
	Role    string // "user" or "assistant"
	Content string
}

type AIAgent struct {
	client AIClient
}

func NewAIAgent(client AIClient) *AIAgent {
	return &AIAgent{client: client}
}

func (a *AIAgent) Chat(ctx context.Context, history []ChatMessage, systemInstruction string) (string, error) {
	return a.client.Chat(ctx, history, systemInstruction)
}
