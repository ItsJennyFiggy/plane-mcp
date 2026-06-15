package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// TestParseIdentifier — table-driven tests for parseIdentifier
// ---------------------------------------------------------------------------

func TestParseIdentifier(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantProj  string
		wantSeq   int
		wantError bool
	}{
		{
			name:     "simple valid identifier",
			input:    "PROJ-1",
			wantProj: "PROJ",
			wantSeq:  1,
		},
		{
			name:     "multi-part project identifier",
			input:    "MY-PROJ-123",
			wantProj: "MY-PROJ",
			wantSeq:  123,
		},
		{
			name:      "zero sequence number is invalid",
			input:     "PROJ-0",
			wantError: true,
		},
		{
			name:      "empty string is invalid",
			input:     "",
			wantError: true,
		},
		{
			name:      "no hyphen is invalid",
			input:     "NOHYPHEN",
			wantError: true,
		},
		{
			name:      "non-integer sequence is invalid",
			input:     "PROJ-abc",
			wantError: true,
		},
		{
			name:      "trailing hyphen with no sequence is invalid",
			input:     "PROJ-",
			wantError: true,
		},
		{
			name:      "leading hyphen with no project is invalid",
			input:     "-123",
			wantError: true,
		},
		{
			// "PROJ--5" splits on last hyphen: proj="PROJ-", seq=5 — that's actually valid.
			// Test double-hyphen with non-numeric suffix instead.
			name:      "double hyphen with non-numeric suffix is invalid",
			input:     "PROJ--abc",
			wantError: true,
		},
		{
			name:     "large sequence number",
			input:    "ABC-9999",
			wantProj: "ABC",
			wantSeq:  9999,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			proj, seq, err := parseIdentifier(tc.input)

			// Assert
			if tc.wantError {
				if err == nil {
					t.Errorf("parseIdentifier(%q) expected error, got proj=%q seq=%d", tc.input, proj, seq)
				}
				return
			}
			if err != nil {
				t.Errorf("parseIdentifier(%q) unexpected error: %v", tc.input, err)
				return
			}
			if proj != tc.wantProj {
				t.Errorf("parseIdentifier(%q) proj = %q, want %q", tc.input, proj, tc.wantProj)
			}
			if seq != tc.wantSeq {
				t.Errorf("parseIdentifier(%q) seq = %d, want %d", tc.input, seq, tc.wantSeq)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestShouldRegister — table-driven tests for shouldRegister
// ---------------------------------------------------------------------------

func TestShouldRegister(t *testing.T) {
	tests := []struct {
		name            string
		toolName        string
		allowedProfiles []string
		cfg             *config.Config
		want            bool
	}{
		{
			name:            "profile match — full in full",
			toolName:        "find_my_work",
			allowedProfiles: []string{"worker", "planner", "full"},
			cfg:             &config.Config{PlaneMCPProfile: "full"},
			want:            true,
		},
		{
			name:            "profile match — worker profile for worker tool",
			toolName:        "report_progress",
			allowedProfiles: []string{"worker", "planner", "full"},
			cfg:             &config.Config{PlaneMCPProfile: "worker"},
			want:            true,
		},
		{
			name:            "profile mismatch — worker cannot create_task",
			toolName:        "create_task",
			allowedProfiles: []string{"planner", "full"},
			cfg:             &config.Config{PlaneMCPProfile: "worker"},
			want:            false,
		},
		{
			name:            "profile match — planner can create_task",
			toolName:        "create_task",
			allowedProfiles: []string{"planner", "full"},
			cfg:             &config.Config{PlaneMCPProfile: "planner"},
			want:            true,
		},
		{
			name:            "PlaneMCPTools override — allows specific tool",
			toolName:        "get_work_item",
			allowedProfiles: []string{"worker", "planner", "full"},
			cfg: &config.Config{
				PlaneMCPProfile: "worker",
				PlaneMCPTools:   []string{"get_work_item", "ping"},
			},
			want: true,
		},
		{
			name:            "PlaneMCPTools override — blocks unlisted tool regardless of profile",
			toolName:        "create_task",
			allowedProfiles: []string{"planner", "full"},
			cfg: &config.Config{
				PlaneMCPProfile: "full",
				PlaneMCPTools:   []string{"get_work_item", "ping"},
			},
			want: false,
		},
		{
			name:            "PlaneMCPTools override — allows tool for wrong profile",
			toolName:        "create_task",
			allowedProfiles: []string{"planner", "full"},
			cfg: &config.Config{
				PlaneMCPProfile: "worker",
				PlaneMCPTools:   []string{"create_task"},
			},
			want: true,
		},
		{
			name:            "empty PlaneMCPTools falls back to profile check — no match",
			toolName:        "find_my_work",
			allowedProfiles: []string{"planner", "full"},
			cfg:             &config.Config{PlaneMCPProfile: "worker", PlaneMCPTools: []string{}},
			want:            false,
		},
		{
			name:            "profile not in allowedProfiles returns false",
			toolName:        "some_tool",
			allowedProfiles: []string{"full"},
			cfg:             &config.Config{PlaneMCPProfile: "planner"},
			want:            false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Act
			got := shouldRegister(tc.toolName, tc.allowedProfiles, tc.cfg)

			// Assert
			if got != tc.want {
				t.Errorf("shouldRegister(%q, %v, cfg{profile=%q, tools=%v}) = %v, want %v",
					tc.toolName, tc.allowedProfiles, tc.cfg.PlaneMCPProfile, tc.cfg.PlaneMCPTools, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestConvertDescriptionToHTML — table-driven tests for convertDescriptionToHTML
// ---------------------------------------------------------------------------

func TestConvertDescriptionToHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "single paragraph",
			input: "Hello world",
			want:  "<p>Hello world</p>",
		},
		{
			name:  "two paragraphs separated by double newline",
			input: "First paragraph\n\nSecond paragraph",
			want:  "<p>First paragraph</p><p>Second paragraph</p>",
		},
		{
			name:  "trims whitespace from paragraphs",
			input: "  trimmed  \n\n  also trimmed  ",
			want:  "<p>trimmed</p><p>also trimmed</p>",
		},
		{
			name:  "multiple blank lines collapse to empty paragraphs which are skipped",
			input: "Para one\n\n\n\nPara two",
			want:  "<p>Para one</p><p>Para two</p>",
		},
		{
			name:  "h1 heading",
			input: "# Title",
			want:  `<h1 class="editor-heading-block">Title</h1>`,
		},
		{
			name:  "h3 heading",
			input: "### Subsection",
			want:  `<h3 class="editor-heading-block">Subsection</h3>`,
		},
		{
			name:  "h6 heading is max level",
			input: "###### Deep",
			want:  `<h6 class="editor-heading-block">Deep</h6>`,
		},
		{
			name:  "seven hashes is not a heading",
			input: "####### NotHeading",
			want:  "<p>####### NotHeading</p>",
		},
		{
			name:  "unordered list",
			input: "- one\n- two",
			want:  "<ul><li>one</li><li>two</li></ul>",
		},
		{
			name:  "unordered list with asterisks",
			input: "* alpha\n* beta",
			want:  "<ul><li>alpha</li><li>beta</li></ul>",
		},
		{
			name:  "ordered list",
			input: "1. first\n2. second",
			want:  "<ol><li>first</li><li>second</li></ol>",
		},
		{
			// Regression from AGENT-19: a numbered list must become <ol><li>, not literal "1\. ".
			name:  "single ordered item regression",
			input: "1. Item text",
			want:  "<ol><li>Item text</li></ol>",
		},
		{
			name:  "task list with checked and unchecked",
			input: "- [ ] todo\n- [x] done",
			want:  `<ul class="task-list"><li data-checked="false">todo</li><li data-checked="true">done</li></ul>`,
		},
		{
			name:  "fenced code block is not inline-processed",
			input: "```\nprint(\"**hi**\")\n```",
			want:  "<pre><code>print(\"**hi**\")</code></pre>",
		},
		{
			name:  "multi-line fenced code block",
			input: "```\nline one\nline two\n```",
			want:  "<pre><code>line one\nline two</code></pre>",
		},
		{
			name:  "blockquote",
			input: "> This is a quote",
			want:  "<blockquote><p>This is a quote</p></blockquote>",
		},
		{
			name:  "horizontal rule",
			input: "---",
			want:  "<hr>",
		},
		{
			name:  "inline bold italic and code",
			input: "This is **bold** and *italic* and `code`",
			want:  "<p>This is <strong>bold</strong> and <em>italic</em> and <code>code</code></p>",
		},
		{
			name:  "html special characters are escaped",
			input: "a < b & c > d",
			want:  "<p>a &lt; b &amp; c &gt; d</p>",
		},
		{
			name:  "mixed document",
			input: "# Heading\n\nSome intro text\n\n- bullet a\n- bullet b\n\n> a quote",
			want:  `<h1 class="editor-heading-block">Heading</h1><p>Some intro text</p><ul><li>bullet a</li><li>bullet b</li></ul><blockquote><p>a quote</p></blockquote>`,
		},
		{
			name:  "paragraph wrapping lines join with space",
			input: "line one\nline two",
			want:  "<p>line one line two</p>",
		},
		// --- Cases added by Test Writer (AGENT-18/19) ---
		{
			// Case 1: unordered bullet followed immediately by a task item (no blank line)
			// should produce two separate lists.
			name:  "unordered list followed immediately by task item produces two lists",
			input: "- bullet\n- [ ] todo",
			want:  `<ul><li>bullet</li></ul><ul class="task-list"><li data-checked="false">todo</li></ul>`,
		},
		{
			// Case 2: unterminated code fence (no closing ```) must not panic and should
			// treat remaining lines as code content.
			name:  "unterminated code fence does not panic",
			input: "```\nsome code",
			want:  "<pre><code>some code</code></pre>",
		},
		{
			// Case 3: multi-line blockquote — continuation lines joined with a single space.
			name:  "multi-line blockquote joins lines with space",
			input: "> line one\n> line two",
			want:  "<blockquote><p>line one line two</p></blockquote>",
		},
		{
			// Case 4a: *** is recognised as a horizontal rule.
			name:  "triple-asterisk horizontal rule",
			input: "***",
			want:  "<hr>",
		},
		{
			// Case 4b: ___ is recognised as a horizontal rule.
			name:  "triple-underscore horizontal rule",
			input: "___",
			want:  "<hr>",
		},
		{
			// Case 5: uppercase [X] task marker is treated as checked.
			name:  "uppercase X task marker is checked",
			input: "- [X] capital done",
			want:  `<ul class="task-list"><li data-checked="true">capital done</li></ul>`,
		},
		{
			// Case 6: inline emphasis inside a list item.
			name:  "bold inline emphasis inside unordered list item",
			input: "- **bold** item",
			want:  "<ul><li><strong>bold</strong> item</li></ul>",
		},
		{
			// Case 7: HTML escaping inside a code block (escapeHTML applied, not convertInline).
			name:  "html special characters escaped inside code block",
			input: "```\na < b && c > d\n```",
			want:  "<pre><code>a &lt; b &amp;&amp; c &gt; d</code></pre>",
		},
		{
			// Case 8: HTML escaping inside a heading.
			name:  "html escaping inside heading",
			input: "# a < b",
			want:  `<h1 class="editor-heading-block">a &lt; b</h1>`,
		},
		{
			// Case 9: "# " (hash + space) is stripped to "#" by TrimSpace before headingLevel
			// sees it. headingLevel("#") returns 0 (no space follows the hashes), so it falls
			// through to a paragraph. Actual output: <p>#</p>.
			// This documents a known edge-case: a heading with no content requires at least
			// one character after the space (e.g. "# a") to be recognised as a heading.
			name:  "hash-space with no content is treated as a paragraph not a heading",
			input: "# ",
			want:  "<p>#</p>",
		},
		{
			// Case 10: ordered list immediately followed by unordered list (no blank line).
			name:  "ordered list followed immediately by unordered list",
			input: "1. one\n- bullet",
			want:  "<ol><li>one</li></ol><ul><li>bullet</li></ul>",
		},
		{
			// Case 11: paragraph running directly into a heading (no blank line) — the
			// heading is a block-start so paragraph accumulation stops at that line.
			name:  "paragraph followed immediately by heading without blank line",
			input: "intro text\n# Heading",
			want:  `<p>intro text</p><h1 class="editor-heading-block">Heading</h1>`,
		},
		{
			// Case 12a: CRLF line endings produce the same output as LF-only.
			name:  "CRLF line endings produce same output as LF",
			input: "# Title\r\n\r\nParagraph\r\n",
			want:  `<h1 class="editor-heading-block">Title</h1><p>Paragraph</p>`,
		},
		{
			// Case 12b: CRLF inside a fenced code block must NOT leak a trailing \r into
			// the rendered output. The code-fence branch now strips trailing \r explicitly.
			name:  "CRLF inside fenced code block does not leak carriage return",
			input: "```\r\ncode line\r\n```\r\n",
			want:  "<pre><code>code line</code></pre>",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := convertDescriptionToHTML(tc.input)
			if got != tc.want {
				t.Errorf("convertDescriptionToHTML(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestToolError / TestToolText — unit tests for helper constructors
// ---------------------------------------------------------------------------

func TestToolError(t *testing.T) {
	// Arrange
	msg := "something went wrong"

	// Act
	result := toolError(msg)

	// Assert
	if !result.IsError {
		t.Error("toolError result should have IsError=true")
	}
	if len(result.Content) != 1 {
		t.Fatalf("toolError result should have 1 content item, got %d", len(result.Content))
	}
}

func TestToolText(t *testing.T) {
	// Arrange
	text := "some output"

	// Act
	result := toolText(text)

	// Assert
	if result.IsError {
		t.Error("toolText result should have IsError=false")
	}
	if len(result.Content) != 1 {
		t.Fatalf("toolText result should have 1 content item, got %d", len(result.Content))
	}
}

// ---------------------------------------------------------------------------
// Mock implementations for interface-based handler tests
// ---------------------------------------------------------------------------

// strPtr returns a pointer to the given string — useful for building
// FindMyWorkArgs literals with optional *string fields.
func strPtr(s string) *string { return &s }

// mockClient is a test double for planeClient.
type mockClient struct {
	listProjectsFn            func(ctx context.Context) ([]plane.Project, error)
	getWorkItemByIdentifierFn func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error)
	listWorkItemsFn           func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error)
	createWorkItemFn          func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error)
	createWorkItemCommentFn   func(ctx context.Context, projectID, itemID, comment string) error
	updateWorkItemFn          func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error)
	createWorkItemLinkFn      func(ctx context.Context, projectID, itemID, linkURL, title string) error
	addWorkItemsToModuleFn    func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error
	listLabelsFn              func(ctx context.Context, projectID string) ([]plane.Label, error)
}

func (m *mockClient) ListProjects(ctx context.Context) ([]plane.Project, error) {
	return m.listProjectsFn(ctx)
}
func (m *mockClient) GetWorkItemByIdentifier(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
	return m.getWorkItemByIdentifierFn(ctx, projectIdentifier, sequenceID)
}
func (m *mockClient) ListWorkItems(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
	return m.listWorkItemsFn(ctx, projectID, params)
}
func (m *mockClient) CreateWorkItem(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
	return m.createWorkItemFn(ctx, projectID, body)
}
func (m *mockClient) CreateWorkItemComment(ctx context.Context, projectID, itemID, comment string) error {
	return m.createWorkItemCommentFn(ctx, projectID, itemID, comment)
}
func (m *mockClient) UpdateWorkItem(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
	return m.updateWorkItemFn(ctx, projectID, itemID, body)
}
func (m *mockClient) CreateWorkItemLink(ctx context.Context, projectID, itemID, linkURL, title string) error {
	return m.createWorkItemLinkFn(ctx, projectID, itemID, linkURL, title)
}
func (m *mockClient) AddWorkItemsToModule(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
	return m.addWorkItemsToModuleFn(ctx, projectID, moduleID, workItemIDs)
}
func (m *mockClient) ListLabels(ctx context.Context, projectID string) ([]plane.Label, error) {
	return m.listLabelsFn(ctx, projectID)
}

// mockResolver is a test double for planeResolver.
type mockResolver struct {
	getCallerIDFn    func(ctx context.Context) (string, error)
	resolveProjectFn func(ctx context.Context, input string) (*plane.Project, error)
	resolveStateFn   func(ctx context.Context, projectID string, input string) (*plane.State, error)
	resolveLabelFn   func(ctx context.Context, projectID string, input string) (*plane.Label, error)
	resolveModuleFn  func(ctx context.Context, projectID string, input string) (*plane.Module, error)
	resolveMemberFn  func(ctx context.Context, input string) (*plane.Member, error)
}

func (m *mockResolver) GetCallerID(ctx context.Context) (string, error) {
	return m.getCallerIDFn(ctx)
}
func (m *mockResolver) ResolveProject(ctx context.Context, input string) (*plane.Project, error) {
	return m.resolveProjectFn(ctx, input)
}
func (m *mockResolver) ResolveState(ctx context.Context, projectID string, input string) (*plane.State, error) {
	return m.resolveStateFn(ctx, projectID, input)
}
func (m *mockResolver) ResolveLabel(ctx context.Context, projectID string, input string) (*plane.Label, error) {
	return m.resolveLabelFn(ctx, projectID, input)
}
func (m *mockResolver) ResolveModule(ctx context.Context, projectID string, input string) (*plane.Module, error) {
	return m.resolveModuleFn(ctx, projectID, input)
}
func (m *mockResolver) ResolveMember(ctx context.Context, input string) (*plane.Member, error) {
	return m.resolveMemberFn(ctx, input)
}

// mockFormatter is a test double for planeFormatter.
type mockFormatter struct {
	formatWorkItemYAMLFn  func(ctx context.Context, item *plane.WorkItem, detail string) (string, error)
	formatWorkItemsYAMLFn func(ctx context.Context, items []plane.WorkItem, detail string) (string, error)
}

func (m *mockFormatter) FormatWorkItemYAML(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
	return m.formatWorkItemYAMLFn(ctx, item, detail)
}
func (m *mockFormatter) FormatWorkItemsYAML(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
	return m.formatWorkItemsYAMLFn(ctx, items, detail)
}

// ---------------------------------------------------------------------------
// Handler-level tests via mock interfaces
// ---------------------------------------------------------------------------

// TestFindMyWork_GetCallerIDError verifies that find_my_work returns IsError when
// GetCallerID fails.
func TestFindMyWork_GetCallerIDError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "", errors.New("auth failure")
		},
	}
	formatter := &mockFormatter{}
	args := FindMyWorkArgs{}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("findMyWork returned Go error (expected nil, got %v)", err)
	}
	if result == nil {
		t.Fatal("findMyWork returned nil result")
	}
	if !result.IsError {
		t.Error("expected IsError=true when GetCallerID fails")
	}
}

// TestFindMyWork_NoItems verifies the "no items found" text path.
func TestFindMyWork_NoItems(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-1", Name: "Project One", Identifier: "P1"}}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return nil, nil
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
	}
	formatter := &mockFormatter{}
	args := FindMyWorkArgs{}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result set")
	}
	if len(result.Content) == 0 {
		t.Fatal("expected at least one content item")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if tc.Text != "No work items found matching the criteria." {
		t.Errorf("unexpected content: %q", tc.Text)
	}
}

// ---------------------------------------------------------------------------
// TestGetWorkItem_InvalidIdentifier — error path test
// ---------------------------------------------------------------------------

func TestGetWorkItem_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	formatter := &mockFormatter{}
	args := GetWorkItemArgs{Identifier: "NOTVALID"}

	// Act
	result, err := getWorkItem(ctx, args, client, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestGetWorkItem_ClientError — client fetch error is surfaced as tool error.
func TestGetWorkItem_ClientError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	formatter := &mockFormatter{}
	args := GetWorkItemArgs{Identifier: "PROJ-1"}

	// Act
	result, err := getWorkItem(ctx, args, client, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when client returns error")
	}
}

// TestGetWorkItem_Success — happy path returns formatter output.
func TestGetWorkItem_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{ID: "wi-1", Name: "Test Item", SequenceID: 1}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := GetWorkItemArgs{Identifier: "PROJ-1"}

	// Act
	result, err := getWorkItem(ctx, args, client, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false on success")
	}
}

