package plane

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
)

// Mock RoundTripper for unit testing without network binding
type mockTransport func(req *http.Request) (*http.Response, error)

func (m mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return m(req)
}

func TestExpandableUnmarshalJSON(t *testing.T) {
	t.Run("UUID String", func(t *testing.T) {
		jsonData := []byte(`"d2df70f9-a821-4866-bac4-dcab37696902"`)
		var exp Expandable[Label]
		err := json.Unmarshal(jsonData, &exp)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if exp.ID != "d2df70f9-a821-4866-bac4-dcab37696902" {
			t.Errorf("expected ID 'd2df70f9-a821-4866-bac4-dcab37696902', got '%s'", exp.ID)
		}
		if exp.Val != nil {
			t.Errorf("expected Val to be nil, got: %+v", exp.Val)
		}
	})

	t.Run("Full Object", func(t *testing.T) {
		jsonData := []byte(`{"id": "d2df70f9-a821-4866-bac4-dcab37696902", "name": "bug", "color": "#ef4444"}`)
		var exp Expandable[Label]
		err := json.Unmarshal(jsonData, &exp)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if exp.ID != "d2df70f9-a821-4866-bac4-dcab37696902" {
			t.Errorf("expected ID 'd2df70f9-a821-4866-bac4-dcab37696902', got '%s'", exp.ID)
		}
		if exp.Val == nil {
			t.Fatal("expected Val to be non-nil")
		}
		if exp.Val.Name != "bug" || exp.Val.Color != "#ef4444" {
			t.Errorf("unexpected field values: %+v", exp.Val)
		}
	})

	t.Run("JSON Null", func(t *testing.T) {
		jsonData := []byte(`null`)
		var exp Expandable[Label]
		err := json.Unmarshal(jsonData, &exp)
		if err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if exp.ID != "" {
			t.Errorf("expected empty ID, got '%s'", exp.ID)
		}
		if exp.Val != nil {
			t.Errorf("expected Val to be nil, got %+v", exp.Val)
		}
	})
}

func TestExpandableMarshalJSON(t *testing.T) {
	t.Run("UUID String", func(t *testing.T) {
		exp := Expandable[Label]{
			ID:  "some-uuid",
			Val: nil,
		}
		data, err := json.Marshal(exp)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		expected := `"some-uuid"`
		if string(data) != expected {
			t.Errorf("expected '%s', got '%s'", expected, string(data))
		}
	})

	t.Run("Full Object", func(t *testing.T) {
		label := &Label{
			ID:    "some-uuid",
			Name:  "chore",
			Color: "#ff0000",
		}
		exp := Expandable[Label]{
			ID:  "some-uuid",
			Val: label,
		}
		data, err := json.Marshal(exp)
		if err != nil {
			t.Fatalf("failed to marshal: %v", err)
		}
		var decoded Label
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("failed to unmarshal result: %v", err)
		}
		if decoded.ID != "some-uuid" || decoded.Name != "chore" {
			t.Errorf("unexpected marshalled content: %s", string(data))
		}
	})
}

func TestClientAuthAndCFHeaders(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:          "test-key",
		PlaneBaseURL:         "https://plane.example.com/",
		PlaneWorkspaceSlug:   "test-workspace",
		CFAccessClientID:     "cf-id",
		CFAccessClientSecret: "cf-secret",
	}

	client := NewClient(cfg)

	// Mock transport to verify headers
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		// Verify standard Plane API headers
		if req.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("expected X-API-Key header to be 'test-key', got '%s'", req.Header.Get("X-API-Key"))
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got '%s'", req.Header.Get("Content-Type"))
		}

		// Verify Cloudflare Access headers
		if req.Header.Get("CF-Access-Client-Id") != "cf-id" {
			t.Errorf("expected CF-Access-Client-Id 'cf-id', got '%s'", req.Header.Get("CF-Access-Client-Id"))
		}
		if req.Header.Get("CF-Access-Client-Secret") != "cf-secret" {
			t.Errorf("expected CF-Access-Client-Secret 'cf-secret', got '%s'", req.Header.Get("CF-Access-Client-Secret"))
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(`[]`)),
		}, nil
	})

	_, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
}

func TestClientPagination(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	client := NewClient(cfg)
	requestCount := 0

	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		requestCount++

		var body string
		switch requestCount {
		case 1:
			// First page: return next_cursor and 2 items
			if req.URL.Query().Get("cursor") != "" {
				t.Errorf("did not expect cursor on first page, got '%s'", req.URL.Query().Get("cursor"))
			}
			body = `{
				"results": [
					{"id": "p1", "name": "Project 1", "identifier": "P1"},
					{"id": "p2", "name": "Project 2", "identifier": "P2"}
				],
				"next_cursor": "page-2-cursor",
				"next_page_results": true
			}`
		case 2:
			// Second page: return final 1 item, no next_cursor
			if req.URL.Query().Get("cursor") != "page-2-cursor" {
				t.Errorf("expected cursor 'page-2-cursor', got '%s'", req.URL.Query().Get("cursor"))
			}
			body = `{
				"results": [
					{"id": "p3", "name": "Project 3", "identifier": "P3"}
				],
				"next_cursor": "",
				"next_page_results": false
			}`
		default:
			t.Errorf("unexpected request count: %d", requestCount)
		}

		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	projects, err := client.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}

	if len(projects) != 3 {
		t.Errorf("expected 3 projects, got %d", len(projects))
	}
	if projects[0].ID != "p1" || projects[1].ID != "p2" || projects[2].ID != "p3" {
		t.Errorf("unexpected project details: %+v", projects)
	}
}

