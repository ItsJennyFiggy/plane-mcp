package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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
			_ = err
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

func TestAuthorizeRequest(t *testing.T) {
	const secret = "test-secret-123"

	tests := []struct {
		name    string
		headers map[string]string
		want    bool
	}{
		{
			name:    "valid secret via Authorization Bearer",
			headers: map[string]string{"Authorization": "Bearer test-secret-123"},
			want:    true,
		},
		{
			name:    "valid secret via X-Caller-Secret",
			headers: map[string]string{"X-Caller-Secret": "test-secret-123"},
			want:    true,
		},
		{
			name:    "invalid secret is rejected",
			headers: map[string]string{"Authorization": "Bearer wrong-secret"},
			want:    false,
		},
		{
			name:    "missing secret is rejected",
			headers: map[string]string{},
			want:    false,
		},
		{
			name:    "empty Bearer value is rejected",
			headers: map[string]string{"Authorization": "Bearer "},
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}

			got := authorizeRequest(req, secret)
			if got != tc.want {
				t.Errorf("authorizeRequest() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCreateHTTPHandlerAuthorizationModes(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "test-api-key")
	os.Setenv("PLANE_BASE_URL", "https://plane.example.com")
	os.Setenv("PLANE_WORKSPACE_SLUG", "my-workspace")

	server, err := createServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	const secret = "dev-secret-xyz"
	handler := createHTTPHandler(server, secret)

	initPayload := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "test-client",
				"version": "1.0.0"
			}
		}
	}`

	tests := []struct {
		name           string
		headers        map[string]string
		expectedStatus int
		expectSession  bool
	}{
		{
			name: "valid secret in Bearer token",
			headers: map[string]string{
				"Content-Type":  "application/json",
				"Accept":        "application/json, text/event-stream",
				"Authorization": "Bearer dev-secret-xyz",
			},
			expectedStatus: http.StatusOK,
			expectSession:  true,
		},
		{
			name: "valid secret in X-Caller-Secret header",
			headers: map[string]string{
				"Content-Type":    "application/json",
				"Accept":          "application/json, text/event-stream",
				"X-Caller-Secret": "dev-secret-xyz",
			},
			expectedStatus: http.StatusOK,
			expectSession:  true,
		},
		{
			name: "invalid secret is rejected",
			headers: map[string]string{
				"Content-Type":    "application/json",
				"Accept":          "application/json, text/event-stream",
				"X-Caller-Secret": "wrong-secret",
			},
			expectedStatus: http.StatusBadRequest,
			expectSession:  false,
		},
		{
			name: "missing secret is rejected",
			headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json, text/event-stream",
			},
			expectedStatus: http.StatusBadRequest,
			expectSession:  false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(initPayload))
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d, got %d. Body: %q", tc.expectedStatus, w.Code, w.Body.String())
			}

			sessionID := w.Header().Get("Mcp-Session-Id")
			if tc.expectSession && sessionID == "" {
				t.Error("expected non-empty Mcp-Session-Id header on successful connection")
			}
			if !tc.expectSession && sessionID != "" {
				t.Errorf("expected empty Mcp-Session-Id header on failed connection, got %s", sessionID)
			}
		})
	}
}

func TestHTTPHandlerHostHeaderRewriting(t *testing.T) {
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "test-api-key")
	os.Setenv("PLANE_BASE_URL", "https://plane.example.com")
	os.Setenv("PLANE_WORKSPACE_SLUG", "my-workspace")

	server, err := createServer()
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	const secret = "dev-secret-xyz"
	handler := createHTTPHandler(server, secret)

	initPayload := `{
		"jsonrpc": "2.0",
		"id": 1,
		"method": "initialize",
		"params": {
			"protocolVersion": "2024-11-05",
			"capabilities": {},
			"clientInfo": {
				"name": "test-client",
				"version": "1.0.0"
			}
		}
	}`

	tests := []struct {
		name           string
		incomingHost   string
		expectedStatus int
	}{
		{
			name:           "host.docker.internal is rewritten and accepted",
			incomingHost:   "host.docker.internal",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "host.docker.internal with port is rewritten and accepted",
			incomingHost:   "host.docker.internal:8085",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "spoofed lookalike host is rejected with 403 Forbidden",
			incomingHost:   "host.docker.internal.attacker.com",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "spoofed lookalike host with port is rejected with 403 Forbidden",
			incomingHost:   "host.docker.internal.attacker.com:8085",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "localhost is accepted",
			incomingHost:   "localhost:8085",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/", strings.NewReader(initPayload))
			ctx := context.WithValue(req.Context(), http.LocalAddrContextKey, &net.TCPAddr{
				IP:   net.IPv4(127, 0, 0, 1),
				Port: 8085,
			})
			req = req.WithContext(ctx)
			req.Host = tc.incomingHost
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/event-stream")
			req.Header.Set("Authorization", "Bearer "+secret)

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("expected status %d for host %q, but got %d. Body: %s", tc.expectedStatus, tc.incomingHost, w.Code, w.Body.String())
			}
		})
	}
}
