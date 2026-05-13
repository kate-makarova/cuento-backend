package MCP

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type loreTopicResult struct {
	ID         int    `json:"id"`
	Name       string `json:"name"`
	Status     int    `json:"status"`
	SubforumID int    `json:"subforum_id"`
}

func registerLoreTools(s *server.MCPServer, db *sql.DB) {
	s.AddTool(
		mcp.NewTool("get_lore_topics",
			mcp.WithDescription("Returns a list of all lore topics with their ID, name, status, and subforum ID."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			rows, err := db.QueryContext(ctx,
				`SELECT t.id, t.name, t.status, t.subforum_id
				 FROM topics t
				 WHERE t.type = 4 AND t.status != 3
				 ORDER BY t.name ASC`,
			)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to query lore topics: %v", err)), nil
			}
			defer rows.Close()

			topics := make([]loreTopicResult, 0)
			for rows.Next() {
				var t loreTopicResult
				if err := rows.Scan(&t.ID, &t.Name, &t.Status, &t.SubforumID); err != nil {
					continue
				}
				topics = append(topics, t)
			}

			data, err := json.Marshal(topics)
			if err != nil {
				return mcp.NewToolResultError("failed to serialize results"), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}