func TestClientPaginationRejectsRepeatedCursor(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	requestCount := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		requestCount++
		if requestCount > 2 {
			return nil, errors.New("pagination loop sentinel")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"results": [],
				"next_cursor": "repeat-cursor",
				"next_page_results": true
			}`)),
		}, nil
	})

	// Act
	_, err := client.ListProjects(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected repeated pagination cursor error")
	}
	if !strings.Contains(err.Error(), "repeated pagination cursor") {
		t.Fatalf("expected repeated cursor error, got %v", err)
	}
	if requestCount != 2 {
		t.Errorf("expected repeated cursor to stop after detecting the second page, got %d requests", requestCount)
	}
}

func TestClientPaginationRejectsExcessivePages(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	requestCount := 0
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		requestCount++
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"results": [], "next_cursor": "cursor-` + strconv.Itoa(requestCount) + `", "next_page_results": true}`)),
		}, nil
	})

	// Act
	_, err := client.ListProjects(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected maximum pagination page error")
	}
	if !strings.Contains(err.Error(), "maximum page count") {
		t.Fatalf("expected maximum page count error, got %v", err)
	}
	if requestCount != maxPaginationPages {
		t.Errorf("expected %d requests before the page guard, got %d", maxPaginationPages, requestCount)
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "404", err: &APIError{StatusCode: http.StatusNotFound}, want: true},
		{name: "other status", err: &APIError{StatusCode: http.StatusBadGateway}, want: false},
		{name: "nil", err: nil, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsNotFoundError(tc.err); got != tc.want {
				t.Errorf("IsNotFoundError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestClientRawArrayFallback(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	client := NewClient(cfg)

	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		// Return raw array directly without paginated envelope
		body := `[
			{"id": "s1", "name": "Todo", "group": "unstarted", "color": "#000"},
			{"id": "s2", "name": "Done", "group": "completed", "color": "#fff"}
		]`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	states, err := client.ListStates(context.Background(), "project-id")
	if err != nil {
		t.Fatalf("ListStates failed: %v", err)
	}

	if len(states) != 2 {
		t.Errorf("expected 2 states, got %d", len(states))
	}
	if states[0].Name != "Todo" || states[1].Name != "Done" {
		t.Errorf("unexpected state details: %+v", states)
	}
}

func TestClientGetWorkItemByIdentifier(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)

	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		expectedPath := "/api/v1/workspaces/test-workspace/work-items/AGENT-8/"
		if req.URL.Path != expectedPath {
			t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
		}
		body := `{"id": "wi-8", "name": "REST Client", "sequence_id": 8}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	item, err := client.GetWorkItemByIdentifier(context.Background(), "AGENT", 8)
	if err != nil {
		t.Fatalf("GetWorkItemByIdentifier failed: %v", err)
	}
	if item.ID != "wi-8" || item.Name != "REST Client" || item.SequenceID != 8 {
		t.Errorf("unexpected item returned: %+v", item)
	}
}

func TestClientRequestErrors(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("HTTP Error Status", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(strings.NewReader("internal server error")),
			}, nil
		})
		_, err := client.ListProjects(context.Background())
		if err == nil {
			t.Fatal("expected error on 500 status, got nil")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected error message to mention 500, got: %v", err)
		}
	})

	t.Run("JSON Marshal Request Body Error", func(t *testing.T) {
		client := NewClient(cfg)
		// Try to send a channel, which cannot be marshalled to JSON
		unmarshalable := make(chan int)
		err := client.request(context.Background(), "POST", "/test", nil, unmarshalable, nil)
		if err == nil {
			t.Fatal("expected error when marshalling unmarshalable body, got nil")
		}
	})

	t.Run("Invalid URL Path", func(t *testing.T) {
		client := NewClient(cfg)
		err := client.request(context.Background(), "GET", "%%invalid path", nil, nil, nil)
		if err == nil {
			t.Fatal("expected error with invalid path, got nil")
		}
	})
}

func TestClientGetMe(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns Member", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/users/me/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "GET" {
				t.Errorf("expected GET, got %s", req.Method)
			}
			body := `{"id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa", "display_name": "Figgy Bot", "email": "figgy@example.com"}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		me, err := client.GetMe(context.Background())

		// Assert
		if err != nil {
			t.Fatalf("GetMe failed: %v", err)
		}
		if me.ID != "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa" {
			t.Errorf("expected ID 'aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa', got '%s'", me.ID)
		}
		if me.DisplayName != "Figgy Bot" {
			t.Errorf("expected DisplayName 'Figgy Bot', got '%s'", me.DisplayName)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 401,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
			}, nil
		})

		// Act
		_, err := client.GetMe(context.Background())

		// Assert
		if err == nil {
			t.Fatal("expected error on 401, got nil")
		}
		if !strings.Contains(err.Error(), "401") {
			t.Errorf("expected error message to mention 401, got: %v", err)
		}
	})
}

