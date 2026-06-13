package plane

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"gopkg.in/yaml.v3"
)

func TestConvertHTMLToMarkdown(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Empty input",
			input:    "",
			expected: "",
		},
		{
			name:     "Simple HTML bold",
			input:    "<p>This is <strong>bold</strong> text.</p>",
			expected: "This is **bold** text.",
		},
		{
			name:     "HTML list",
			input:    "<ul><li>Item 1</li><li>Item 2</li></ul>",
			expected: "- Item 1\n- Item 2",
		},
		{
			name:     "Invalid HTML fallback stripping",
			input:    "<invalid-tag>hello</invalid-tag>",
			expected: "hello",
		},
		{
			name:     "Link conversion",
			input:    `<p>Check <a href="https://example.com">this link</a>.</p>`,
			expected: "Check [this link](https://example.com).",
		},
		{
			name:     "Table conversion",
			input:    "<table><thead><tr><th>Name</th><th>Role</th></tr></thead><tbody><tr><td>Alice</td><td>Admin</td></tr><tr><td>Bob</td><td>Editor</td></tr></tbody></table>",
			expected: "| Name  | Role   |\n|-------|--------|\n| Alice | Admin  |\n| Bob   | Editor |",
		},
		{
			name:     "Strikethrough conversion",
			input:    "<p>This is <del>deprecated</del> <s>old</s> <strike>gone</strike>.</p>",
			expected: "This is ~~deprecated~~ ~~old~~ ~~gone~~.",
		},
		{
			name: "Tiptap task list checked and unchecked",
			input: `<ul class="not-prose pl-2 space-y-2" data-type="taskList">
  <li class="relative" data-checked="false" data-type="taskItem">
    <label><input type="checkbox"><span></span></label>
    <div><p class="editor-paragraph-block">Build the converter</p></div>
  </li>
  <li class="relative" data-checked="true" data-type="taskItem">
    <label><input type="checkbox" checked="checked"><span></span></label>
    <div><p class="editor-paragraph-block">Write tests</p></div>
  </li>
</ul>`,
			expected: "- [ ] Build the converter\n- [x] Write tests",
		},
		{
			name:     "Regular ul unaffected by tasklist renderer",
			input:    "<ul><li>Normal item</li></ul>",
			expected: "- Normal item",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ConvertHTMLToMarkdown(tt.input)
			if got != tt.expected {
				t.Errorf("ConvertHTMLToMarkdown() = %q, expected %q", got, tt.expected)
			}
		})
	}
}

func TestStripHTMLFallback(t *testing.T) {
	input := "<div>Hello <span>World</span>!</div>"
	expected := "Hello World!"
	got := stripHTML(input)
	if got != expected {
		t.Errorf("stripHTML() = %q, expected %q", got, expected)
	}
}