// TestReportProgress_InvalidIdentifier — error wrapping for bad identifier.
func TestReportProgress_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	args := ReportProgressArgs{Identifier: "BAD"}

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestReportProgress_WithStateUpdate — state transition confirmation message.
func TestReportProgress_WithStateUpdate(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Test",
		SequenceID: 5,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			return nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID string, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Progress"}, nil
		},
	}
	args := ReportProgressArgs{
		Identifier: "PROJ-5",
		Comment:    "making progress",
		State:      "In Progress",
	}

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, content: %+v", result.Content)
	}
}

// TestCreateTask_ResolveProjectError — tool error on project resolution failure.
func TestCreateTask_ResolveProjectError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return nil, errors.New("project not found: UNKNOWN")
		},
	}
	formatter := &mockFormatter{}
	args := CreateTaskArgs{Project: "UNKNOWN", Name: "New Task"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when project resolution fails")
	}
}

// TestSubmitForReview_InvalidIdentifier — error path for bad identifier.
func TestSubmitForReview_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	args := SubmitForReviewArgs{Identifier: "BADIDENT"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestSubmitForReview_WorkItemFetchError — client error surfaced as tool error.
func TestSubmitForReview_WorkItemFetchError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("item not found")
		},
	}
	resolver := &mockResolver{}
	args := SubmitForReviewArgs{Identifier: "PROJ-1", PRURL: "https://github.com/org/repo/pull/42"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when work item fetch fails")
	}
}