func TestClientListWorkItems(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns work items", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			// Verify filter params are forwarded
			if req.URL.Query().Get("state_group") != "started" {
				t.Errorf("expected state_group=started, got '%s'", req.URL.Query().Get("state_group"))
			}
			body := `[
				{"id": "wi-1", "name": "Task One", "sequence_id": 1},
				{"id": "wi-2", "name": "Task Two", "sequence_id": 2}
			]`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		items, err := client.ListWorkItems(context.Background(), "proj-1", map[string]string{
			"state_group": "started",
		})

		// Assert
		if err != nil {
			t.Fatalf("ListWorkItems failed: %v", err)
		}
		if len(items) != 2 {
			t.Errorf("expected 2 items, got %d", len(items))
		}
		if items[0].ID != "wi-1" || items[1].ID != "wi-2" {
			t.Errorf("unexpected items: %+v", items)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("project not found")),
			}, nil
		})

		// Act
		_, err := client.ListWorkItems(context.Background(), "proj-missing", nil)

		// Assert
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error message to mention 404, got: %v", err)
		}
	})
}

func TestClientListIntakeWorkItemsUsesExpandedIssueData(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/api/v1/workspaces/test-workspace/projects/project-1/intake-issues/" {
			t.Errorf("unexpected intake list path: %s", req.URL.Path)
		}
		if req.URL.Query().Get("expand") != "issue" {
			t.Errorf("expected expand=issue, got %q", req.URL.Query().Get("expand"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"results": [{
					"id": "intake-1",
					"issue": {
						"id": "issue-1",
						"name": "Incoming request",
						"sequence_id": 10,
						"priority": "high"
					},
					"status": -2,
					"snoozed_till": null,
					"duplicate_to": null
				}],
				"next_cursor": "",
				"next_page_results": false
			}`)),
		}, nil
	})

	// Act
	items, err := client.ListIntakeWorkItems(context.Background(), "project-1")

	// Assert
	if err != nil {
		t.Fatalf("ListIntakeWorkItems failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one intake item, got %d", len(items))
	}
	issue := items[0].UnderlyingIssue()
	if issue == nil || issue.ID != "issue-1" || issue.SequenceID != 10 {
		t.Fatalf("unexpected expanded issue: %+v", issue)
	}
	if items[0].Status != -2 {
		t.Errorf("expected pending status -2, got %d", items[0].Status)
	}
}

func TestClientGetIntakeWorkItemUsesUnderlyingIssueID(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		expectedPath := "/api/v1/workspaces/test-workspace/projects/project-1/intake-issues/issue-1/"
		if req.URL.Path != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, req.URL.Path)
		}
		if req.URL.Query().Get("expand") != "issue" {
			t.Errorf("expected expand=issue, got %q", req.URL.Query().Get("expand"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"id": "intake-1",
				"issue": {"id": "issue-1", "name": "Incoming request", "sequence_id": 10},
				"status": 1
			}`)),
		}, nil
	})

	// Act
	item, err := client.GetIntakeWorkItem(context.Background(), "project-1", "issue-1")

	// Assert
	if err != nil {
		t.Fatalf("GetIntakeWorkItem failed: %v", err)
	}
	if item.ID != "intake-1" || item.UnderlyingIssue().ID != "issue-1" {
		t.Fatalf("unexpected intake item: %+v", item)
	}
}

func TestEnrichmentNotAppliedErrorMessage(t *testing.T) {
	err := &EnrichmentNotAppliedError{
		Identifier:    "ASBX-10",
		IssueUUID:     "issue-10",
		IgnoredFields: []string{"name", "priority"},
		Causes:        []string{"cause one", "cause two"},
	}
	want := "enrichment_not_applied: intake record ASBX-10 does not reflect the requested update (HTTP 2xx but Plane did not store: name, priority); likely causes: cause one; cause two"
	if err.Error() != want {
		t.Errorf("Error() = %q, want %q", err.Error(), want)
	}

	statusErr := &EnrichmentNotAppliedError{
		Identifier:    "ASBX-11",
		IssueUUID:     "issue-11",
		StatusChanged: true,
		Causes:        []string{"status drifted"},
	}
	if !strings.Contains(statusErr.Error(), "enrichment_not_applied: intake record ASBX-11") {
		t.Errorf("status-change variant missing header: %q", statusErr.Error())
	}
}