func TestFormatWorkItemYAML(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock resolver fetch calls using specific suffix matching
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		if strings.HasSuffix(req.URL.Path, "/projects/") {
			body = `[{"id": "` + projUUID1 + `", "name": "Agent Infra", "identifier": "AGENT"}]`
		} else if strings.HasSuffix(req.URL.Path, "/states/") {
			body = `[{"id": "` + stateUUID1 + `", "name": "In Progress", "group": "started"}]`
		} else if strings.HasSuffix(req.URL.Path, "/members/") {
			body = `[{"id": "` + userUUID1 + `", "email": "bot@example.com", "display_name": "FiggyBot"}]`
		} else if strings.HasSuffix(req.URL.Path, "/labels/") {
			body = `[{"id": "` + labelUUID1 + `", "name": "core"}]`
		} else {
			body = `[]`
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	parentID := "parent-uuid"
	estimatePoint := 5
	itemType := "feature"

	item := &WorkItem{
		ID:                  "wi-123",
		Name:                "Test task",
		DescriptionHTML:     "<p>Some <strong>rich</strong> details</p>",
		Priority:            "high",
		StartDate:           "2026-06-13",
		TargetDate:          "2026-06-20",
		SequenceID:          42,
		Project:             Expandable[Project]{ID: projUUID1},
		State:               Expandable[State]{ID: stateUUID1},
		Assignees:           []Expandable[Member]{{ID: userUUID1}},
		Labels:              []Expandable[Label]{{ID: labelUUID1}},
		Parent:              &parentID,
		EstimatePoint:       &estimatePoint,
		Type:                &itemType,
	}

	t.Run("Summary Mode", func(t *testing.T) {
		got, err := FormatWorkItemYAML(context.Background(), item, resolver, "summary")
		if err != nil {
			t.Fatalf("FormatWorkItemYAML failed: %v", err)
		}

		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(got), &m); err != nil {
			t.Fatalf("failed to unmarshal yaml output: %v", err)
		}

		// Verify expected fields
		if m["identifier"] != "AGENT-42" {
			t.Errorf("expected identifier AGENT-42, got %v", m["identifier"])
		}
		if m["name"] != "Test task" {
			t.Errorf("expected name 'Test task', got %v", m["name"])
		}
		if m["state"] != "In Progress" {
			t.Errorf("expected state 'In Progress', got %v", m["state"])
		}
		if m["priority"] != "high" {
			t.Errorf("expected priority 'high', got %v", m["priority"])
		}
		assignees, ok := m["assignees"].([]interface{})
		if !ok || len(assignees) != 1 || assignees[0] != "FiggyBot" {
			t.Errorf("expected assignees ['FiggyBot'], got %v", m["assignees"])
		}

		// Ensure full fields are omitted
		if _, exists := m["description"]; exists {
			t.Errorf("description should be omitted in summary mode")
		}
		if _, exists := m["labels"]; exists {
			t.Errorf("labels should be omitted in summary mode")
		}
	})

	t.Run("Full Mode", func(t *testing.T) {
		got, err := FormatWorkItemYAML(context.Background(), item, resolver, "full")
		if err != nil {
			t.Fatalf("FormatWorkItemYAML failed: %v", err)
		}

		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(got), &m); err != nil {
			t.Fatalf("failed to unmarshal yaml output: %v", err)
		}

		if m["description"] != "Some **rich** details" {
			t.Errorf("expected markdown description, got %v", m["description"])
		}
		labels, ok := m["labels"].([]interface{})
		if !ok || len(labels) != 1 || labels[0] != "core" {
			t.Errorf("expected labels ['core'], got %v", m["labels"])
		}
		if m["start_date"] != "2026-06-13" || m["target_date"] != "2026-06-20" {
			t.Errorf("unexpected dates: %v, %v", m["start_date"], m["target_date"])
		}
		if m["parent"] != "parent-uuid" {
			t.Errorf("expected parent 'parent-uuid', got %v", m["parent"])
		}
		if m["estimate_point"] != 5 {
			t.Errorf("expected estimate_point 5, got %v", m["estimate_point"])
		}
		if m["type"] != "feature" {
			t.Errorf("expected type 'feature', got %v", m["type"])
		}
	})

	t.Run("Unrecognized Mode Defaults to Summary", func(t *testing.T) {
		got, err := FormatWorkItemYAML(context.Background(), item, resolver, "some-random-invalid-mode")
		if err != nil {
			t.Fatalf("FormatWorkItemYAML failed: %v", err)
		}

		var m map[string]interface{}
		if err := yaml.Unmarshal([]byte(got), &m); err != nil {
			t.Fatalf("failed to unmarshal yaml output: %v", err)
		}

		if _, exists := m["description"]; exists {
			t.Errorf("description should be omitted in defaulted summary mode")
		}
		if m["identifier"] != "AGENT-42" {
			t.Errorf("expected identifier AGENT-42, got %v", m["identifier"])
		}
	})
}

func TestFormatWorkItemYAMLResilience(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock resolver to always fail
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader("internal server error")),
		}, nil
	})

	item := &WorkItem{
		ID:         "wi-123",
		Name:       "Resilient task",
		SequenceID: 10,
		Project:    Expandable[Project]{ID: "proj-invalid-uuid"}, // not a UUID, will force resolution lookup which fails
	}

	got, err := FormatWorkItemYAML(context.Background(), item, resolver, "summary")
	if err != nil {
		t.Fatalf("FormatWorkItemYAML should not fail on resolution errors, got: %v", err)
	}

	var m map[string]interface{}
	if err := yaml.Unmarshal([]byte(got), &m); err != nil {
		t.Fatalf("failed to unmarshal yaml: %v", err)
	}

	// Should fallback to original ID as identifier
	if m["identifier"] != "wi-123" {
		t.Errorf("expected identifier to fallback to ID 'wi-123', got %v", m["identifier"])
	}
}

