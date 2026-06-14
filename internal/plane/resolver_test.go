package plane

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
)

// Valid UUIDs to satisfy isUUID validation
const (
	projUUID1  = "11111111-1111-1111-1111-111111111111"
	projUUID2  = "22222222-2222-2222-2222-222222222222"
	stateUUID1 = "33333333-3333-3333-3333-333333333333"
	stateUUID2 = "44444444-4444-4444-4444-444444444444"
	stateUUID3 = "55555555-5555-5555-5555-555555555555"
	labelUUID1  = "66666666-6666-6666-6666-666666666666"
	labelUUID2  = "77777777-7777-7777-7777-777777777777"
	userUUID1   = "88888888-8888-8888-8888-888888888888"
	userUUID2   = "99999999-9999-9999-9999-999999999999"
	moduleUUID1 = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	moduleUUID2 = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
)

func TestResolverProjectResolution(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock ListProjects API call
	requestCount := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		requestCount++
		body := `[
			{"id": "` + projUUID1 + `", "name": "Agent Infra", "identifier": "AGENT"},
			{"id": "` + projUUID2 + `", "name": "Platform Work", "identifier": "PLAT"}
		]`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	t.Run("Resolve by UUID", func(t *testing.T) {
		p, err := resolver.ResolveProject(context.Background(), projUUID1)
		if err != nil {
			t.Fatalf("failed to resolve project: %v", err)
		}
		if p.Identifier != "AGENT" {
			t.Errorf("expected identifier AGENT, got '%s'", p.Identifier)
		}
	})

	t.Run("Resolve by Identifier Case-Insensitive", func(t *testing.T) {
		p, err := resolver.ResolveProject(context.Background(), "plat")
		if err != nil {
			t.Fatalf("failed to resolve project: %v", err)
		}
		if p.ID != projUUID2 {
			t.Errorf("expected ID %s, got '%s'", projUUID2, p.ID)
		}
	})

	t.Run("Resolve by Name Case-Insensitive", func(t *testing.T) {
		p, err := resolver.ResolveProject(context.Background(), "agent infra")
		if err != nil {
			t.Fatalf("failed to resolve project: %v", err)
		}
		if p.ID != projUUID1 {
			t.Errorf("expected ID %s, got '%s'", projUUID1, p.ID)
		}
	})

	t.Run("Verify Cache Hit (No API call)", func(t *testing.T) {
		initialRequests := requestCount
		_, err := resolver.ResolveProject(context.Background(), "AGENT")
		if err != nil {
			t.Fatalf("failed to resolve: %v", err)
		}
		if requestCount != initialRequests {
			t.Errorf("expected request count to remain %d, but increased to %d (cache miss)", initialRequests, requestCount)
		}
	})
}

func TestResolverStateScopingAndResolution(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock state endpoints
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		if strings.Contains(req.URL.Path, projUUID1) {
			body = `[
				{"id": "` + stateUUID1 + `", "name": "Todo", "group": "unstarted"},
				{"id": "` + stateUUID2 + `", "name": "In Progress", "group": "started"}
			]`
		} else if strings.Contains(req.URL.Path, projUUID2) {
			body = `[
				{"id": "` + stateUUID3 + `", "name": "Backlog", "group": "backlog"}
			]`
		} else {
			return &http.Response{StatusCode: 404, Body: io.NopCloser(strings.NewReader("not found"))}, nil
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	t.Run("Resolve Todo in Project A", func(t *testing.T) {
		s, err := resolver.ResolveState(context.Background(), projUUID1, "Todo")
		if err != nil {
			t.Fatalf("failed to resolve state: %v", err)
		}
		if s.ID != stateUUID1 {
			t.Errorf("expected ID %s, got '%s'", stateUUID1, s.ID)
		}
	})

	t.Run("Project Scoping", func(t *testing.T) {
		// "Todo" exists in Project A (projUUID1) but not Project B (projUUID2)
		_, err := resolver.ResolveState(context.Background(), projUUID2, "Todo")
		if err == nil {
			t.Fatal("expected error resolving state 'Todo' in Project B, got nil")
		}
		expectedErr := "state not found for project " + projUUID2 + ": Todo"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%v'", expectedErr, err)
		}
	})
}