func TestClientCreateIntakeWorkItemNestsIssueFields(t *testing.T) {
	// Arrange
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		expectedPath := "/api/v1/workspaces/test-workspace/projects/project-1/intake-issues/"
		if req.URL.Path != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, req.URL.Path)
		}
		if req.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", req.Method)
		}
		var payload struct {
			Issue map[string]any `json:"issue"`
		}
		if err := json.NewDecoder(req.Body).Decode(&payload); err != nil {
			t.Fatalf("failed to decode request body: %v", err)
		}
		if payload.Issue["name"] != "Quick idea" || payload.Issue["priority"] != "high" {
			t.Errorf("unexpected nested issue fields: %+v", payload.Issue)
		}
		return &http.Response{
			StatusCode: http.StatusCreated,
			Body: io.NopCloser(strings.NewReader(`{
				"id": "intake-9",
				"issue": {"id": "issue-9", "name": "Quick idea", "sequence_id": 9},
				"status": -2,
				"source": "IN_APP"
			}`)),
		}, nil
	})

	// Act
	item, err := client.CreateIntakeWorkItem(context.Background(), "project-1", map[string]any{
		"name":     "Quick idea",
		"priority": "high",
	})

	// Assert
	if err != nil {
		t.Fatalf("CreateIntakeWorkItem failed: %v", err)
	}
	if item.ID != "intake-9" || item.Status != -2 || item.Source != "IN_APP" {
		t.Errorf("unexpected intake record: %+v", item)
	}
	if item.UnderlyingIssueID() != "issue-9" {
		t.Errorf("expected underlying issue UUID issue-9, got %q", item.UnderlyingIssueID())
	}
}

func TestIntakeWorkItemUsesIssueDetailWhenIssueIsOnlyAnID(t *testing.T) {
	// Arrange
	var item IntakeWorkItem
	err := json.Unmarshal([]byte(`{
		"id": "intake-1",
		"issue": "issue-1",
		"issue_detail": {"id": "issue-1", "name": "Expanded request", "sequence_id": 10},
		"status": -1
	}`), &item)

	// Assert
	if err != nil {
		t.Fatalf("failed to decode intake item: %v", err)
	}
	issue := item.UnderlyingIssue()
	if issue == nil || issue.Name != "Expanded request" || issue.SequenceID != 10 {
		t.Fatalf("expected issue_detail fallback, got %+v", issue)
	}
	if item.UnderlyingIssueID() != "issue-1" || IntakeStatusName(item.Status) != "declined" {
		t.Errorf("unexpected intake metadata: issue=%q status=%q", item.UnderlyingIssueID(), IntakeStatusName(item.Status))
	}
}

func TestClientCreateWorkItem(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path creates and returns work item", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}

			// Verify request body
			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if reqBody["name"] != "New Task" {
				t.Errorf("expected body name='New Task', got: %v", reqBody["name"])
			}

			body := `{"id": "wi-new", "name": "New Task", "sequence_id": 42}`
			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		item, err := client.CreateWorkItem(context.Background(), "proj-1", map[string]any{
			"name": "New Task",
		})

		// Assert
		if err != nil {
			t.Fatalf("CreateWorkItem failed: %v", err)
		}
		if item.ID != "wi-new" || item.Name != "New Task" {
			t.Errorf("unexpected item: %+v", item)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader("bad request")),
			}, nil
		})

		// Act
		_, err := client.CreateWorkItem(context.Background(), "proj-1", map[string]any{})

		// Assert
		if err == nil {
			t.Fatal("expected error on 400, got nil")
		}
		if !strings.Contains(err.Error(), "400") {
			t.Errorf("expected error message to mention 400, got: %v", err)
		}
	})
}

