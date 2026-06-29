package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	PlaneAPIKey          string
	PlaneBaseURL         string
	PlaneWorkspaceSlug   string
	PlaneMCPProfile      string
	PlaneMCPTools        []string
	CFAccessClientID     string
	CFAccessClientSecret string
	MCPCallerSecret      string
}

func Load() (*Config, error) {
	apiKey := os.Getenv("PLANE_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("PLANE_API_KEY environment variable is required")
	}

	baseURL := os.Getenv("PLANE_BASE_URL")
	if baseURL == "" {
		return nil, fmt.Errorf("PLANE_BASE_URL environment variable is required")
	}

	workspaceSlug := os.Getenv("PLANE_WORKSPACE_SLUG")
	if workspaceSlug == "" {
		return nil, fmt.Errorf("PLANE_WORKSPACE_SLUG environment variable is required")
	}

	profile := os.Getenv("PLANE_MCP_PROFILE")
	if profile == "" {
		profile = "full"
	}
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile != "worker" && profile != "planner" && profile != "full" && profile != "reviewer" {
		return nil, fmt.Errorf("PLANE_MCP_PROFILE must be one of 'worker', 'planner', 'reviewer', or 'full', got '%s'", profile)
	}

	var tools []string
	toolsEnv := os.Getenv("PLANE_MCP_TOOLS")
	if toolsEnv != "" {
		parts := strings.Split(toolsEnv, ",")
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				tools = append(tools, trimmed)
			}
		}
	}

	return &Config{
		PlaneAPIKey:          apiKey,
		PlaneBaseURL:         baseURL,
		PlaneWorkspaceSlug:   workspaceSlug,
		PlaneMCPProfile:      profile,
		PlaneMCPTools:        tools,
		CFAccessClientID:     os.Getenv("CF_ACCESS_CLIENT_ID"),
		CFAccessClientSecret: os.Getenv("CF_ACCESS_CLIENT_SECRET"),
		MCPCallerSecret:      os.Getenv("MCP_CALLER_SECRET"),
	}, nil
}