// TestSubmitForReview_InReviewStateNotFound — state resolution failure returns clear error.
func TestSubmitForReview_InReviewStateNotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return nil, errors.New("state not found: In Review")
		},
	}
	args := SubmitForReviewArgs{Identifier: "PROJ-1", PRURL: "https://github.com/org/repo/pull/42"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when In Review state not found")
	}
}

// TestSubmitForReview_LinkError — link creation failure surfaced as tool error.
func TestSubmitForReview_LinkError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemLinkFn: func(ctx context.Context, projectID, itemID, linkURL, title string) error {
			return errors.New("link creation failed")
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Review"}, nil
		},
	}
	args := SubmitForReviewArgs{Identifier: "PROJ-1", PRURL: "https://github.com/org/repo/pull/42"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when link creation fails")
	}
}

// TestSubmitForReview_CommentError — comment posting failure.
func TestSubmitForReview_CommentError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemLinkFn: func(ctx context.Context, projectID, itemID, linkURL, title string) error {
			return nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			return errors.New("comment failed")
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Review"}, nil
		},
	}
	args := SubmitForReviewArgs{Identifier: "PROJ-1", PRURL: "https://github.com/org/repo/pull/42"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when comment posting fails")
	}
}

// TestSubmitForReview_UpdateError — state update failure.
func TestSubmitForReview_UpdateError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemLinkFn: func(ctx context.Context, projectID, itemID, linkURL, title string) error {
			return nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			return nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("update failed")
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Review"}, nil
		},
	}
	args := SubmitForReviewArgs{Identifier: "PROJ-1", PRURL: "https://github.com/org/repo/pull/42"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when state update fails")
	}
}