func TestClientUpdateWorkItem(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path updates and returns work item", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-42/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "PATCH" {
				t.Errorf("expected PATCH, got %s", req.Method)
			}

			// Verify request body
			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if reqBody["priority"] != "high" {
				t.Errorf("expected body priority='high', got: %v", reqBody["priority"])
			}

			body := `{"id": "wi-42", "name": "Existing Task", "priority": "high", "sequence_id": 42}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		item, err := client.UpdateWorkItem(context.Background(), "proj-1", "wi-42", map[string]any{
			"priority": "high",
		})

		// Assert
		if err != nil {
			t.Fatalf("UpdateWorkItem failed: %v", err)
		}
		if item.ID != "wi-42" || item.Priority != "high" {
			t.Errorf("unexpected item: %+v", item)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		})

		// Act
		_, err := client.UpdateWorkItem(context.Background(), "proj-1", "wi-missing", map[string]any{})

		// Assert
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error message to mention 404, got: %v", err)
		}
	})
}

func TestClientCreateWorkItemComment(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path sends comment with HTML wrapping", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-42/comments/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}

			// Verify request body has comment_html wrapped in <p> tags
			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			expectedHTML := "<p>Hello world</p>"
			if reqBody["comment_html"] != expectedHTML {
				t.Errorf("expected comment_html='%s', got: %v", expectedHTML, reqBody["comment_html"])
			}

			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(`{"id": "comment-1"}`)),
			}, nil
		})

		// Act
		err := client.CreateWorkItemComment(context.Background(), "proj-1", "wi-42", "Hello world")

		// Assert
		if err != nil {
			t.Fatalf("CreateWorkItemComment failed: %v", err)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 403,
				Body:       io.NopCloser(strings.NewReader("forbidden")),
			}, nil
		})

		// Act
		err := client.CreateWorkItemComment(context.Background(), "proj-1", "wi-42", "Should fail")

		// Assert
		if err == nil {
			t.Fatal("expected error on 403, got nil")
		}
		if !strings.Contains(err.Error(), "403") {
			t.Errorf("expected error message to mention 403, got: %v", err)
		}
	})

	t.Run("Non-HTML text starting with < gets wrapped in <p>", func(t *testing.T) {
		testCases := []struct {
			name         string
			text         string
			expectedHTML string
		}{
			{"arrow text", "<- check this out", "<p><- check this out</p>"},
			{"heart emoticon", "<3 text", "<p><3 text</p>"},
			{"just less-than sign", "< not HTML", "<p>< not HTML</p>"},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				client := NewClient(cfg)
				client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
					var reqBody map[string]any
					bodyBytes, _ := io.ReadAll(req.Body)
					if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
						t.Fatalf("failed to parse request body: %v", err)
					}
					if reqBody["comment_html"] != tc.expectedHTML {
						t.Errorf("expected comment_html=%q, got %q", tc.expectedHTML, reqBody["comment_html"])
					}
					return &http.Response{
						StatusCode: 201,
						Body:       io.NopCloser(strings.NewReader(`{"id": "comment-1"}`)),
					}, nil
				})
				err := client.CreateWorkItemComment(context.Background(), "proj-1", "wi-42", tc.text)
				if err != nil {
					t.Fatalf("CreateWorkItemComment failed: %v", err)
				}
			})
		}
	})

	t.Run("Real HTML is NOT wrapped", func(t *testing.T) {
		testCases := []struct {
			name string
			text string
		}{
			{"paragraph", "<p>test</p>"},
			{"div", "<div>test</div>"},
			{"html comment", "<!-- comment -->"},
			{"self-closing tag", "<br/>"},
		}
		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				client := NewClient(cfg)
				client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
					var reqBody map[string]any
					bodyBytes, _ := io.ReadAll(req.Body)
					if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
						t.Fatalf("failed to parse request body: %v", err)
					}
					if reqBody["comment_html"] != tc.text {
						t.Errorf("expected comment_html=%q (unchanged), got %q", tc.text, reqBody["comment_html"])
					}
					return &http.Response{
						StatusCode: 201,
						Body:       io.NopCloser(strings.NewReader(`{"id": "comment-1"}`)),
					}, nil
				})
				err := client.CreateWorkItemComment(context.Background(), "proj-1", "wi-42", tc.text)
				if err != nil {
					t.Fatalf("CreateWorkItemComment failed: %v", err)
				}
			})
		}
	})
}

func TestClientCreateWorkItemLink(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path sends link with url and title", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-42/links/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}

			// Verify request body contains expected url and title fields
			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if reqBody["url"] != "https://example.com/docs" {
				t.Errorf("expected url='https://example.com/docs', got: %v", reqBody["url"])
			}
			if reqBody["title"] != "Reference Docs" {
				t.Errorf("expected title='Reference Docs', got: %v", reqBody["title"])
			}

			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(`{"id": "link-1"}`)),
			}, nil
		})

		// Act
		err := client.CreateWorkItemLink(context.Background(), "proj-1", "wi-42", "https://example.com/docs", "Reference Docs")

		// Assert
		if err != nil {
			t.Fatalf("CreateWorkItemLink failed: %v", err)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 422,
				Body:       io.NopCloser(strings.NewReader("unprocessable entity")),
			}, nil
		})

		// Act
		err := client.CreateWorkItemLink(context.Background(), "proj-1", "wi-42", "not-a-url", "Bad Link")

		// Assert
		if err == nil {
			t.Fatal("expected error on 422, got nil")
		}
		if !strings.Contains(err.Error(), "422") {
			t.Errorf("expected error message to mention 422, got: %v", err)
		}
	})
}

func TestClientListModules(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns modules", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/modules/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			body := `[
				{"id": "mod-1", "name": "Sprint One"},
				{"id": "mod-2", "name": "Sprint Two"}
			]`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		modules, err := client.ListModules(context.Background(), "proj-1")

		// Assert
		if err != nil {
			t.Fatalf("ListModules failed: %v", err)
		}
		if len(modules) != 2 {
			t.Errorf("expected 2 modules, got %d", len(modules))
		}
		if modules[0].ID != "mod-1" || modules[0].Name != "Sprint One" {
			t.Errorf("unexpected module: %+v", modules[0])
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("project not found")),
			}, nil
		})

		// Act
		_, err := client.ListModules(context.Background(), "proj-missing")

		// Assert
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error message to mention 404, got: %v", err)
		}
	})
}

func TestClientAddWorkItemsToModule(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path posts issues array", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/modules/mod-1/module-issues/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}

			// Verify request body carries the issues array
			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			issues, ok := reqBody["issues"].([]any)
			if !ok {
				t.Fatalf("expected issues to be an array, got: %v", reqBody["issues"])
			}
			if len(issues) != 1 || issues[0] != "wi-42" {
				t.Errorf("expected issues=[wi-42], got: %v", issues)
			}

			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		})

		// Act
		err := client.AddWorkItemsToModule(context.Background(), "proj-1", "mod-1", []string{"wi-42"})

		// Assert
		if err != nil {
			t.Fatalf("AddWorkItemsToModule failed: %v", err)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader("bad request")),
			}, nil
		})

		// Act
		err := client.AddWorkItemsToModule(context.Background(), "proj-1", "mod-1", []string{"wi-42"})

		// Assert
		if err == nil {
			t.Fatal("expected error on 400, got nil")
		}
		if !strings.Contains(err.Error(), "400") {
			t.Errorf("expected error message to mention 400, got: %v", err)
		}
	})

	t.Run("Empty workItemIDs slice posts empty array not null", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			// Assert request path and method
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/modules/mod-1/module-issues/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}

			// Assert body: "issues" must be an empty array (not null / missing)
			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			rawIssues, exists := reqBody["issues"]
			if !exists {
				t.Fatal("expected 'issues' key in request body, but it was absent")
			}
			issues, ok := rawIssues.([]any)
			if !ok {
				t.Fatalf("expected 'issues' to be a JSON array, got %T: %v", rawIssues, rawIssues)
			}
			if len(issues) != 0 {
				t.Errorf("expected empty issues array, got length %d: %v", len(issues), issues)
			}

			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		})

		// Act
		err := client.AddWorkItemsToModule(context.Background(), "proj-1", "mod-1", []string{})

		// Assert
		if err != nil {
			t.Fatalf("AddWorkItemsToModule with empty slice failed: %v", err)
		}
	})
}

func TestClientSearchWorkItems(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns search results", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/work-items/search/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			// Verify search params are forwarded
			if req.URL.Query().Get("search") != "login bug" {
				t.Errorf("expected search=login bug, got '%s'", req.URL.Query().Get("search"))
			}
			if req.URL.Query().Get("project_id") != "proj-1" {
				t.Errorf("expected project_id=proj-1, got '%s'", req.URL.Query().Get("project_id"))
			}
			body := `{"issues": [{"id": "wi-1", "name": "Fix login", "sequence_id": 1, "project__identifier": "PROJ", "project_id": "proj-1", "workspace__slug": "test-workspace"}, {"id": "wi-2", "name": "Login error", "sequence_id": 2, "project__identifier": "PROJ", "project_id": "proj-1", "workspace__slug": "test-workspace"}]}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		results, err := client.SearchWorkItems(context.Background(), map[string]string{
			"search":     "login bug",
			"project_id": "proj-1",
		})

		// Assert
		if err != nil {
			t.Fatalf("SearchWorkItems failed: %v", err)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
		if results[0].ID != "wi-1" || results[0].Name != "Fix login" || results[0].SequenceID != 1 {
			t.Errorf("unexpected result[0]: %+v", results[0])
		}
		if results[0].ProjectIdentifier != "PROJ" {
			t.Errorf("expected ProjectIdentifier='PROJ', got '%s'", results[0].ProjectIdentifier)
		}
		if results[1].ID != "wi-2" {
			t.Errorf("unexpected result[1]: %+v", results[1])
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(strings.NewReader("server error")),
			}, nil
		})

		// Act
		_, err := client.SearchWorkItems(context.Background(), map[string]string{
			"search": "anything",
		})

		// Assert
		if err == nil {
			t.Fatal("expected error on 500, got nil")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected error message to mention 500, got: %v", err)
		}
	})
}

