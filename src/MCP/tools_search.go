package MCP

import (
	"context"
	"cuento-backend/src/Services"
	"database/sql"
	"encoding/json"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type bucketSearchResult struct {
	Bucket  string                      `json:"bucket"`
	Results []Services.SearchResultItem `json:"results"`
}

func registerSearchTools(s *server.MCPServer, db *sql.DB) {
	s.AddTool(
		mcp.NewTool("get_search_buckets",
			mcp.WithDescription("Returns the list of available search buckets that can be used with search_content."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			data, _ := json.Marshal(Services.AllSonicBuckets)
			return mcp.NewToolResultText(string(data)), nil
		},
	)

	bucketsDesc := strings.Join(Services.AllSonicBuckets, ", ")

	s.AddTool(
		mcp.NewTool("search_content",
			mcp.WithDescription("Search forum content using a keyword or phrase. "+
				"Queries Sonic for matching IDs, then fetches full content from the database. "+
				"Results are filtered by subforum visibility permissions. "+
				"Available buckets: "+bucketsDesc+"."),
			mcp.WithNumber("user_id",
				mcp.Required(),
				mcp.Description("The ID of the current user. Used to apply subforum visibility permissions."),
			),
			mcp.WithString("query",
				mcp.Required(),
				mcp.Description("The keyword or phrase to search for."),
			),
			mcp.WithArray("buckets",
				mcp.WithStringItems(),
				mcp.Description("Which buckets to search. Omit to search all buckets. Available: "+bucketsDesc),
			),
			mcp.WithNumber("subforum_id",
				mcp.Description("Optional. Restrict results to a specific subforum ID."),
			),
			mcp.WithNumber("topic_type",
				mcp.Description("Optional. Restrict results by topic type: 0=general, 1=episode, 2=character_sheet, 3=wanted_character, 4=lore."),
			),
			mcp.WithNumber("limit",
				mcp.Description("Max results per bucket. Default 10, max 100."),
			),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			query := strings.TrimSpace(req.GetString("query", ""))
			if query == "" {
				return mcp.NewToolResultError("query must not be empty"), nil
			}

			userID := req.GetInt("user_id", 0)

			buckets := req.GetStringSlice("buckets", Services.AllSonicBuckets)

			filterSubforum := req.GetInt("subforum_id", 0)
			filterTopicType := req.GetInt("topic_type", 0)

			limit := req.GetInt("limit", 10)
			if limit <= 0 || limit > 100 {
				limit = 10
			}

			visibleIDs, _ := Services.GetVisibleSubforums(userID, "subforum_read", db)
			visibleSet := make(map[int]bool, len(visibleIDs))
			for _, id := range visibleIDs {
				visibleSet[id] = true
			}

			var output []bucketSearchResult
			for _, bucket := range buckets {
				items, err := Services.SearchInBucket(bucket, query, limit, filterSubforum, filterTopicType, visibleSet, db)
				if err != nil || len(items) == 0 {
					continue
				}
				output = append(output, bucketSearchResult{Bucket: bucket, Results: items})
			}

			if output == nil {
				output = []bucketSearchResult{}
			}

			data, err := json.Marshal(output)
			if err != nil {
				return mcp.NewToolResultError("failed to serialize results"), nil
			}
			return mcp.NewToolResultText(string(data)), nil
		},
	)
}