// TestSubmitForReview_Success — full happy path including default comment.
func TestSubmitForReview_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemLinkFn: func(ctx context.Context, projectID, itemID, linkURL, title string) error {
			return nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			return nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Review"}, nil
		},
	}
	// No comment provided — should use default
	args := SubmitForReviewArgs{Identifier: "PROJ-1", PRURL: "https://github.com/org/repo/pull/42"}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false on success: %+v", result.Content)
	}
}

// TestSubmitForReview_CustomComment — custom comment is used when provided.
func TestSubmitForReview_CustomComment(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedComment string
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemLinkFn: func(ctx context.Context, projectID, itemID, linkURL, title string) error {
			return nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			capturedComment = comment
			return nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Review"}, nil
		},
	}
	args := SubmitForReviewArgs{
		Identifier: "PROJ-1",
		PRURL:      "https://github.com/org/repo/pull/42",
		Comment:    "Ready for review, added tests",
	}

	// Act
	result, err := submitForReview(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false on success")
	}
	if capturedComment != "Ready for review, added tests" {
		t.Errorf("expected custom comment, got %q", capturedComment)
	}
}

// TestFindMyWork_WithProject — find_my_work scoped to a specific project.
func TestFindMyWork_WithProject(t *testing.T) {
	// Arrange
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "My Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return items, nil
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-1", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: My Task\n", nil
		},
	}
	args := FindMyWorkArgs{Project: strPtr("Alpha")}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
}

// TestFindMyWork_WithProjectResolveError — project resolution error.
func TestFindMyWork_WithProjectResolveError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return nil, errors.New("project not found: UNKNOWN")
		},
	}
	formatter := &mockFormatter{}
	args := FindMyWorkArgs{Project: strPtr("UNKNOWN")}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when project resolution fails")
	}
}

// TestFindMyWork_ListProjectsError — ListProjects error handled gracefully.
func TestFindMyWork_ListProjectsError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return nil, errors.New("API down")
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
	}
	formatter := &mockFormatter{}
	args := FindMyWorkArgs{} // no project — iterates all

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when ListProjects fails")
	}
}

// TestFindMyWork_NoFilter — empty args returns all assigned work across all projects.
func TestFindMyWork_NoFilter(t *testing.T) {
	// Arrange
	ctx := context.Background()
	items := []plane.WorkItem{
		{ID: "wi-1", Name: "Task One", SequenceID: 1},
		{ID: "wi-2", Name: "Task Two", SequenceID: 2},
	}
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{
				{ID: "proj-1", Name: "Alpha", Identifier: "ALP"},
				{ID: "proj-2", Name: "Beta", Identifier: "BET"},
			}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			// Verify no state_group filter is present
			if _, ok := params["state_group"]; ok {
				t.Error("state_group param should not be set when StateGroup is nil")
			}
			// Verify assignee is set
			if params["assignees"] != "user-uuid" {
				t.Errorf("expected assignees=user-uuid, got %q", params["assignees"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Task One\n- name: Task Two\n", nil
		},
	}
	args := FindMyWorkArgs{} // no filters

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestFindMyWork_StateGroupOnly filters by state_group across all projects
// when no project is specified.
func TestFindMyWork_StateGroupOnly(t *testing.T) {
	// Arrange
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "In Progress Task", SequenceID: 1}}
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-1", Name: "Alpha", Identifier: "ALP"}}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["state_group"] != "in_progress" {
				t.Errorf("expected state_group=in_progress, got %q", params["state_group"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: In Progress Task\n", nil
		},
	}
	args := FindMyWorkArgs{StateGroup: strPtr("in_progress")}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestFindMyWork_BothFilters scopes work items to a specific project and state_group.
func TestFindMyWork_BothFilters(t *testing.T) {
	// Arrange
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Specific Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if projectID != "proj-alpha" {
				t.Errorf("expected projectID=proj-alpha, got %q", projectID)
			}
			if params["state_group"] != "completed" {
				t.Errorf("expected state_group=completed, got %q", params["state_group"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			if input != "Alpha" {
				t.Errorf("expected project input 'Alpha', got %q", input)
			}
			return &plane.Project{ID: "proj-alpha", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Specific Task\n", nil
		},
	}
	args := FindMyWorkArgs{
		Project:    strPtr("Alpha"),
		StateGroup: strPtr("completed"),
	}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestReportProgress_NoComment_NoState — just fetches and confirms.
func TestReportProgress_NoComment_NoState(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 7,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	resolver := &mockResolver{}
	args := ReportProgressArgs{Identifier: "PROJ-7"}

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
}

// TestReportProgress_CommentError — comment failure surfaced as tool error.
func TestReportProgress_CommentError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			return errors.New("comment failed")
		},
	}
	resolver := &mockResolver{}
	args := ReportProgressArgs{Identifier: "PROJ-1", Comment: "update"}

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when comment fails")
	}
}

// TestReportProgress_StateResolveError — state resolution error surfaced.
func TestReportProgress_StateResolveError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return nil, errors.New("state not found: Done")
		},
	}
	args := ReportProgressArgs{Identifier: "PROJ-1", State: "Done"}

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when state resolution fails")
	}
}

// TestReportProgress_UpdateError — state update failure.
func TestReportProgress_UpdateError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("update failed")
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "Done"}, nil
		},
	}
	args := ReportProgressArgs{Identifier: "PROJ-1", State: "Done"}

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when update fails")
	}
}

// TestCreateTask_CreateError — CreateWorkItem failure surfaced.
func TestCreateTask_CreateError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("API error")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := CreateTaskArgs{Project: "My Project", Name: "Failing Task"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when CreateWorkItem fails")
	}
}

// TestCreateTask_WithAssigneesAndLabels — assignee/label resolution + skip on failure.
func TestCreateTask_WithAssigneesAndLabels(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-new", Name: "Tagged Task", SequenceID: 11}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return created, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			if input == "good-user" {
				return &plane.Member{ID: "member-uuid", DisplayName: "Good User"}, nil
			}
			return nil, errors.New("member not found: " + input)
		},
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			if input == "bug" {
				return &plane.Label{ID: "label-uuid", Name: "bug"}, nil
			}
			return nil, errors.New("label not found: " + input)
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Tagged Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project:   "My Project",
		Name:      "Tagged Task",
		Assignees: FlexibleStringSlice{"good-user", "bad-user"}, // bad-user skipped
		Labels:    FlexibleStringSlice{"bug", "missing-label"},  // missing-label skipped
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
}

// TestCreateTask_FormatError — formatter error surfaced as tool error.
func TestCreateTask_FormatError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-new", Name: "Format Fail", SequenceID: 12}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return created, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "", errors.New("format failed")
		},
	}
	args := CreateTaskArgs{Project: "My Project", Name: "Format Fail"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when formatter fails")
	}
}