func TestClientListComments(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path parses comments response", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-42/comments/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "GET" {
				t.Errorf("expected GET, got %s", req.Method)
			}

			body := `{
				"results": [
					{
						"id": "comment-1",
						"created_at": "2025-06-01T10:00:00Z",
						"comment_html": "<p>Hello <strong>world</strong></p>",
						"actor_detail": {
							"id": "user-1",
							"display_name": "Alice",
							"first_name": "Alice",
							"last_name": "Smith"
						}
					},
					{
						"id": "comment-2",
						"created_at": "2025-06-01T11:00:00Z",
						"comment_html": "<p>Second comment</p>",
						"actor_detail": {
							"id": "user-2",
							"display_name": "",
							"first_name": "Bob",
							"last_name": "Jones"
						}
					}
				],
				"next_cursor": "",
				"next_page_results": false
			}`

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		comments, err := client.ListComments(context.Background(), "proj-1", "wi-42")

		// Assert
		if err != nil {
			t.Fatalf("ListComments failed: %v", err)
		}
		if len(comments) != 2 {
			t.Fatalf("expected 2 comments, got %d", len(comments))
		}
		if comments[0].ID != "comment-1" {
			t.Errorf("expected ID 'comment-1', got '%s'", comments[0].ID)
		}
		if comments[0].CommentHTML != "<p>Hello <strong>world</strong></p>" {
			t.Errorf("unexpected comment_html: %s", comments[0].CommentHTML)
		}
		if comments[0].ActorDetail.DisplayName != "Alice" {
			t.Errorf("expected actor display_name 'Alice', got '%s'", comments[0].ActorDetail.DisplayName)
		}
		if comments[1].ID != "comment-2" {
			t.Errorf("expected ID 'comment-2', got '%s'", comments[1].ID)
		}
		if comments[1].ActorDetail.FirstName != "Bob" || comments[1].ActorDetail.LastName != "Jones" {
			t.Errorf("unexpected actor detail: %+v", comments[1].ActorDetail)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		})

		// Act
		_, err := client.ListComments(context.Background(), "proj-1", "wi-42")

		// Assert
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error message to mention 404, got: %v", err)
		}
	})
}

