//go:build live

package plane_test

import (
	"context"
	"os"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
)

// TestLiveSearchWorkItems validates that the Plane search endpoint returns
// sequence_id as a JSON number and that the Go client unmarshals it
// correctly into SearchWorkItemResult.SequenceID (int).
//
// Requires live credentials. Run with:
//
//	PLANE_API_KEY=... PLANE_BASE_URL=... PLANE_WORKSPACE_SLUG=... \
//	  go test -tags=live -run TestLiveSearchWorkItems ./internal/plane/ -count=1 -v
func TestLiveSearchWorkItems(t *testing.T) {
	apiKey := os.Getenv("PLANE_API_KEY")
	baseURL := os.Getenv("PLANE_BASE_URL")
	workspaceSlug := os.Getenv("PLANE_WORKSPACE_SLUG")

	if apiKey == "" || baseURL == "" || workspaceSlug == "" {
		t.Skip("PLANE_API_KEY, PLANE_BASE_URL, and PLANE_WORKSPACE_SLUG must be set")
	}

	cfg := &config.Config{
		PlaneAPIKey:        apiKey,
		PlaneBaseURL:       baseURL,
		PlaneWorkspaceSlug: workspaceSlug,
	}
	client := plane.NewClient(cfg)

	ctx := context.Background()

	// Search for any common word that should return results.
	results, err := client.SearchWorkItems(ctx, map[string]string{
		"search": "test",
		"limit":  "5",
	})
	if err != nil {
		t.Fatalf("SearchWorkItems failed: %v", err)
	}

	// Even if the workspace has zero "test" work items, we should get a
	// valid (empty) response without a JSON unmarshal error.
	t.Logf("Got %d search results for query 'test'", len(results))

	for i, r := range results {
		t.Logf("  [%d] id=%s name=%q sequence_id=%d project=%s workspace=%s",
			i, r.ID, r.Name, r.SequenceID, r.ProjectIdentifier, r.WorkspaceSlug)

		if r.ID == "" {
			t.Errorf("result[%d]: id must not be empty", i)
		}
		if r.Name == "" {
			t.Errorf("result[%d]: name must not be empty", i)
		}
		// The critical assertion: SequenceID must be a positive int.
		// Before the fix, SequenceID was typed as string, causing
		// json.Unmarshal to fail on the Plane API's JSON number.
		if r.SequenceID <= 0 {
			t.Errorf("result[%d]: sequence_id must be > 0, got %d", i, r.SequenceID)
		}
		if r.ProjectIdentifier == "" {
			t.Errorf("result[%d]: project__identifier must not be empty", i)
		}
		if r.ProjectID == "" {
			t.Errorf("result[%d]: project_id must not be empty", i)
		}
	}
}