func TestFormatWorkItemsYAML(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		var body string
		if strings.HasSuffix(req.URL.Path, "/projects/") {
			body = `[{"id": "` + projUUID1 + `", "name": "Agent Infra", "identifier": "AGENT"}]`
		} else if strings.HasSuffix(req.URL.Path, "/states/") {
			body = `[{"id": "` + stateUUID1 + `", "name": "In Progress", "group": "started"}]`
		} else if strings.HasSuffix(req.URL.Path, "/members/") {
			body = `[{"id": "` + userUUID1 + `", "email": "bot@example.com", "display_name": "FiggyBot"}]`
		} else if strings.HasSuffix(req.URL.Path, "/labels/") {
			body = `[{"id": "` + labelUUID1 + `", "name": "core"}]`
		} else {
			body = `[]`
		}
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
		}, nil
	})

	items := []WorkItem{
		{
			ID:         "wi-1",
			Name:       "Task one",
			SequenceID: 1,
			Project:    Expandable[Project]{ID: projUUID1},
			State:      Expandable[State]{ID: stateUUID1},
		},
		{
			ID:              "wi-2",
			Name:            "Task two",
			DescriptionHTML: "<p>rich description</p>",
			SequenceID:      2,
			Project:         Expandable[Project]{ID: projUUID1},
			State:           Expandable[State]{ID: stateUUID1},
			Labels:          []Expandable[Label]{{ID: labelUUID1}},
		},
	}

	t.Run("Summary Mode List", func(t *testing.T) {
		got, err := FormatWorkItemsYAML(context.Background(), items, resolver, "summary")
		if err != nil {
			t.Fatalf("FormatWorkItemsYAML failed: %v", err)
		}

		var list []map[string]interface{}
		if err := yaml.Unmarshal([]byte(got), &list); err != nil {
			t.Fatalf("failed to unmarshal list: %v", err)
		}

		if len(list) != 2 {
			t.Fatalf("expected list of length 2, got %d", len(list))
		}
		if list[0]["identifier"] != "AGENT-1" || list[1]["identifier"] != "AGENT-2" {
			t.Errorf("unexpected list identifiers: %v", list)
		}
		if _, exists := list[1]["description"]; exists {
			t.Errorf("description should not exist in list summary mode")
		}
	})

	t.Run("Full Mode List", func(t *testing.T) {
		got, err := FormatWorkItemsYAML(context.Background(), items, resolver, "full")
		if err != nil {
			t.Fatalf("FormatWorkItemsYAML failed: %v", err)
		}

		var list []map[string]interface{}
		if err := yaml.Unmarshal([]byte(got), &list); err != nil {
			t.Fatalf("failed to unmarshal list: %v", err)
		}

		if len(list) != 2 {
			t.Fatalf("expected list of length 2, got %d", len(list))
		}
		if list[1]["description"] != "rich description" {
			t.Errorf("expected description 'rich description', got %v", list[1]["description"])
		}
		labels, ok := list[1]["labels"].([]interface{})
		if !ok || len(labels) != 1 || labels[0] != "core" {
			t.Errorf("expected labels ['core'], got %v", list[1]["labels"])
		}
	})
}

func TestFormatWorkItemsYAMLResilience(t *testing.T) {
	cfg := &config.Config{
		PlaneAPIKey:        "test-key",
		PlaneBaseURL:       "https://plane.example.com",
		PlaneWorkspaceSlug: "test-workspace",
	}
	client := NewClient(cfg)
	resolver := NewResolver(client)

	// Mock resolver to always fail
	client.HTTPClient.Transport = mockTransport(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 500,
			Body:       io.NopCloser(strings.NewReader("internal server error")),
		}, nil
	})

	items := []WorkItem{
		{
			ID:         "wi-1",
			Name:       "Resilient list task",
			SequenceID: 10,
			Project:    Expandable[Project]{ID: "proj-invalid-uuid"}, // not a UUID, will force resolution lookup which fails
		},
	}

	got, err := FormatWorkItemsYAML(context.Background(), items, resolver, "summary")
	if err != nil {
		t.Fatalf("FormatWorkItemsYAML should not fail on resolution errors, got: %v", err)
	}

	var list []map[string]interface{}
	if err := yaml.Unmarshal([]byte(got), &list); err != nil {
		t.Fatalf("failed to unmarshal list: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected list of length 1, got %d", len(list))
	}
	if list[0]["identifier"] != "wi-1" {
		t.Errorf("expected identifier to fallback to 'wi-1', got %v", list[0]["identifier"])
	}
}
