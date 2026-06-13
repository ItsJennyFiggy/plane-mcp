package tools

import (
	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Register wires up all five tool handlers to the MCP server, gated by profile/tool allowlist.
//
// Compile-time interface assertions ensure that *plane.Client and *plane.Resolver satisfy
// the planeClient and planeResolver interfaces defined in tools.go. These assertions will
// fail to compile until the parallel agent adds the missing methods.
func Register(server *mcp.Server, client *plane.Client, resolver *plane.Resolver, cfg *config.Config) {
	// Compile-time interface satisfaction checks.
	// These will fail until the parallel agent's methods are merged.
	var _ planeClient = client
	var _ planeResolver = resolver

	f := &resolverFormatter{resolver: resolver}
	registerWithDeps(server, client, resolver, f, cfg)
}