func TestResolverLabelResolution(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		body := `[
			{"id": "` + labelUUID1 + `", "name": "type:bug", "color": "#ff0000"},
			{"id": "` + labelUUID2 + `", "name": "type:feature", "color": "#0000ff"}
		]`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	t.Run("Resolve Label by Name", func(t *testing.T) {
		l, err := resolver.ResolveLabel(context.Background(), projUUID1, "type:bug")
		if err != nil {
			t.Fatalf("failed to resolve label: %v", err)
		}
		if l.ID != labelUUID1 {
			t.Errorf("expected ID %s, got '%s'", labelUUID1, l.ID)
		}
	})
}

func TestResolverModuleResolution(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	requestCount := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		requestCount++
		body := `[
			{"id": "` + moduleUUID1 + `", "name": "Sprint One"},
			{"id": "` + moduleUUID2 + `", "name": "Sprint Two"}
		]`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	t.Run("Resolve Module by Name Case-Insensitive", func(t *testing.T) {
		m, err := resolver.ResolveModule(context.Background(), projUUID1, "sprint one")
		if err != nil {
			t.Fatalf("failed to resolve module: %v", err)
		}
		if m.ID != moduleUUID1 {
			t.Errorf("expected ID %s, got '%s'", moduleUUID1, m.ID)
		}
	})

	t.Run("Resolve Module by UUID", func(t *testing.T) {
		m, err := resolver.ResolveModule(context.Background(), projUUID1, moduleUUID2)
		if err != nil {
			t.Fatalf("failed to resolve module: %v", err)
		}
		if m.Name != "Sprint Two" {
			t.Errorf("expected name 'Sprint Two', got '%s'", m.Name)
		}
	})

	t.Run("Verify Cache Hit (No API call)", func(t *testing.T) {
		initialRequests := requestCount
		_, err := resolver.ResolveModule(context.Background(), projUUID1, "Sprint One")
		if err != nil {
			t.Fatalf("failed to resolve: %v", err)
		}
		if requestCount != initialRequests {
			t.Errorf("expected request count to remain %d, but increased to %d (cache miss)", initialRequests, requestCount)
		}
	})

	t.Run("Not Found returns error", func(t *testing.T) {
		_, err := resolver.ResolveModule(context.Background(), projUUID1, "Nonexistent Module")
		if err == nil {
			t.Fatal("expected error resolving unknown module, got nil")
		}
		expectedErr := "module not found for project " + projUUID1 + ": Nonexistent Module"
		if err.Error() != expectedErr {
			t.Errorf("expected error '%s', got '%v'", expectedErr, err)
		}
	})
}

func TestResolverMemberResolution(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		body := `[
			{"id": "` + userUUID1 + `", "first_name": "Figgy", "last_name": "Bot", "email": "figgy@example.com", "display_name": "FiggyBot"},
			{"id": "` + userUUID2 + `", "first_name": "Jenny", "last_name": "Figgy", "email": "jenny@example.com", "display_name": ""}
		]`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	t.Run("Resolve by DisplayName", func(t *testing.T) {
		m, err := resolver.ResolveMember(context.Background(), "figgybot")
		if err != nil {
			t.Fatalf("failed to resolve member: %v", err)
		}
		if m.ID != userUUID1 {
			t.Errorf("expected ID %s, got '%s'", userUUID1, m.ID)
		}
	})

	t.Run("Resolve by Full Name Fallback", func(t *testing.T) {
		m, err := resolver.ResolveMember(context.Background(), "jenny figgy")
		if err != nil {
			t.Fatalf("failed to resolve member: %v", err)
		}
		if m.ID != userUUID2 {
			t.Errorf("expected ID %s, got '%s'", userUUID2, m.ID)
		}
	})

	t.Run("Resolve by Email", func(t *testing.T) {
		m, err := resolver.ResolveMember(context.Background(), "figgy@example.com")
		if err != nil {
			t.Fatalf("failed to resolve member: %v", err)
		}
		if m.ID != userUUID1 {
			t.Errorf("expected ID %s, got '%s'", userUUID1, m.ID)
		}
	})
}