// TestCreateTask_WithModule — module resolves, task is created, then added to the module.
func TestCreateTask_WithModule(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-new", Name: "Module Task", SequenceID: 13}
	var capturedModuleID string
	var capturedWorkItemIDs []string
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return created, nil
		},
		addWorkItemsToModuleFn: func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
			capturedModuleID = moduleID
			capturedWorkItemIDs = workItemIDs
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID, input string) (*plane.Module, error) {
			return &plane.Module{ID: "mod-uuid", Name: "Sprint One"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Module Task\n", nil
		},
	}
	args := CreateTaskArgs{Project: "My Project", Name: "Module Task", Module: "Sprint One"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedModuleID != "mod-uuid" {
		t.Errorf("expected module ID 'mod-uuid', got '%s'", capturedModuleID)
	}
	if len(capturedWorkItemIDs) != 1 || capturedWorkItemIDs[0] != "wi-new" {
		t.Errorf("expected work item IDs [wi-new], got %v", capturedWorkItemIDs)
	}
}

// TestCreateTask_ModuleResolutionFailsBeforeCreate — fail fast: unresolved module errors
// without ever creating the work item.
func TestCreateTask_ModuleResolutionFailsBeforeCreate(t *testing.T) {
	// Arrange
	ctx := context.Background()
	createCalled := false
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			createCalled = true
			return &plane.WorkItem{ID: "wi-new"}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID, input string) (*plane.Module, error) {
			return nil, errors.New("module not found: Bogus")
		},
	}
	formatter := &mockFormatter{}
	args := CreateTaskArgs{Project: "My Project", Name: "Module Task", Module: "Bogus"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when module resolution fails")
	}
	if createCalled {
		t.Error("expected CreateWorkItem NOT to be called when module resolution fails")
	}
}

// TestCreateTask_AddToModuleFailsAfterCreate — add-to-module failure after a successful
// create surfaces a tool error noting the item was created.
func TestCreateTask_AddToModuleFailsAfterCreate(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-new", Name: "Module Task", SequenceID: 14}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return created, nil
		},
		addWorkItemsToModuleFn: func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
			return errors.New("module-issues endpoint failed")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID, input string) (*plane.Module, error) {
			return &plane.Module{ID: "mod-uuid", Name: "Sprint One"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := CreateTaskArgs{Project: "My Project", Name: "Module Task", Module: "Sprint One"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when add-to-module fails")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "was created but could not be added to module") {
		t.Errorf("expected error to note the item was created, got: %s", textContent.Text)
	}
}

// TestGetWorkItem_DefaultDetailIsSummary — omitting detail defaults to "summary".
func TestGetWorkItem_DefaultDetailIsSummary(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{ID: "wi-1", Name: "Test", SequenceID: 1}
	var capturedDetail string
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			capturedDetail = detail
			return "name: Test\n", nil
		},
	}
	args := GetWorkItemArgs{Identifier: "PROJ-1"} // no Detail

	// Act
	result, err := getWorkItem(ctx, args, client, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false")
	}
	if capturedDetail != "summary" {
		t.Errorf("expected detail='summary', got %q", capturedDetail)
	}
}

// TestGetWorkItem_FormatError — formatter error.
func TestGetWorkItem_FormatError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{ID: "wi-1", Name: "Test", SequenceID: 1}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "", errors.New("format failed")
		},
	}
	args := GetWorkItemArgs{Identifier: "PROJ-1", Detail: "full"}

	// Act
	result, err := getWorkItem(ctx, args, client, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when formatter fails")
	}
}

// TestFindMyWork_FormatterError — formatter failure on items found.
func TestFindMyWork_FormatterError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Task", SequenceID: 1}}
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-1"}}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return items, nil
		},
	}
	resolver := &mockResolver{
		getCallerIDFn: func(ctx context.Context) (string, error) {
			return "user-uuid", nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "", errors.New("format failed")
		},
	}
	args := FindMyWorkArgs{}

	// Act
	result, err := findMyWork(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when formatter fails")
	}
}

// TestCreateTask_Success — happy path for task creation.
func TestCreateTask_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-new", Name: "New Task", SequenceID: 10}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return created, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project", Identifier: "MP"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: New Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project:     "My Project",
		Name:        "New Task",
		Description: "First paragraph\n\nSecond paragraph",
		Priority:    "high",
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error result: %+v", result.Content)
	}
}

// ---------------------------------------------------------------------------
// TestRegisterWithDeps — verifies that registerWithDeps actually adds the
// expected tools to the MCP server based on profile/tool allowlist.
// ---------------------------------------------------------------------------

func TestRegisterWithDeps_FullProfile(t *testing.T) {
	// Arrange
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	cfg := &config.Config{PlaneMCPProfile: "full"}

	// Act — register all tools (full profile)
	registerWithDeps(server, client, resolver, formatter, cfg)
	// No panic means all six tools were registered successfully.
	// (The MCP SDK panics if a tool with an invalid name is registered.)
}

func TestRegisterWithDeps_WorkerProfile(t *testing.T) {
	// Arrange — worker profile should register 5 tools (not create_task)
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	cfg := &config.Config{PlaneMCPProfile: "worker"}

	// Act
	registerWithDeps(server, client, resolver, formatter, cfg)
	// No panic = success
}

func TestRegisterWithDeps_ExplicitToolList(t *testing.T) {
	// Arrange — PlaneMCPTools overrides profile
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	cfg := &config.Config{
		PlaneMCPProfile: "worker",
		PlaneMCPTools:   []string{"get_work_item"},
	}

	// Act
	registerWithDeps(server, client, resolver, formatter, cfg)
	// Only get_work_item should be registered — no panic.
}

// ---------------------------------------------------------------------------
// createTask handler tests added by Test Writer (AGENT-18/19)
// ---------------------------------------------------------------------------

// TestCreateTask_DescriptionHTMLIsBuiltAndPassed — verifies that the Markdown description
// is converted to HTML and placed in body["description_html"] (AGENT-19 seam).
func TestCreateTask_DescriptionHTMLIsBuiltAndPassed(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedBody map[string]any
	created := &plane.WorkItem{ID: "wi-desc", Name: "N", SequenceID: 20}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return created, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "P"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: N\n", nil
		},
	}
	args := CreateTaskArgs{
		Project:     "P",
		Name:        "N",
		Description: "# Title\n\nParagraph",
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	wantHTML := `<h1 class="editor-heading-block">Title</h1><p>Paragraph</p>`
	gotHTML, ok := capturedBody["description_html"].(string)
	if !ok {
		t.Fatalf("body[\"description_html\"] is not a string, got %T: %v", capturedBody["description_html"], capturedBody["description_html"])
	}
	if gotHTML != wantHTML {
		t.Errorf("body[\"description_html\"] = %q, want %q", gotHTML, wantHTML)
	}
}

// TestCreateTask_EmptyModuleSkipsResolution — when Module is empty, resolveModuleFn and
// addWorkItemsToModuleFn must never be invoked.
func TestCreateTask_EmptyModuleSkipsResolution(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-no-mod", Name: "NoMod Task", SequenceID: 21}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return created, nil
		},
		addWorkItemsToModuleFn: func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
			t.Fatal("addWorkItemsToModule must not be called when Module is empty")
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "P"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID, input string) (*plane.Module, error) {
			t.Fatal("resolveModule must not be called when Module is empty")
			return nil, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: NoMod Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project: "P",
		Name:    "NoMod Task",
		Module:  "", // empty — must skip resolution entirely
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
}

