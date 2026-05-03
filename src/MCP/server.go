package MCP

import (
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"

	"github.com/mark3labs/mcp-go/server"
)

var activeAgent *AIAgent

func StartMCPServer(db *sql.DB, addr string) error {
	agent, err := buildAgent(db)
	if err != nil {
		fmt.Printf("MCP: AI agent not initialized: %v\n", err)
	}
	activeAgent = agent

	s := server.NewMCPServer(
		"cuento",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	// Tools will be registered here

	httpServer := server.NewStreamableHTTPServer(s)
	fmt.Printf("MCP server listening on %s/mcp\n", addr)
	return httpServer.Start(addr)
}

func buildAgent(db *sql.DB) (*AIAgent, error) {
	apiKey, err := Services.GetGlobalSetting("ai_api_key", db)
	if err != nil || apiKey == "" {
		return nil, fmt.Errorf("ai_api_key not set")
	}

	aiName, err := Services.GetGlobalSetting("ai_name", db)
	if err != nil || aiName == "" {
		return nil, fmt.Errorf("ai_name not set")
	}

	switch aiName {
	case "gemini":
		client, err := NewGeminiClient(apiKey, "gemini-2.0-flash")
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client: %w", err)
		}
		return NewAIAgent(client), nil
	default:
		return nil, fmt.Errorf("unknown ai_name: %s", aiName)
	}
}
