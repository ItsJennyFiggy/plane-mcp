package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerStubToolRegistration(t *testing.T) {
	// Arrange
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "test-api-key")
	os.Setenv("PLANE_BASE_URL", "https://plane.example.com")
	os.Setenv("PLANE_WORKSPACE_SLUG", "my-workspace")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server, err := createServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()

	// Run the server in a goroutine
	go func() {
		if err := server.Run(ctx, serverTransport); err != nil {
			// ignore context cancelled error which is expected
		}
	}()

	// Connect the client
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "test-client",
		Version: "1.0.0",
	}, nil)

	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	defer session.Close()

	// Act
	toolsResult, err := session.ListTools(ctx, &mcp.ListToolsParams{})

	// Assert
	if err != nil {
		t.Fatalf("failed to list tools: %v", err)
	}

	foundPing := false
	for _, tool := range toolsResult.Tools {
		if tool.Name == "ping" {
			foundPing = true
			break
		}
	}

	if !foundPing {
		t.Errorf("expected to find tool 'ping', but got: %v", toolsResult.Tools)
	}

	// Act: Call the ping tool
	callResult, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "ping",
	})

	// Assert
	if err != nil {
		t.Fatalf("failed to call ping tool: %v", err)
	}
	if len(callResult.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(callResult.Content))
	}
	textContent, ok := callResult.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected content to be TextContent, got %T", callResult.Content[0])
	}
	expectedText := "pong: connected to workspace my-workspace"
	if textContent.Text != expectedText {
		t.Errorf("expected text '%s', got '%s'", expectedText, textContent.Text)
	}
}

func TestCreateServer_MissingConfig(t *testing.T) {
	// Arrange
	os.Clearenv() // missing required config

	// Act
	_, err := createServer()

	// Assert
	if err == nil {
		t.Error("expected error when config is missing, got nil")
	}
}
