package MCP

import (
	"context"
	"cuento-backend/src/Entities"
	"database/sql"
	"fmt"
)

const proofreadSystemInstruction = `You are a proofreader. Your only task is to fix typos, grammatical errors, and punctuation mistakes in the text the user provides.

Rules you must follow without exception:
- Do NOT rephrase, reword, or restructure any sentence.
- Do NOT add, remove, or change any ideas, facts, or meaning.
- Do NOT change the tone, style, or voice of the text.
- Do NOT expand abbreviations or change formatting.
- Only correct clear errors: misspellings, wrong verb forms, missing or misplaced punctuation, obvious agreement errors.
- If a sentence is awkward but grammatically acceptable, leave it as-is.
- Return only the corrected text. Do not add any commentary, explanation, or markup.`

// ProofreadText checks the input for prompt injection, then asks a large model
// to fix only typos, grammar, and punctuation — no rephrasing.
func ProofreadText(text string, db *sql.DB) (string, error) {
	check, err := CheckForPromptInjection(text, db)
	if err != nil {
		return "", fmt.Errorf("injection check failed: %w", err)
	}
	if check.IsInjection {
		return "", fmt.Errorf("suspicious input detected: %s", check.Reason)
	}

	client, err := activePool.ClientForMinSize(Entities.AIModelSizeLarge, Entities.AIModelTypeText)
	if err != nil {
		return "", fmt.Errorf("no AI model available: %w", err)
	}

	reply, _, err := client.Chat(context.Background(), []ChatMessage{
		{Role: "user", Content: text},
	}, proofreadSystemInstruction)
	if err != nil {
		return "", fmt.Errorf("proofreading failed: %w", err)
	}

	return reply, nil
}
