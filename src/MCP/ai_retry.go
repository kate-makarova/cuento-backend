package MCP

import (
	"cuento-backend/src/Entities"
	"strings"
)

// withModelRetry calls fn with a client from pool, retrying with a fresh model when
// shouldRetry returns true for the error. The first attempt uses initialClient;
// subsequent attempts call pool.ClientExcluding with the accumulated list of failed
// model IDs.
//
// Pass nil for shouldRetry to use the default (retry on 429 / quota errors only).
// onRetry is called before each retry with the failing model ID; pass nil if not needed.
func withModelRetry(
	pool ModelPool,
	minSize Entities.AIModelSize,
	modelType Entities.AIModelType,
	initialClient AIClient,
	fn func(AIClient) error,
	onRetry func(modelID string),
	shouldRetry ...func(error) bool,
) error {
	retryable := defaultShouldRetry
	if len(shouldRetry) > 0 && shouldRetry[0] != nil {
		retryable = shouldRetry[0]
	}

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

		if !retryable(err) || attempt == maxRetries {
			return err
		}

		excluded = append(excluded, client.ModelID())
		if onRetry != nil {
			onRetry(client.ModelID())
		}
	}

	return nil
}

func defaultShouldRetry(err error) bool {
	return strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "quota")
}