func TestResolveWorkItem(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock all requests for lookup
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		if strings.Contains(req.URL.Path, "projects") && !strings.Contains(req.URL.Path, "states") && !strings.Contains(req.URL.Path, "labels") {
			body = `[{"id": "` + projUUID1 + `", "name": "Agent Infra", "identifier": "AGENT"}]`
		} else if strings.Contains(req.URL.Path, "states") {
			body = `[{"id": "` + stateUUID1 + `", "name": "Todo", "group": "unstarted"}]`
		} else if strings.Contains(req.URL.Path, "labels") {
			body = `[{"id": "` + labelUUID1 + `", "name": "bug"}]`
		} else if strings.Contains(req.URL.Path, "members") {
			body = `[{"id": "` + userUUID1 + `", "first_name": "Figgy", "last_name": "Bot", "email": "figgy@example.com", "display_name": "FiggyBot"}]`
		} else {
			body = `[]`
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	t.Run("Resolve with String UUIDs", func(t *testing.T) {
		item := &WorkItem{
			ID:         "wi-1",
			Name:       "Test task",
			SequenceID: 101,
			Project:    Expandable[Project]{ID: projUUID1},
			State:      Expandable[State]{ID: stateUUID1},
			Assignees:  []Expandable[Member]{{ID: userUUID1}},
			Labels:     []Expandable[Label]{{ID: labelUUID1}},
		}

		resolved, err := resolver.ResolveWorkItem(context.Background(), item)
		if err != nil {
			t.Fatalf("failed to resolve work item: %v", err)
		}

		if resolved.ProjectName != "Agent Infra" || resolved.ProjectIdentifier != "AGENT" {
			t.Errorf("project resolution failed: %+v", resolved)
		}
		if resolved.StateName != "Todo" || resolved.StateGroup != "unstarted" {
			t.Errorf("state resolution failed: %+v", resolved)
		}
		if len(resolved.AssigneeNames) != 1 || resolved.AssigneeNames[0] != "FiggyBot" {
			t.Errorf("assignee resolution failed: %+v", resolved)
		}
		if len(resolved.LabelNames) != 1 || resolved.LabelNames[0] != "bug" {
			t.Errorf("label resolution failed: %+v", resolved)
		}
	})

	t.Run("Resolve with Already Expanded Values", func(t *testing.T) {
		item := &WorkItem{
			ID:         "wi-2",
			Name:       "Expanded task",
			SequenceID: 102,
			Project:    Expandable[Project]{Val: &Project{ID: projUUID2, Name: "Platform Work", Identifier: "PLAT"}},
			State:      Expandable[State]{Val: &State{ID: stateUUID3, Name: "Backlog", Group: "backlog"}},
			Assignees:  []Expandable[Member]{{Val: &Member{ID: userUUID2, FirstName: "Jenny", LastName: "Figgy", Email: "jenny@example.com"}}},
			Labels:     []Expandable[Label]{{Val: &Label{ID: labelUUID2, Name: "type:feature"}}},
		}

		resolved, err := resolver.ResolveWorkItem(context.Background(), item)
		if err != nil {
			t.Fatalf("failed to resolve: %v", err)
		}

		if resolved.ProjectName != "Platform Work" || resolved.ProjectIdentifier != "PLAT" {
			t.Errorf("project resolution failed: %+v", resolved)
		}
		if resolved.StateName != "Backlog" || resolved.StateGroup != "backlog" {
			t.Errorf("state resolution failed: %+v", resolved)
		}
		if len(resolved.AssigneeNames) != 1 || resolved.AssigneeNames[0] != "Jenny Figgy" {
			t.Errorf("assignee resolution failed: %+v", resolved)
		}
		if len(resolved.LabelNames) != 1 || resolved.LabelNames[0] != "type:feature" {
			t.Errorf("label resolution failed: %+v", resolved)
		}
	})
}

func TestResolverFailures(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock HTTP client to return errors
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader("server error")),
		}, nil
	})

	t.Run("Project Resolution Error", func(t *testing.T) {
		_, err := resolver.ResolveProject(context.Background(), "invalid-project")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("State Resolution Error", func(t *testing.T) {
		_, err := resolver.ResolveState(context.Background(), projUUID1, "invalid-state")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Label Resolution Error", func(t *testing.T) {
		_, err := resolver.ResolveLabel(context.Background(), projUUID1, "invalid-label")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Member Resolution Error", func(t *testing.T) {
		_, err := resolver.ResolveMember(context.Background(), "invalid-member")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("Empty Input Validation", func(t *testing.T) {
		if _, err := resolver.ResolveProject(context.Background(), ""); err == nil {
			t.Error("expected error on empty project name")
		}
		if _, err := resolver.ResolveState(context.Background(), projUUID1, ""); err == nil {
			t.Error("expected error on empty state name")
		}
		if _, err := resolver.ResolveLabel(context.Background(), projUUID1, ""); err == nil {
			t.Error("expected error on empty label name")
		}
		if _, err := resolver.ResolveMember(context.Background(), ""); err == nil {
			t.Error("expected error on empty member name")
		}
	})
}

func TestResolverGetCallerID(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("First call hits API and returns ID", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		resolver := NewResolver(client)
		requestCount := 0

		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			requestCount++
			expectedPath := "/api/v1/users/me/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			body := `{"id": "` + userUUID1 + `", "display_name": "Figgy Bot", "email": "figgy@example.com"}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		id, err := resolver.GetCallerID(context.Background())

		// Assert
		if err != nil {
			t.Fatalf("GetCallerID failed: %v", err)
		}
		if id != userUUID1 {
			t.Errorf("expected ID '%s', got '%s'", userUUID1, id)
		}
		if requestCount != 1 {
			t.Errorf("expected exactly 1 API request, got %d", requestCount)
		}
	})

	t.Run("Second call returns cached value without extra API request", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		resolver := NewResolver(client)
		requestCount := 0

		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			requestCount++
			body := `{"id": "` + userUUID1 + `", "display_name": "Figgy Bot", "email": "figgy@example.com"}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act: call twice
		id1, err := resolver.GetCallerID(context.Background())
		if err != nil {
			t.Fatalf("first GetCallerID failed: %v", err)
		}
		id2, err := resolver.GetCallerID(context.Background())
		if err != nil {
			t.Fatalf("second GetCallerID failed: %v", err)
		}

		// Assert: both return same ID and only one HTTP request was made
		if id1 != userUUID1 {
			t.Errorf("expected id1 '%s', got '%s'", userUUID1, id1)
		}
		if id2 != userUUID1 {
			t.Errorf("expected id2 '%s', got '%s'", userUUID1, id2)
		}
		if requestCount != 1 {
			t.Errorf("expected exactly 1 API request (cache hit on second call), got %d", requestCount)
		}
	})

	t.Run("API error propagates", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		resolver := NewResolver(client)

		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
			}, nil
		})

		// Act
		_, err := resolver.GetCallerID(context.Background())

		// Assert
		if err == nil {
			t.Fatal("expected error on 401, got nil")
		}
		if !strings.Contains(err.Error(), "failed to identify caller") {
			t.Errorf("expected error to mention 'failed to identify caller', got: %v", err)
		}
	})
}

func TestResolveWorkItemFallback(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock all requests to fail (force fallbacks)
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 404,
			Body:       io.NopCloser(strings.NewReader("not found")),
		}, nil
	})

	item := &WorkItem{
		ID:         "wi-fallback",
		Name:       "Fallback task",
		SequenceID: 99,
		Project:    Expandable[Project]{ID: projUUID1},
		State:      Expandable[State]{ID: stateUUID1},
		Assignees:  []Expandable[Member]{{ID: userUUID1}},
		Labels:     []Expandable[Label]{{ID: labelUUID1}},
	}

	resolved, err := resolver.ResolveWorkItem(context.Background(), item)
	if err != nil {
		t.Fatalf("ResolveWorkItem should succeed even if lookup fails, got err: %v", err)
	}

	// Verify it fell back to UUID strings correctly
	if resolved.ProjectID != projUUID1 || resolved.ProjectName != "" {
		t.Errorf("unexpected project fallback: %+v", resolved)
	}
	if resolved.StateID != stateUUID1 || resolved.StateName != "" {
		t.Errorf("unexpected state fallback: %+v", resolved)
	}
	if len(resolved.AssigneeNames) != 1 || resolved.AssigneeNames[0] != userUUID1 {
		t.Errorf("unexpected assignee fallback: %+v", resolved)
	}
	if len(resolved.LabelNames) != 1 || resolved.LabelNames[0] != labelUUID1 {
		t.Errorf("unexpected label fallback: %+v", resolved)
	}
}
