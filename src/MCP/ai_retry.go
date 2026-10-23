package MCP

import (
	"cuento-backend/src/Entities"
	"strings"
)

// withModelRetry calls fn with a client from pool, retrying with a fresh model on
// 429 rate-limit errors. The first attempt uses initialClient; subsequent attempts
// call pool.ClientExcluding with the accumulated list of failed model IDs.
//
// onRetry is called before each retry with the model ID that was rate-limited; pass
// nil if no side effects are needed. Returns the first non-429 error, or nil on success.
func withModelRetry(
	pool ModelPool,
	minSize Entities.AIModelSize,
	modelType Entities.AIModelType,
	initialClient AIClient,
	fn func(AIClient) error,
	onRetry func(modelID string),
) error {
	const maxRetries = 4
	excluded := make([]string, 0, maxRetries)
	client := initialClient

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			var err error
			client, err = pool.ClientExcluding(minSize, modelType, excluded)
			if err != nil {
				return err
			}
		}

		err := fn(client)
		if err == nil {
			return nil
		}

		is429 := strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "quota")
		if !is429 || attempt == maxRetries {
			return err
		}

		excluded = append(excluded, client.ModelID())
		if onRetry != nil {
			onRetry(client.ModelID())
		}
	}

	return nil
}
