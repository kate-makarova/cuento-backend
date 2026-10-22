package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

const proofreadSystemInstruction = `You are a proofreader. Your only task is to fix typos, grammatical errors, and punctuation mistakes in the text the user provides.

Rules you must follow without exception:
- Do NOT rephrase, reword, or restructure any sentence.
- Do NOT add, remove, or change any ideas, facts, or meaning.
- Do NOT change the tone, style, or voice of the text.
- Do NOT expand abbreviations or change formatting.
- Only correct clear errors: misspellings, wrong verb forms, missing or misplaced punctuation, obvious agreement errors.
- If a sentence is awkward but grammatically acceptable, leave it as-is.

Return your response as a JSON object with exactly this structure:
{"text": "<corrected text>", "fixes": ["<short description of fix 1>", "<short description of fix 2>"]}
If no fixes were needed, return: {"text": "<original text unchanged>", "fixes": []}
Return only the raw JSON object — no markdown fences, no commentary.`

// ProofreadResult holds the corrected text and a human-readable list of changes.
type ProofreadResult struct {
	Text  string   `json:"text"`
	Fixes []string `json:"fixes"`
}

// ProofreadText checks the input for prompt injection, then asks a large model
// to fix only typos, grammar, and punctuation — no rephrasing.
func ProofreadText(text string, db *sql.DB) (*ProofreadResult, error) {
	pool := getActivePool()
	if pool == nil {
		return nil, fmt.Errorf("AI is not configured")
	}
	client, err := pool.ClientForMinSize(Entities.AIModelSizeLarge, Entities.AIModelTypeText)
	if err != nil {
		return nil, fmt.Errorf("no AI model available: %w", err)
	}

	reply, _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: text},
	}, proofreadSystemInstruction)
	if err != nil {
		return nil, fmt.Errorf("proofreading failed: %w", err)
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
