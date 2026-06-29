package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

// authorizeRequest checks the caller secret from either the Authorization
// Bearer header or the X-Caller-Secret header, never a URL query param.
func authorizeRequest(req *http.Request, secret string) bool {
	provided := ""
	auth := req.Header.Get("Authorization")
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		provided = auth[len("Bearer "):]
	}
	if provided == "" {
		provided = req.Header.Get("X-Caller-Secret")
	}
	return provided != "" && provided == secret
}

// createHTTPHandler wraps the MCP Streamable HTTP handler with a caller-secret
// check and a DNS-rebinding-safe Host header rewrite for sibling Docker
// containers reaching this service via host.docker.internal.
func createHTTPHandler(server *mcp.Server, secret string) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(req *http.Request) *mcp.Server {
		if !authorizeRequest(req, secret) {
			log.Printf("Unauthorized connection attempt from %s", req.RemoteAddr)
			return nil
		}
		return server
	}, nil)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Bypass DNS rebinding check for Docker containers by rewriting the Host header.
		// We extract the hostname to perform an exact comparison to prevent subdomain bypasses.
		host := r.Host
		port := ""
		if idx := strings.LastIndex(r.Host, ":"); idx != -1 {
			if !strings.HasSuffix(r.Host, "]") { // Avoid matching bracketed IPv6 address without port
				host = r.Host[:idx]
				port = r.Host[idx:]
			}
		}
		if host == "host.docker.internal" {
			r.Host = "localhost" + port
		}
		mcpHandler.ServeHTTP(w, r)
	})
}

func main() {
	var portFlag string
	var hostFlag string

	flag.StringVar(&portFlag, "port", "", "Port to run the Streamable HTTP server on (e.g. 8080). Overrides PORT env var. If unset, runs stdio transport.")
	flag.StringVar(&hostFlag, "host", "127.0.0.1", "Host/IP address to bind the HTTP server to (default 127.0.0.1). Use 0.0.0.0 to allow connections from Docker containers.")
	flag.Parse()

	port := portFlag
	if port == "" {
		port = os.Getenv("PORT")
	}

	server, err := createServer()
	if err != nil {
		log.Fatalf("Server startup failed: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if port != "" {
		secret := os.Getenv("MCP_CALLER_SECRET")
		if secret == "" {
			log.Fatal("MCP_CALLER_SECRET must be set to run the Streamable HTTP transport")
		}

		handler := createHTTPHandler(server, secret)
		httpServer := &http.Server{
			Addr:         hostFlag + ":" + port,
			Handler:      handler,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		go func() {
			log.Printf("Starting plane-mcp Streamable HTTP server on http://%s:%s...", hostFlag, port)
			if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("HTTP server failed: %v", err)
			}
		}()

		<-ctx.Done()
		log.Println("Shutting down Streamable HTTP server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Printf("HTTP server shutdown error: %v", err)
		}
		log.Println("Server exited properly")
		return
	}

	log.Println("Starting plane-mcp server on stdio transport...")
	if err := server.Run(ctx, &mcp.StdioTransport{}); err != nil {
		if errors.Is(err, mcp.ErrConnectionClosed) || errors.Is(err, context.Canceled) {
			log.Println("Server stopped")
			return
		}
		log.Fatalf("Server run failed: %v", err)
	}
	log.Println("Server stopped")
}
