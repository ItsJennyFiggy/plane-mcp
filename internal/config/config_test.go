package config

import (
	"os"
	"reflect"
	"testing"
)

func TestLoad_Success(t *testing.T) {
	// Arrange
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "test-api-key")
	os.Setenv("PLANE_BASE_URL", "https://plane.example.com")
	os.Setenv("PLANE_WORKSPACE_SLUG", "my-workspace")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.PlaneAPIKey != "test-api-key" {
		t.Errorf("expected PlaneAPIKey 'test-api-key', got '%s'", cfg.PlaneAPIKey)
	}
	if cfg.PlaneBaseURL != "https://plane.example.com" {
		t.Errorf("expected PlaneBaseURL 'https://plane.example.com', got '%s'", cfg.PlaneBaseURL)
	}
	if cfg.PlaneWorkspaceSlug != "my-workspace" {
		t.Errorf("expected PlaneWorkspaceSlug 'my-workspace', got '%s'", cfg.PlaneWorkspaceSlug)
	}
	if cfg.PlaneMCPProfile != "full" {
		t.Errorf("expected PlaneMCPProfile to default to 'full', got '%s'", cfg.PlaneMCPProfile)
	}
	if len(cfg.PlaneMCPTools) != 0 {
		t.Errorf("expected PlaneMCPTools to be empty, got %v", cfg.PlaneMCPTools)
	}
}

func TestLoad_MissingRequired(t *testing.T) {
	// Arrange
	requiredEnv := []string{"PLANE_API_KEY", "PLANE_BASE_URL", "PLANE_WORKSPACE_SLUG"}

	for _, missing := range requiredEnv {
		os.Clearenv()
		// Set all except 'missing'
		for _, env := range requiredEnv {
			if env != missing {
				os.Setenv(env, "some-val")
			}
		}

		// Act
		_, err := Load()

		// Assert
		if err == nil {
			t.Errorf("expected error when %s is missing, got nil", missing)
		}
	}
}

func TestLoad_InvalidProfile(t *testing.T) {
	// Arrange
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "key")
	os.Setenv("PLANE_BASE_URL", "url")
	os.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
	os.Setenv("PLANE_MCP_PROFILE", "invalid-profile")

	// Act
	_, err := Load()

	// Assert
	if err == nil {
		t.Error("expected error for invalid profile 'invalid-profile', got nil")
	}
}

func TestLoad_ValidProfiles(t *testing.T) {
	validProfiles := []string{"worker", "planner", "full"}

	for _, profile := range validProfiles {
		// Arrange
		os.Clearenv()
		os.Setenv("PLANE_API_KEY", "key")
		os.Setenv("PLANE_BASE_URL", "url")
		os.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
		os.Setenv("PLANE_MCP_PROFILE", profile)

		// Act
		cfg, err := Load()

		// Assert
		if err != nil {
			t.Fatalf("expected no error for profile '%s', got %v", profile, err)
		}
		if cfg.PlaneMCPProfile != profile {
			t.Errorf("expected profile '%s', got '%s'", profile, cfg.PlaneMCPProfile)
		}
	}
}

func TestLoad_ParseTools(t *testing.T) {
	// Arrange
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "key")
	os.Setenv("PLANE_BASE_URL", "url")
	os.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
	os.Setenv("PLANE_MCP_TOOLS", "tool1,tool2, tool3 ") // tests trimming whitespace

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	expectedTools := []string{"tool1", "tool2", "tool3"}
	if !reflect.DeepEqual(cfg.PlaneMCPTools, expectedTools) {
		t.Errorf("expected tools %v, got %v", expectedTools, cfg.PlaneMCPTools)
	}
}

func TestLoad_CFAccess(t *testing.T) {
	// Arrange
	os.Clearenv()
	os.Setenv("PLANE_API_KEY", "key")
	os.Setenv("PLANE_BASE_URL", "url")
	os.Setenv("PLANE_WORKSPACE_SLUG", "workspace")
	os.Setenv("CF_ACCESS_CLIENT_ID", "cf-id")
	os.Setenv("CF_ACCESS_CLIENT_SECRET", "cf-secret")

	// Act
	cfg, err := Load()

	// Assert
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.CFAccessClientID != "cf-id" {
		t.Errorf("expected CFAccessClientID 'cf-id', got '%s'", cfg.CFAccessClientID)
	}
	if cfg.CFAccessClientSecret != "cf-secret" {
		t.Errorf("expected CFAccessClientSecret 'cf-secret', got '%s'", cfg.CFAccessClientSecret)
	}
}
