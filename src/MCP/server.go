package MCP

import (
	"context"
	"cuento-backend/src/Services"
	"database/sql"
	"fmt"

	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/genai"
)

var activePool *AIModelPool

func ReinitializeAgent(db *sql.DB) {
	activePool = NewAIModelPool(db)
	fmt.Println("MCP: AI model pool reinitialized")
}

func StartMCPServer(db *sql.DB, addr string) error {
	activePool = NewAIModelPool(db)

	s := server.NewMCPServer(
		"cuento",
		"1.0.0",
		server.WithToolCapabilities(true),
	)

	registerLoreTools(s, db)
	registerSearchTools(s, db)

	StartQueueWorker(db)

	httpServer := server.NewStreamableHTTPServer(s)
	fmt.Printf("MCP server listening on %s/mcp\n", addr)
	return httpServer.Start(addr)
}

func ListAvailableModels(db *sql.DB) ([]string, error) {
	apiKey, err := Services.GetGlobalSetting("ai_api_key", db)
	if err != nil || apiKey == "" {
		return []string{}, nil
	}

	aiName, err := Services.GetGlobalSetting("ai_name", db)
	if err != nil || aiName == "" {
		return []string{}, nil
	}

	switch aiName {
	case "gemini":
		client, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: apiKey})
		if err != nil {
			return nil, fmt.Errorf("failed to create Gemini client: %w", err)
		}
		page, err := client.Models.List(context.Background(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list Gemini models: %w", err)
		}
		names := make([]string, 0, len(page.Items))
		for _, m := range page.Items {
			names = append(names, m.Name)
		}
		return names, nil
	default:
		return []string{}, nil
	}
}