func TestClientGetLastComment(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns latest comment", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-42/comments/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "GET" {
				t.Errorf("expected GET, got %s", req.Method)
			}
			if req.URL.Query().Get("per_page") != "1" {
				t.Errorf("expected per_page=1, got '%s'", req.URL.Query().Get("per_page"))
			}
			if req.URL.Query().Get("order_by") != "-created_at" {
				t.Errorf("expected order_by=-created_at, got '%s'", req.URL.Query().Get("order_by"))
			}

			body := `{
				"results": [
					{
						"id": "comment-1",
						"created_at": "2026-06-15T18:00:00Z",
						"comment_html": "<p>This is the latest comment</p>",
						"actor_detail": {
							"id": "user-1",
							"display_name": "Jane Doe",
							"first_name": "Jane",
							"last_name": "Doe"
						}
					}
				],
				"next_cursor": "",
				"next_page_results": false
			}`

			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		comment, err := client.GetLastComment(context.Background(), "proj-1", "wi-42")

		// Assert
		if err != nil {
			t.Fatalf("GetLastComment failed: %v", err)
		}
		if comment == nil {
			t.Fatal("expected comment, got nil")
		}
		if comment.ID != "comment-1" || comment.CommentHTML != "<p>This is the latest comment</p>" || comment.CreatedAt != "2026-06-15T18:00:00Z" {
			t.Errorf("unexpected comment content: %+v", comment)
		}
		if comment.ActorDetail.ID != "user-1" || comment.ActorDetail.DisplayName != "Jane Doe" || comment.ActorDetail.FirstName != "Jane" || comment.ActorDetail.LastName != "Doe" {
			t.Errorf("unexpected actor detail: %+v", comment.ActorDetail)
		}
	})

	t.Run("Empty path returns nil, nil", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			body := `{"results": []}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		// Act
		comment, err := client.GetLastComment(context.Background(), "proj-1", "wi-42")

		// Assert
		if err != nil {
			t.Fatalf("GetLastComment failed: %v", err)
		}
		if comment != nil {
			t.Errorf("expected nil comment, got %+v", comment)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		// Arrange
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 500,
				Body:       io.NopCloser(strings.NewReader("server error")),
			}, nil
		})

		// Act
		_, err := client.GetLastComment(context.Background(), "proj-1", "wi-42")

		// Assert
		if err == nil {
			t.Fatal("expected error on 500, got nil")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("expected error message to mention 500, got: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// Relation management client tests
// ---------------------------------------------------------------------------

func TestClientGetWorkItem(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns work item", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-42/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "GET" {
				t.Errorf("expected GET, got %s", req.Method)
			}
			body := `{"id": "wi-42", "name": "Test Item", "sequence_id": 42}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		item, err := client.GetWorkItem(context.Background(), "proj-1", "wi-42")
		if err != nil {
			t.Fatalf("GetWorkItem failed: %v", err)
		}
		if item.ID != "wi-42" || item.Name != "Test Item" || item.SequenceID != 42 {
			t.Errorf("unexpected item: %+v", item)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		})

		_, err := client.GetWorkItem(context.Background(), "proj-1", "wi-missing")
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error message to mention 404, got: %v", err)
		}
	})
}