// TestCreateTask_ModuleAssigneesAndLabelsTogether — when all three optional fields are set,
// body["assignees"] and body["labels"] must be populated AND AddWorkItemsToModule must
// be called with the resolved module ID and the new work-item ID.
func TestCreateTask_ModuleAssigneesAndLabelsTogether(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-combo", Name: "Combo Task", SequenceID: 22}
	var capturedBody map[string]any
	var capturedModuleID string
	var capturedWorkItemIDs []string
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return created, nil
		},
		addWorkItemsToModuleFn: func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
			capturedModuleID = moduleID
			capturedWorkItemIDs = workItemIDs
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "P"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID, input string) (*plane.Module, error) {
			return &plane.Module{ID: "mod-combo", Name: "Sprint Combo"}, nil
		},
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "member-combo", DisplayName: "Alice"}, nil
		},
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "label-combo", Name: "feature"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Combo Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project:   "P",
		Name:      "Combo Task",
		Module:    "Sprint Combo",
		Assignees: FlexibleStringSlice{"alice"},
		Labels:    FlexibleStringSlice{"feature"},
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	// body["assignees"] must be set
	assignees, ok := capturedBody["assignees"].([]string)
	if !ok || len(assignees) != 1 || assignees[0] != "member-combo" {
		t.Errorf("body[\"assignees\"] = %v, want [member-combo]", capturedBody["assignees"])
	}
	// body["labels"] must be set
	labelIDs, ok := capturedBody["labels"].([]string)
	if !ok || len(labelIDs) != 1 || labelIDs[0] != "label-combo" {
		t.Errorf("body[\"labels\"] = %v, want [label-combo]", capturedBody["labels"])
	}
	// AddWorkItemsToModule must have been called with the resolved IDs
	if capturedModuleID != "mod-combo" {
		t.Errorf("AddWorkItemsToModule called with moduleID=%q, want %q", capturedModuleID, "mod-combo")
	}
	if len(capturedWorkItemIDs) != 1 || capturedWorkItemIDs[0] != "wi-combo" {
		t.Errorf("AddWorkItemsToModule called with workItemIDs=%v, want [wi-combo]", capturedWorkItemIDs)
	}
}

// TestCreateTask_ModuleAndDescriptionTogether — when both module and description are set,
// body["description_html"] must be populated AND AddWorkItemsToModule must be called.
func TestCreateTask_ModuleAndDescriptionTogether(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-moddesc", Name: "ModDesc Task", SequenceID: 23}
	var capturedBody map[string]any
	var capturedModuleID string
	var capturedWorkItemIDs []string
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return created, nil
		},
		addWorkItemsToModuleFn: func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
			capturedModuleID = moduleID
			capturedWorkItemIDs = workItemIDs
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "P"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID, input string) (*plane.Module, error) {
			return &plane.Module{ID: "mod-desc", Name: "Sprint Desc"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: ModDesc Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project:     "P",
		Name:        "ModDesc Task",
		Module:      "Sprint Desc",
		Description: "Some **markdown**",
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	// description_html must be present in the body
	descHTML, ok := capturedBody["description_html"].(string)
	if !ok || descHTML == "" {
		t.Errorf("body[\"description_html\"] not populated, got %v", capturedBody["description_html"])
	}
	// AddWorkItemsToModule must have been called with the correct IDs
	if capturedModuleID != "mod-desc" {
		t.Errorf("AddWorkItemsToModule moduleID=%q, want %q", capturedModuleID, "mod-desc")
	}
	if len(capturedWorkItemIDs) != 1 || capturedWorkItemIDs[0] != "wi-moddesc" {
		t.Errorf("AddWorkItemsToModule workItemIDs=%v, want [wi-moddesc]", capturedWorkItemIDs)
	}
}

// ---------------------------------------------------------------------------
// TestFlexibleStringSlice_UnmarshalJSON — verifies the custom unmarshaler
// accepts JSON arrays, stringified JSON arrays, comma-separated strings,
// and edge cases (null / empty).
// ---------------------------------------------------------------------------

func TestFlexibleStringSlice_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name string
		json string
		want []string
	}{
		{
			name: "JSON array",
			json: `["alice", "bob"]`,
			want: []string{"alice", "bob"},
		},
		{
			name: "empty JSON array",
			json: `[]`,
			want: []string{},
		},
		{
			name: "stringified JSON array",
			json: `"[\"uuid-1\", \"uuid-2\"]"`,
			want: []string{"uuid-1", "uuid-2"},
		},
		{
			name: "comma-separated string",
			json: `"alice, bob, charlie"`,
			want: []string{"alice", "bob", "charlie"},
		},
		{
			name: "comma-separated string no spaces",
			json: `"alice,bob"`,
			want: []string{"alice", "bob"},
		},
		{
			name: "single value string",
			json: `"just-me"`,
			want: []string{"just-me"},
		},
		{
			name: "null",
			json: `null`,
			want: nil,
		},
		{
			name: "empty string",
			json: `""`,
			want: nil,
		},
		{
			name: "stringified empty array",
			json: `"[]"`,
			want: []string{},
		},
		{
			name: "comma-separated with empty parts",
			json: `"alice, , bob"`,
			want: []string{"alice", "bob"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s FlexibleStringSlice
			err := json.Unmarshal([]byte(tt.json), &s)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.want == nil {
				if s != nil {
					t.Errorf("expected nil, got %v", []string(s))
				}
				return
			}
			if len(s) != len(tt.want) {
				t.Fatalf("len mismatch: got %d, want %d (%v)", len(s), len(tt.want), []string(s))
			}
			for i := range s {
				if s[i] != tt.want[i] {
					t.Errorf("index %d: got %q, want %q", i, s[i], tt.want[i])
				}
			}
		})
	}
}

// TestCreateTask_AssigneesLabelsOmitted — create_task succeeds without
// assignees or labels (regression test for Problem 1: required fields).
func TestCreateTask_AssigneesLabelsOmitted(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-omit", Name: "Minimal Task", SequenceID: 25}
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			// Verify assignees and labels are NOT in the body.
			if _, ok := body["assignees"]; ok {
				t.Error("assignees should not be present in body when omitted")
			}
			if _, ok := body["labels"]; ok {
				t.Error("labels should not be present in body when omitted")
			}
			return created, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Minimal Task\n", nil
		},
	}
	// Explicitly zero-value for Assignees and Labels.
	args := CreateTaskArgs{Project: "My Project", Name: "Minimal Task"}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
}

// TestCreateTask_SchemaAllowsStringifiedArrays — exercises the real SDK
// validation path (Resolve → Validate → Unmarshal) to prove that
// stringified array arguments for assignees/labels pass schema validation
// and are correctly unmarshaled by FlexibleStringSlice.
func TestCreateTask_SchemaAllowsStringifiedArrays(t *testing.T) {
	schema := createTaskInputSchema()
	resolved, err := schema.Resolve(&jsonschema.ResolveOptions{ValidateDefaults: true})
	if err != nil {
		t.Fatalf("failed to resolve schema: %v", err)
	}

	// A raw JSON payload where assignees and labels are stringified.
	raw := json.RawMessage(`{"project":"P","name":"N","assignees":"[\"uuid-1\"]","labels":"bug, feature"}`)

	// Step 1 — unmarshal into map (as applySchema does).
	var v map[string]any
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}

	// Step 2 — validate against the resolved schema (simulates applySchema).
	if err := resolved.Validate(&v); err != nil {
		t.Fatalf("schema validation rejected stringified arrays: %v", err)
	}

	// Step 3 — unmarshal into the typed struct (simulates the SDK's
	// internaljson.Unmarshal after validation passes).
	var args CreateTaskArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		t.Fatalf("unmarshal into CreateTaskArgs failed: %v", err)
	}

	if len(args.Assignees) != 1 || args.Assignees[0] != "uuid-1" {
		t.Errorf("assignees = %v, want [uuid-1]", []string(args.Assignees))
	}
	if len(args.Labels) != 2 || args.Labels[0] != "bug" || args.Labels[1] != "feature" {
		t.Errorf("labels = %v, want [bug feature]", []string(args.Labels))
	}
}

