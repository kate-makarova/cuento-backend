package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
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

	const maxRetries = 4
	var reply string
	var excluded []string
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		var client AIClient
		if attempt == 0 {
			client, lastErr = pool.ClientForMinSize(Entities.AIModelSizeLarge, Entities.AIModelTypeText)
		} else {
			client, lastErr = pool.ClientExcluding(Entities.AIModelSizeLarge, Entities.AIModelTypeText, excluded)
		}
		if lastErr != nil {
			return nil, fmt.Errorf("no AI model available: %w", lastErr)
		}

		reply, _, lastErr = client.Chat(context.Background(), []ChatMessage{
			{Role: "user", Content: text},
		}, proofreadSystemInstruction)
		if lastErr == nil {
			break
		}

		is429 := strings.Contains(lastErr.Error(), "429") || strings.Contains(lastErr.Error(), "quota")
		if !is429 || attempt == maxRetries {
			return nil, fmt.Errorf("proofreading failed: %w", lastErr)
		}
		excluded = append(excluded, client.ModelID())
	}

	// Strip accidental markdown fences the model may add.
	reply = strings.TrimSpace(reply)
	if strings.HasPrefix(reply, "```") {
		reply = strings.TrimPrefix(reply, "```json")
		reply = strings.TrimPrefix(reply, "```")
		reply = strings.TrimSuffix(reply, "```")
		reply = strings.TrimSpace(reply)
	}

	var result ProofreadResult
	if err := json.Unmarshal([]byte(reply), &result); err != nil {
		return nil, fmt.Errorf("failed to parse proofreading response: %w", err)
	}
	return &result, nil
}