func TestClientListWorkItemRelations(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path returns relations", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-1/relations/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "GET" {
				t.Errorf("expected GET, got %s", req.Method)
			}
			body := `{
				"blocking": [{"project_id": "proj-uuid", "issue_id": "wi-2"}],
				"blocked_by": [],
				"duplicate": [],
				"relates_to": [{"project_id": "proj-uuid", "issue_id": "wi-3"}],
				"start_after": [],
				"start_before": [],
				"finish_after": [],
				"finish_before": []
			}`
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(body)),
			}, nil
		})

		relations, err := client.ListWorkItemRelations(context.Background(), "proj-1", "wi-1")
		if err != nil {
			t.Fatalf("ListWorkItemRelations failed: %v", err)
		}
		if len(relations.Blocking) != 1 || relations.Blocking[0].IssueID != "wi-2" {
			t.Errorf("unexpected blocking relations: %+v", relations.Blocking)
		}
		if len(relations.RelatesTo) != 1 || relations.RelatesTo[0].IssueID != "wi-3" {
			t.Errorf("unexpected relates_to relations: %+v", relations.RelatesTo)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 404,
				Body:       io.NopCloser(strings.NewReader("not found")),
			}, nil
		})

		_, err := client.ListWorkItemRelations(context.Background(), "proj-1", "wi-missing")
		if err == nil {
			t.Fatal("expected error on 404, got nil")
		}
		if !strings.Contains(err.Error(), "404") {
			t.Errorf("expected error message to mention 404, got: %v", err)
		}
	})
}

func TestClientCreateWorkItemRelation(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}

	t.Run("Happy path posts relation", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			expectedPath := "/api/v1/workspaces/test-workspace/projects/proj-1/work-items/wi-1/relations/"
			if req.URL.Path != expectedPath {
				t.Errorf("expected path '%s', got '%s'", expectedPath, req.URL.Path)
			}
			if req.Method != "POST" {
				t.Errorf("expected POST, got %s", req.Method)
			}

			var reqBody map[string]any
			bodyBytes, _ := io.ReadAll(req.Body)
			if err := json.Unmarshal(bodyBytes, &reqBody); err != nil {
				t.Fatalf("failed to parse request body: %v", err)
			}
			if reqBody["relation_type"] != "blocking" {
				t.Errorf("expected relation_type='blocking', got %v", reqBody["relation_type"])
			}
			issues, ok := reqBody["issues"].([]any)
			if !ok || len(issues) != 1 || issues[0] != "wi-2" {
				t.Errorf("expected issues=[wi-2], got %v", reqBody["issues"])
			}

			return &http.Response{
				StatusCode: 201,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
			}, nil
		})

		err := client.CreateWorkItemRelation(context.Background(), "proj-1", "wi-1", "blocking", []string{"wi-2"})
		if err != nil {
			t.Fatalf("CreateWorkItemRelation failed: %v", err)
		}
	})

	t.Run("Error path propagates error", func(t *testing.T) {
		client := NewClient(cfg)
		client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 400,
				Body:       io.NopCloser(strings.NewReader("bad request")),
			}, nil
		})

		err := client.CreateWorkItemRelation(context.Background(), "proj-1", "wi-1", "blocking", []string{"wi-2"})
		if err == nil {
			t.Fatal("expected error on 400, got nil")
		}
		if !strings.Contains(err.Error(), "400") {
			t.Errorf("expected error message to mention 400, got: %v", err)
		}
	})
}

func TestClientTransitionIntakeWorkItem(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	var capturedBody map[string]any
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		expectedPath := "/api/v1/workspaces/test-workspace/projects/project-1/intake-issues/issue-1/"
		if req.Method != http.MethodPatch {
			t.Errorf("expected PATCH, got %s", req.Method)
		}
		if req.URL.Path != expectedPath {
			t.Errorf("expected path %q, got %q", expectedPath, req.URL.Path)
		}
		if req.URL.Query().Get("expand") != "issue" {
			t.Errorf("expected expand=issue, got %q", req.URL.Query().Get("expand"))
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("failed to read request body: %v", err)
		}
		if err := json.Unmarshal(body, &capturedBody); err != nil {
			t.Errorf("failed to decode request body: %v", err)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body: io.NopCloser(strings.NewReader(`{
				"id": "intake-1",
				"issue": {"id": "issue-1", "name": "Incoming request", "sequence_id": 10},
				"status": 2,
				"duplicate_to": "issue-target"
			}`)),
		}, nil
	})

	item, err := client.TransitionIntakeWorkItem(context.Background(), "project-1", "issue-1", map[string]any{
		"status":       2,
		"duplicate_to": "issue-target",
	})
	if err != nil {
		t.Fatalf("TransitionIntakeWorkItem failed: %v", err)
	}
	if item.Status != 2 || item.DuplicateTo == nil || *item.DuplicateTo != "issue-target" {
		t.Fatalf("unexpected transitioned item: %+v", item)
	}
	if capturedBody["status"] != float64(2) || capturedBody["duplicate_to"] != "issue-target" {
		t.Errorf("unexpected patched body: %+v", capturedBody)
	}
}

func TestTransitionNotAppliedErrorMessage(t *testing.T) {
	err := &TransitionNotAppliedError{
		Identifier: "ASBX-10",
		IssueUUID:  "issue-10",
		Requested:  "accepted",
		Observed:   "pending",
		Causes:     []string{"insufficient role", "no default state"},
	}
	msg := err.Error()
	for _, want := range []string{"transition_not_applied", "ASBX-10", `"pending"`, `"accepted"`, "insufficient role", "no default state"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q: %s", want, msg)
		}
	}
}
