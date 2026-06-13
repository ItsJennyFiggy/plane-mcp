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