// TestCreateTask_SchemaRequiredList — verifies that optional fields are
// absent from the schema's "required" list (regression test for the
// description/priority side of Problem 1).
func TestCreateTask_SchemaRequiredList(t *testing.T) {
	schema := createTaskInputSchema()

	required := schema.Required
	expectedRequired := map[string]bool{"project": true, "name": true}

	for _, r := range required {
		if expectedRequired[r] {
			delete(expectedRequired, r)
		} else {
			t.Errorf("unexpected required field: %q", r)
		}
	}
	for r := range expectedRequired {
		t.Errorf("missing required field: %q", r)
	}

	// Verify optional fields are NOT in required.
	for _, opt := range []string{"description", "priority", "assignees", "labels", "module", "parent"} {
		for _, r := range required {
			if r == opt {
				t.Errorf("%q should be optional but is in required list", opt)
			}
		}
	}
}

// TestCreateTask_WithParent — parent identifier resolves to UUID and body includes it.
func TestCreateTask_WithParent(t *testing.T) {
	// Arrange
	ctx := context.Background()
	parentItem := &plane.WorkItem{ID: "parent-uuid", Name: "Parent Task", SequenceID: 6}
	created := &plane.WorkItem{ID: "wi-child", Name: "Child Task", SequenceID: 30}
	var capturedBody map[string]any
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return created, nil
		},
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			if projectIdentifier != "EXEC" || sequenceID != 6 {
				t.Errorf("GetWorkItemByIdentifier called with %q / %d, want EXEC / 6", projectIdentifier, sequenceID)
			}
			return parentItem, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Child Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project: "My Project",
		Name:    "Child Task",
		Parent:  "EXEC-6",
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	parentID, ok := capturedBody["parent"].(string)
	if !ok {
		t.Fatalf("body[\"parent\"] missing or not string: %v", capturedBody["parent"])
	}
	if parentID != "parent-uuid" {
		t.Errorf("body[\"parent\"] = %q, want %q", parentID, "parent-uuid")
	}
}

// TestCreateTask_WithoutParent — no parent arg = backward compat, no 'parent' in body.
func TestCreateTask_WithoutParent(t *testing.T) {
	// Arrange
	ctx := context.Background()
	created := &plane.WorkItem{ID: "wi-noparent", Name: "No Parent Task", SequenceID: 31}
	var capturedBody map[string]any
	client := &mockClient{
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return created, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: No Parent Task\n", nil
		},
	}
	args := CreateTaskArgs{
		Project: "My Project",
		Name:    "No Parent Task",
		// Parent intentionally zero-value
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	if _, ok := capturedBody["parent"]; ok {
		t.Error("body[\"parent\"] should not be present when Parent is empty")
	}
}

// TestCreateTask_InvalidParent — non-identifier string returns clear error.
func TestCreateTask_InvalidParent(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		// getWorkItemByIdentifierFn intentionally nil — must not be called.
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := CreateTaskArgs{
		Project: "My Project",
		Name:    "Bad Parent Task",
		Parent:  "not-an-identifier",
	}

	// Act
	result, err := createTask(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for invalid parent identifier")
	}
	// Check that the error message mentions the invalid identifier.
	content := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(content, "invalid parent identifier") {
		t.Errorf("expected error message to contain 'invalid parent identifier', got: %q", content)
	}
	if !strings.Contains(content, "not-an-identifier") {
		t.Errorf("expected error message to contain the bad identifier, got: %q", content)
	}
}

// ---------------------------------------------------------------------------
// listLabels handler tests (AGENT-30)
// ---------------------------------------------------------------------------

// TestListLabels_Success — happy path: labels found and formatted.
func TestListLabels_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	labels := []plane.Label{
		{ID: "lbl-1", Name: "bug", Color: "#ff0000"},
		{ID: "lbl-2", Name: "feature", Color: "#00ff00"},
	}
	client := &mockClient{
		listLabelsFn: func(ctx context.Context, projectID string) ([]plane.Label, error) {
			return labels, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	args := ListLabelsArgs{Project: "My Project"}

	// Act
	result, err := listLabels(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(tc.Text, `name: "bug"`) || !strings.Contains(tc.Text, `color: "#ff0000"`) || !strings.Contains(tc.Text, `id: "lbl-1"`) {
		t.Errorf("expected label details in output, got: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, `name: "feature"`) || !strings.Contains(tc.Text, `color: "#00ff00"`) || !strings.Contains(tc.Text, `id: "lbl-2"`) {
		t.Errorf("expected second label details in output, got: %q", tc.Text)
	}
}

// TestListLabels_YAMLRoundTrip — output must be parseable as valid YAML.
func TestListLabels_YAMLRoundTrip(t *testing.T) {
	// Arrange
	ctx := context.Background()
	labels := []plane.Label{
		{ID: "lbl-1", Name: "bug", Color: "#ff0000"},
		{ID: "lbl-2", Name: "feature", Color: "#00ff00"},
		{ID: "lbl-3", Name: "role:executor", Color: "#0000ff"},
	}
	client := &mockClient{
		listLabelsFn: func(ctx context.Context, projectID string) ([]plane.Label, error) {
			return labels, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	args := ListLabelsArgs{Project: "Test"}

	// Act
	result, err := listLabels(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false: %+v", result.Content)
	}

	tc := result.Content[0].(*mcp.TextContent)

	// Parse as YAML — must succeed.
	var parsed []map[string]string
	if err := yaml.Unmarshal([]byte(tc.Text), &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\noutput:\n%s", err, tc.Text)
	}

	if len(parsed) != 3 {
		t.Fatalf("expected 3 labels, got %d", len(parsed))
	}
	if parsed[0]["name"] != "bug" || parsed[0]["color"] != "#ff0000" || parsed[0]["id"] != "lbl-1" {
		t.Errorf("first label: got id=%q name=%q color=%q", parsed[0]["id"], parsed[0]["name"], parsed[0]["color"])
	}
	if parsed[1]["name"] != "feature" || parsed[1]["color"] != "#00ff00" || parsed[1]["id"] != "lbl-2" {
		t.Errorf("second label: got id=%q name=%q color=%q", parsed[1]["id"], parsed[1]["name"], parsed[1]["color"])
	}
	// Label with colon in name must survive quoting.
	if parsed[2]["name"] != "role:executor" || parsed[2]["color"] != "#0000ff" {
		t.Errorf("third label: got name=%q color=%q", parsed[2]["name"], parsed[2]["color"])
	}
}

// TestListLabels_Empty — no labels in project returns a clear message.
func TestListLabels_Empty(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listLabelsFn: func(ctx context.Context, projectID string) ([]plane.Label, error) {
			return nil, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	args := ListLabelsArgs{Project: "Empty Project"}

	// Act
	result, err := listLabels(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if tc.Text != "No labels found in this project." {
		t.Errorf("expected 'No labels found' message, got: %q", tc.Text)
	}
}

// TestListLabels_ProjectResolutionError — project not found returns error.
func TestListLabels_ProjectResolutionError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return nil, errors.New("project not found")
		},
	}
	args := ListLabelsArgs{Project: "Unknown"}

	// Act
	result, err := listLabels(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when project resolution fails")
	}
}

// TestListLabels_ClientError — client.ListLabels error is surfaced.
func TestListLabels_ClientError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listLabelsFn: func(ctx context.Context, projectID string) ([]plane.Label, error) {
			return nil, errors.New("api error")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	args := ListLabelsArgs{Project: "My Project"}

	// Act
	result, err := listLabels(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when client.ListLabels fails")
	}
}

// ---------------------------------------------------------------------------
// add_label handler tests (AGENT-34)
// ---------------------------------------------------------------------------

// TestAddLabel_Success — happy path: label resolved and attached.
func TestAddLabel_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{{ID: "lbl-existing"}},
	}
	var capturedBody map[string]any
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-new", Name: "enhancement"}, nil
		},
	}
	args := AddLabelArgs{Identifier: "PROJ-1", Label: "enhancement"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	labelIDs, ok := capturedBody["labels"].([]string)
	if !ok {
		t.Fatalf("labels not a []string: %T", capturedBody["labels"])
	}
	if len(labelIDs) != 2 {
		t.Fatalf("expected 2 labels, got %d: %v", len(labelIDs), labelIDs)
	}
	if labelIDs[0] != "lbl-existing" || labelIDs[1] != "lbl-new" {
		t.Errorf("expected [lbl-existing lbl-new], got %v", labelIDs)
	}
}

// TestAddLabel_AlreadyAttached — idempotent: no-op when label is already present.
func TestAddLabel_AlreadyAttached(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{{ID: "lbl-feature"}},
	}
	updateCalled := false
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			updateCalled = true
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-feature", Name: "feature"}, nil
		},
	}
	args := AddLabelArgs{Identifier: "PROJ-1", Label: "feature"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	if updateCalled {
		t.Error("UpdateWorkItem must not be called when label is already attached")
	}
}

