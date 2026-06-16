package plane

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
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
		if requestCount == 1 {
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
		} else if requestCount == 2 {
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
		} else {
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
			body := `[
				{"id": "wi-1", "name": "Fix login", "sequence_id": 1, "project__identifier": "PROJ", "project_id": "proj-1", "workspace__slug": "test-workspace"},
				{"id": "wi-2", "name": "Login error", "sequence_id": 2, "project__identifier": "PROJ", "project_id": "proj-1", "workspace__slug": "test-workspace"}
			]`
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
