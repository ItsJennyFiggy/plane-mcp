// Package tools — production entry point for tool registration.
//
// This file contains Register(), the function that wires concrete *plane.Client
// and *plane.Resolver into the MCP server. It is intentionally kept separate
// from tools.go so that the rest of the package (pure functions, interfaces,
// internal handler implementations) can compile and be tested independently.
//
// NOTE: This file will NOT compile until the parallel agent adds the following
// methods to the plane package:
//   - (*plane.Client).ListWorkItems
//   - (*plane.Client).CreateWorkItem
//   - (*plane.Client).CreateWorkItemComment
//   - (*plane.Client).UpdateWorkItem
//   - (*plane.Client).CreateWorkItemLink
//   - (*plane.Resolver).GetCallerID
//
// Until then, build the test binary with the build tag "skip_register" to
// exclude this file:
//
//	go test ./internal/tools/... -tags skip_register -run TestParseIdentifier
//
// Or, because the test commands in the task spec use -run (which scopes to
// specific test functions), you can also run the tests directly against
// tools_test.go after stripping this file from the build — see stubs_test.go.

//go:build !skip_register

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
