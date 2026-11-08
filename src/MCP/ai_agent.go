package MCP

import "context"

// ChatSource holds a post/topic ID pair collected from tool call results.
type ChatSource struct {
	PostID    *int64 `json:"post_id,omitempty"`
	TopicID   *int64 `json:"topic_id,omitempty"`
	TopicName string `json:"topic_name,omitempty"`
	TopicType *int   `json:"topic_type,omitempty"`
}

type AIClient interface {
	Chat(ctx context.Context, history []ChatMessage, systemInstruction string) (string, []ChatSource, error)
	ModelID() string
}

// StreamingClient is an optional extension of AIClient for token-by-token streaming.
// Use a type assertion to check: sc, ok := client.(StreamingClient)
type StreamingClient interface {
	ChatSimpleStream(ctx context.Context, messages []ChatMessage, systemInstruction string, onToken func(string) error) error
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

func (a *AIAgent) Chat(ctx context.Context, history []ChatMessage, systemInstruction string) (string, []ChatSource, error) {
	return a.client.Chat(ctx, history, systemInstruction)
}
