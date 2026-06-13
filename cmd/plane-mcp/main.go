package main

import (
	"context"
	"fmt"
	"log"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/ItsJennyFiggy/plane-mcp/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type PingArgs struct{}

func createServer() (*mcp.Server, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("config error: %w", err)
	}

	planeClient := plane.NewClient(cfg)
	resolver := plane.NewResolver(planeClient)

	server := mcp.NewServer(&mcp.Implementation{
		Name:    "plane-mcp",
		Version: "1.0.0",
	}, nil)

	// Add ping stub tool
	mcp.AddTool(server, &mcp.Tool{
		Name:        "ping",
		Description: "Verify the MCP server connection",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args PingArgs) (*mcp.CallToolResult, any, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{
					Text: fmt.Sprintf("pong: connected to workspace %s", cfg.PlaneWorkspaceSlug),
				},
			},
		}, nil, nil
	})

	tools.Register(server, planeClient, resolver, cfg)

	return server, nil
}

func main() {
	server, err := createServer()
	if err != nil {
		log.Fatalf("Server startup failed: %v", err)
	}

	log.Println("Starting plane-mcp server on stdio transport...")
	if err := server.Run(context.Background(), &mcp.StdioTransport{}); err != nil {
		log.Fatalf("Server run failed: %v", err)
	}
	log.Println("Server stopped")
}
