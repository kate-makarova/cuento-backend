package MCP

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"google.golang.org/genai"
)

// ToolExecutor is a function that executes a tool call and returns a JSON string result.
type ToolExecutor func(ctx context.Context, args map[string]any) (string, error)

// ToolDefinition pairs a Gemini FunctionDeclaration with its executor.
type ToolDefinition struct {
	Declaration *genai.FunctionDeclaration
	Execute     ToolExecutor
}

// toolRegistry holds all registered tools for use by the AI agent.
var toolRegistry []ToolDefinition

// registerTool adds a tool to both the MCP server and the AI tool registry.
func registerTool(s *server.MCPServer, tool mcp.Tool, declaration *genai.FunctionDeclaration, executor ToolExecutor) {
	// Register with MCP server for external clients
	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := executor(ctx, req.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})

	// Register in AI tool registry
	toolRegistry = append(toolRegistry, ToolDefinition{
		Declaration: declaration,
		Execute:     executor,
	})
}

// GetToolRegistry returns all registered tool definitions for use by the AI agent.
func GetToolRegistry() []ToolDefinition {
	return toolRegistry
}

// buildGeminiTools converts the registry into a Gemini Tool slice.
func buildGeminiTools() []*genai.Tool {
	if len(toolRegistry) == 0 {
		return nil
	}
	declarations := make([]*genai.FunctionDeclaration, len(toolRegistry))
	for i, t := range toolRegistry {
		declarations[i] = t.Declaration
	}
	return []*genai.Tool{{FunctionDeclarations: declarations}}
}

// findExecutor looks up a tool executor by function name.
func findExecutor(name string) (ToolExecutor, bool) {
	for _, t := range toolRegistry {
		if t.Declaration.Name == name {
			return t.Execute, true
		}
	}
	return nil, false
}

// buildSchemaProperties is a helper to build genai.Schema properties map.
func buildSchemaProperties(props map[string]*genai.Schema) map[string]*genai.Schema {
	return props
}

func schemaObject(required []string, props map[string]*genai.Schema) *genai.Schema {
	return &genai.Schema{
		Type:       genai.TypeObject,
		Properties: props,
		Required:   required,
	}
}

func schemaString(description string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeString, Description: description}
}

func schemaInteger(description string) *genai.Schema {
	return &genai.Schema{Type: genai.TypeInteger, Description: description}
}

func schemaStringArray(description string) *genai.Schema {
	return &genai.Schema{
		Type:        genai.TypeArray,
		Description: description,
		Items:       &genai.Schema{Type: genai.TypeString},
	}
}
