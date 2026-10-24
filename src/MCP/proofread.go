package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const proofreadSystemInstruction = `You are a proofreader. Fix all spelling, grammar, and punctuation errors in the text the user provides.

What to fix:
- Spelling mistakes and typos
- Grammatical errors: wrong verb forms, tense inconsistencies, subject-verb disagreement, incorrect pronoun case
- Punctuation errors: missing or misplaced commas, periods, apostrophes, quotation marks

What NOT to change:
- Do NOT rephrase, reword, or restructure any sentence.
- Do NOT add, remove, or change any ideas, facts, or meaning.
- Do NOT change the tone, style, or voice of the text.
- Do NOT expand abbreviations or change formatting.
- If a sentence is stylistically unusual but grammatically correct, leave it as-is.

Return your response as a JSON object with exactly this structure:
{"text": "<corrected text>", "fixes": ["<short description of fix 1>", "<short description of fix 2>"]}
If no fixes were needed, return: {"text": "<original text unchanged>", "fixes": []}
Return only the raw JSON object — no markdown fences, no commentary.`

// ProofreadResult holds the corrected text and a human-readable list of changes.
type ProofreadResult struct {
	Text  string   `json:"text"`
	Fixes []string `json:"fixes"`
}

// ProofreadText asks a large model to fix spelling, grammar, and punctuation — no rephrasing.
func ProofreadText(text string, db *sql.DB) (*ProofreadResult, error) {
	pool := getActivePool()
	if pool == nil {
		return nil, fmt.Errorf("AI is not configured")
	}

	client, err := pool.ClientForMinSize(Entities.AIModelSizeLarge, Entities.AIModelTypeText)
	if err != nil {
		return nil, fmt.Errorf("no AI model available: %w", err)
	}

	var result ProofreadResult
	err = withModelRetry(
		pool,
		Entities.AIModelSizeLarge, Entities.AIModelTypeText,
		client,
		func(c AIClient) error {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			reply, _, e := c.Chat(ctx, []ChatMessage{
				{Role: "user", Content: text},
			}, proofreadSystemInstruction)
			if e != nil {
				return e
			}

			reply = strings.TrimSpace(reply)
			if strings.HasPrefix(reply, "```") {
				reply = strings.TrimPrefix(reply, "```json")
				reply = strings.TrimPrefix(reply, "```")
				reply = strings.TrimSuffix(reply, "```")
				reply = strings.TrimSpace(reply)
			}

			if err := json.Unmarshal([]byte(reply), &result); err != nil {
				return fmt.Errorf("failed to parse proofreading response: %w", err)
			}
			return nil
		},
		nil,
		func(err error) bool {
			msg := err.Error()
			return strings.Contains(msg, "429") ||
				strings.Contains(msg, "quota") ||
				strings.Contains(msg, "failed to parse proofreading response") ||
				strings.Contains(msg, "context deadline exceeded")
		},
	)
	if err != nil {
		return nil, err
	}
	return &result, nil
}
