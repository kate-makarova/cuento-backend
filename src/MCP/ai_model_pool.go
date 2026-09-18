package MCP

import (
	"cuento-backend/src/Entities"
	"database/sql"
	"fmt"
)

type AIModelPool struct {
	db *sql.DB
}

func NewAIModelPool(db *sql.DB) *AIModelPool {
	return &AIModelPool{db: db}
}

// ClientForMinSize returns the smallest active AI client of the given type whose size >= minSize.
func (p *AIModelPool) ClientForMinSize(minSize Entities.AIModelSize, modelType Entities.AIModelType) (AIClient, error) {
	var (
		apiAddress, apiKey, machineName string
		protocolType                    int
	)

	err := p.db.QueryRow(`
		SELECT api_address, api_key, machine_name, protocol_type
		FROM ai_models
		WHERE is_active = 1 AND size >= ? AND model_type = ?
		ORDER BY size ASC
		LIMIT 1
	`, minSize, modelType).Scan(&apiAddress, &apiKey, &machineName, &protocolType)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no active AI model available with size >= %d and model_type = %d", minSize, modelType)
	}
	if err != nil {
		return nil, fmt.Errorf("querying ai_models: %w", err)
	}

	switch Entities.AIModelProtocolType(protocolType) {
	case Entities.AIModelProtocolOpenAI:
		return NewOpenAICompatClient(apiKey, machineName, apiAddress)
	case Entities.AIModelProtocolAnthropic:
		return NewClaudeClient(apiKey, machineName)
	case Entities.AIModelProtocolGemini:
		return NewGeminiClient(apiKey, machineName)
	default:
		return nil, fmt.Errorf("unsupported protocol_type: %d", protocolType)
	}
}