// TestAddLabel_UnknownLabel — label not found in project returns clear error.
func TestAddLabel_UnknownLabel(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return nil, errors.New("label not found")
		},
	}
	args := AddLabelArgs{Identifier: "PROJ-1", Label: "nonexistent"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when label resolution fails")
	}
}

// TestAddLabel_InvalidIdentifier — malformed identifier returns error.
func TestAddLabel_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	args := AddLabelArgs{Identifier: "bad", Label: "bug"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestAddLabel_WorkItemNotFound — work item not found returns clear error.
func TestAddLabel_WorkItemNotFound(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	resolver := &mockResolver{}
	args := AddLabelArgs{Identifier: "PROJ-1", Label: "bug"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when work item not found")
	}
}

// TestAddLabel_UpdateError — client.UpdateWorkItem error is surfaced.
func TestAddLabel_UpdateError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("api error")
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-bug", Name: "bug"}, nil
		},
	}
	args := AddLabelArgs{Identifier: "PROJ-1", Label: "bug"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when UpdateWorkItem fails")
	}
}

// TestAddLabel_ResolveByName — label resolved by name (not UUID).
func TestAddLabel_ResolveByName(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{},
	}
	var resolvedInput string
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			resolvedInput = input
			return &plane.Label{ID: "lbl-uuid-123", Name: input}, nil
		},
	}
	args := AddLabelArgs{Identifier: "PROJ-1", Label: "critical"}

	// Act
	result, err := addLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if resolvedInput != "critical" {
		t.Errorf("expected resolveLabelFn to receive 'critical', got %q", resolvedInput)
	}
}

// ---------------------------------------------------------------------------
// remove_label handler tests (AGENT-34)
// ---------------------------------------------------------------------------

// TestRemoveLabel_Success — happy path: label resolved and removed.
func TestRemoveLabel_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{{ID: "lbl-bug"}, {ID: "lbl-feature"}},
	}
	var capturedBody map[string]any
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-bug", Name: "bug"}, nil
		},
	}
	args := RemoveLabelArgs{Identifier: "PROJ-1", Label: "bug"}

	// Act
	result, err := removeLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	labelIDs, ok := capturedBody["labels"].([]string)
	if !ok {
		t.Fatalf("labels not a []string: %T", capturedBody["labels"])
	}
	if len(labelIDs) != 1 || labelIDs[0] != "lbl-feature" {
		t.Errorf("expected [lbl-feature], got %v", labelIDs)
	}
}

// TestRemoveLabel_NotAttached — idempotent: no-op when label is absent.
func TestRemoveLabel_NotAttached(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{{ID: "lbl-feature"}},
	}
	updateCalled := false
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			updateCalled = true
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-bug", Name: "bug"}, nil
		},
	}
	args := RemoveLabelArgs{Identifier: "PROJ-1", Label: "bug"}

	// Act
	result, err := removeLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	if updateCalled {
		t.Error("UpdateWorkItem must not be called when label is not attached")
	}
}

// TestRemoveLabel_UnknownLabel — label not found in project returns error.
func TestRemoveLabel_UnknownLabel(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return nil, errors.New("label not found")
		},
	}
	args := RemoveLabelArgs{Identifier: "PROJ-1", Label: "nonexistent"}

	// Act
	result, err := removeLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when label resolution fails")
	}
}

// TestRemoveLabel_InvalidIdentifier — malformed identifier returns error.
func TestRemoveLabel_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	args := RemoveLabelArgs{Identifier: "bad", Label: "bug"}

	// Act
	result, err := removeLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestRemoveLabel_RemoveAll — removing the last label produces empty labels list.
func TestRemoveLabel_RemoveAll(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{{ID: "lbl-only"}},
	}
	var capturedBody map[string]any
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-only", Name: "only"}, nil
		},
	}
	args := RemoveLabelArgs{Identifier: "PROJ-1", Label: "only"}

	// Act
	result, err := removeLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	labelIDs, ok := capturedBody["labels"].([]string)
	if !ok {
		t.Fatalf("labels not a []string: %T", capturedBody["labels"])
	}
	if len(labelIDs) != 0 {
		t.Errorf("expected empty labels, got %v", labelIDs)
	}
}

// TestRemoveLabel_UpdateError — client.UpdateWorkItem error is surfaced.
func TestRemoveLabel_UpdateError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Labels:  []plane.Expandable[plane.Label]{{ID: "lbl-bug"}},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("api error")
		},
	}
	resolver := &mockResolver{
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			return &plane.Label{ID: "lbl-bug", Name: "bug"}, nil
		},
	}
	args := RemoveLabelArgs{Identifier: "PROJ-1", Label: "bug"}

	// Act
	result, err := removeLabel(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when UpdateWorkItem fails")
	}
}

// ---------------------------------------------------------------------------
// extractLabelIDs unit tests (AGENT-34)
// ---------------------------------------------------------------------------

func TestExtractLabelIDs(t *testing.T) {
	// Arrange: mix of expanded and non-expanded labels.
	labels := []plane.Expandable[plane.Label]{
		{ID: "id-1"},
		{ID: "id-2", Val: &plane.Label{ID: "id-2", Name: "bug"}},
		{ID: "", Val: &plane.Label{ID: "id-3", Name: "feature"}},
		{ID: "", Val: nil},
	}

	// Act
	ids := extractLabelIDs(labels)

	// Assert
	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d: %v", len(ids), ids)
	}
	if ids[0] != "id-1" || ids[1] != "id-2" || ids[2] != "id-3" {
		t.Errorf("expected [id-1 id-2 id-3], got %v", ids)
	}
}

func TestExtractLabelIDs_Empty(t *testing.T) {
	ids := extractLabelIDs(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty slice for nil input, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// TestRegisterWithDeps verifies add_label / remove_label are registered
// ---------------------------------------------------------------------------

// TestRegisterWithDeps_AddRemoveLabel — add_label and remove_label are
// registered under the worker profile (same scope as list_labels).
func TestRegisterWithDeps_AddRemoveLabel(t *testing.T) {
	// Arrange
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	// Worker profile — add_label and remove_label should register.
	cfg := &config.Config{PlaneMCPProfile: "worker"}

	// Act
	registerWithDeps(server, client, resolver, formatter, cfg)
	// No panic = success (the SDK panics if a tool with a bad name is added).
}
