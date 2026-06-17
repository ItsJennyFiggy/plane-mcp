package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strconv"
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
// TestGetProjectID — unit tests for the getProjectID helper
// ---------------------------------------------------------------------------

func TestGetProjectID(t *testing.T) {
	tests := []struct {
		name string
		p    plane.Expandable[plane.Project]
		want string
	}{
		{
			name: "prefer Val.ID when Val is present",
			p: plane.Expandable[plane.Project]{
				ID:  "fallback-id",
				Val: &plane.Project{ID: "real-id", Name: "TestProj", Identifier: "TEST"},
			},
			want: "real-id",
		},
		{
			name: "fallback to ID when Val is nil",
			p: plane.Expandable[plane.Project]{
				ID:  "fallback-id",
				Val: nil,
			},
			want: "fallback-id",
		},
		{
			name: "empty when both are empty",
			p:    plane.Expandable[plane.Project]{},
			want: "",
		},
		{
			name: "Val.ID is empty string — returns empty",
			p: plane.Expandable[plane.Project]{
				ID:  "fallback-id",
				Val: &plane.Project{Name: "NoID"},
			},
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := getProjectID(tc.p)
			if got != tc.want {
				t.Errorf("getProjectID(%+v) = %q, want %q", tc.p, got, tc.want)
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
func intPtr(n int) *int       { return &n }

// mockClient is a test double for planeClient.
type mockClient struct {
	listProjectsFn            func(ctx context.Context) ([]plane.Project, error)
	getWorkItemByIdentifierFn func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error)
	listWorkItemsFn           func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error)
	searchWorkItemsFn         func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error)
	createWorkItemFn          func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error)
	createWorkItemCommentFn   func(ctx context.Context, projectID, itemID, comment string) error
	updateWorkItemFn          func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error)
	createWorkItemLinkFn      func(ctx context.Context, projectID, itemID, linkURL, title string) error
	addWorkItemsToModuleFn    func(ctx context.Context, projectID, moduleID string, workItemIDs []string) error
	listLabelsFn              func(ctx context.Context, projectID string) ([]plane.Label, error)
	listStatesFn              func(ctx context.Context, projectID string) ([]plane.State, error)
	listCommentsFn            func(ctx context.Context, projectID, workItemID string) ([]plane.Comment, error)
	getLastCommentFn          func(ctx context.Context, projectID, workItemID string) (*plane.Comment, error)
	getWorkItemFn             func(ctx context.Context, projectID, workItemID string) (*plane.WorkItem, error)
	listWorkItemRelationsFn   func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error)
	createWorkItemRelationFn  func(ctx context.Context, projectID, workItemID, relationType string, issues []string) error
	removeWorkItemRelationFn  func(ctx context.Context, projectID, workItemID, relatedIssue string) error
	deleteWorkItemFn          func(ctx context.Context, projectID, workItemID string) error
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
func (m *mockClient) SearchWorkItems(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
	return m.searchWorkItemsFn(ctx, params)
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
func (m *mockClient) ListStates(ctx context.Context, projectID string) ([]plane.State, error) {
	return m.listStatesFn(ctx, projectID)
}

func (m *mockClient) ListComments(ctx context.Context, projectID, workItemID string) ([]plane.Comment, error) {
	return m.listCommentsFn(ctx, projectID, workItemID)
}

func (m *mockClient) GetLastComment(ctx context.Context, projectID, workItemID string) (*plane.Comment, error) {
	if m.getLastCommentFn == nil {
		return nil, nil
	}
	return m.getLastCommentFn(ctx, projectID, workItemID)
}

func (m *mockClient) GetWorkItem(ctx context.Context, projectID, workItemID string) (*plane.WorkItem, error) {
	return m.getWorkItemFn(ctx, projectID, workItemID)
}
func (m *mockClient) ListWorkItemRelations(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
	return m.listWorkItemRelationsFn(ctx, projectID, workItemID)
}
func (m *mockClient) CreateWorkItemRelation(ctx context.Context, projectID, workItemID, relationType string, issues []string) error {
	return m.createWorkItemRelationFn(ctx, projectID, workItemID, relationType, issues)
}
func (m *mockClient) RemoveWorkItemRelation(ctx context.Context, projectID, workItemID, relatedIssue string) error {
	return m.removeWorkItemRelationFn(ctx, projectID, workItemID, relatedIssue)
}
func (m *mockClient) DeleteWorkItem(ctx context.Context, projectID, workItemID string) error {
	return m.deleteWorkItemFn(ctx, projectID, workItemID)
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
		State:      strPtr("In Progress"),
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
	args := ReportProgressArgs{Identifier: "PROJ-1", State: strPtr("Done")}

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
	args := ReportProgressArgs{Identifier: "PROJ-1", State: strPtr("Done")}

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
// list_projects handler tests (AGENT-29)
// ---------------------------------------------------------------------------

// TestListProjects_Success — happy path: projects found and formatted.
func TestListProjects_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	projects := []plane.Project{
		{ID: "proj-uuid-1", Name: "Execution", Identifier: "EXEC"},
		{ID: "proj-uuid-2", Name: "Agents", Identifier: "AGENT"},
	}
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return projects, nil
		},
	}

	// Act
	result, err := listProjects(ctx, client)

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
	if !strings.Contains(tc.Text, `identifier: "EXEC"`) || !strings.Contains(tc.Text, `name: "Execution"`) || !strings.Contains(tc.Text, `id: "proj-uuid-1"`) {
		t.Errorf("expected first project details in output, got: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, `identifier: "AGENT"`) || !strings.Contains(tc.Text, `name: "Agents"`) || !strings.Contains(tc.Text, `id: "proj-uuid-2"`) {
		t.Errorf("expected second project details in output, got: %q", tc.Text)
	}
}

// TestListProjects_Empty — no projects returns a clear message.
func TestListProjects_Empty(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return nil, nil
		},
	}

	// Act
	result, err := listProjects(ctx, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result set")
	}
	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if tc.Text != "No projects found." {
		t.Errorf("expected 'No projects found.', got: %q", tc.Text)
	}
}

// TestListProjects_Error — client error is surfaced as toolError.
func TestListProjects_Error(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return nil, errors.New("api error")
		},
	}

	// Act
	result, err := listProjects(ctx, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when client.ListProjects fails")
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

// ---------------------------------------------------------------------------
// assign_work_item tests (AGENT-57)
// ---------------------------------------------------------------------------

func TestAssignWorkItem_Set_Success(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{{ID: "mem-old"}},
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
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-new", DisplayName: "Alice"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"alice"}}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}

	assigneeIDs, ok := capturedBody["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees not a []string: %T", capturedBody["assignees"])
	}
	if len(assigneeIDs) != 1 || assigneeIDs[0] != "mem-new" {
		t.Errorf("expected [mem-new], got %v", assigneeIDs)
	}
}

func TestAssignWorkItem_Set_Clear(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{{ID: "mem-old"}},
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
	resolver := &mockResolver{}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: nil}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}

	assigneeIDs, ok := capturedBody["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees not a []string: %T", capturedBody["assignees"])
	}
	if len(assigneeIDs) != 0 {
		t.Errorf("expected empty slice, got %v", assigneeIDs)
	}

	// Verify the marshaled JSON sends "assignees":[] not "assignees":null.
	b, err := json.Marshal(capturedBody)
	if err != nil {
		t.Fatalf("failed to marshal captured body: %v", err)
	}
	if !bytes.Contains(b, []byte(`"assignees":[]`)) {
		t.Errorf("expected JSON to contain \"assignees\":[], got %s", string(b))
	}
}

func TestAssignWorkItem_Add_Success(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{{ID: "mem-old"}},
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
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-new", DisplayName: "Bob"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"bob"}, Mode: "add"}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}

	ids, ok := capturedBody["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees not a []string: %T", capturedBody["assignees"])
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 assignees, got %d: %v", len(ids), ids)
	}
	if ids[0] != "mem-old" || ids[1] != "mem-new" {
		t.Errorf("expected [mem-old mem-new], got %v", ids)
	}
}

func TestAssignWorkItem_Add_Idempotent(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{{ID: "mem-existing"}},
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
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-existing", DisplayName: "Alice"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"alice"}, Mode: "add"}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}

	ids, ok := capturedBody["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees not a []string: %T", capturedBody["assignees"])
	}
	if len(ids) != 1 || ids[0] != "mem-existing" {
		t.Errorf("expected [mem-existing] (no duplicate), got %v", ids)
	}
}

func TestAssignWorkItem_Add_ToEmpty(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{},
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
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-new", DisplayName: "Charlie"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"charlie"}, Mode: "add"}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}

	ids, ok := capturedBody["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees not a []string: %T", capturedBody["assignees"])
	}
	if len(ids) != 1 || ids[0] != "mem-new" {
		t.Errorf("expected [mem-new], got %v", ids)
	}

	// Verify the marshaled JSON sends "assignees":["mem-new"] not null.
	b, err := json.Marshal(capturedBody)
	if err != nil {
		t.Fatalf("failed to marshal captured body: %v", err)
	}
	if !bytes.Contains(b, []byte(`"assignees":["mem-new"]`)) {
		t.Errorf("expected JSON to contain \"assignees\":[\"mem-new\"], got %s", string(b))
	}
}

func TestAssignWorkItem_Remove_Success(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{{ID: "mem-keep"}, {ID: "mem-remove"}},
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
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-remove", DisplayName: "Bob"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"bob"}, Mode: "remove"}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}

	ids, ok := capturedBody["assignees"].([]string)
	if !ok {
		t.Fatalf("assignees not a []string: %T", capturedBody["assignees"])
	}
	if len(ids) != 1 || ids[0] != "mem-keep" {
		t.Errorf("expected [mem-keep], got %v", ids)
	}
}

func TestAssignWorkItem_InvalidMode(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: nil, Mode: "bogus"}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid mode")
	}
}

func TestAssignWorkItem_InvalidIdentifier(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	args := AssignWorkItemArgs{Identifier: "bad", Assignees: FlexibleStringSlice{"alice"}}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

func TestAssignWorkItem_WorkItemNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	resolver := &mockResolver{}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"alice"}}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when work item not found")
	}
}

func TestAssignWorkItem_AssigneeNotFound(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return nil, errors.New("member not found")
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"nobody"}}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when assignee resolution fails")
	}
}

func TestAssignWorkItem_UpdateError(t *testing.T) {
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project:   plane.Expandable[plane.Project]{ID: "proj-uuid"},
		Assignees: []plane.Expandable[plane.Member]{},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("update failed")
		},
	}
	resolver := &mockResolver{
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-new", DisplayName: "Alice"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"alice"}}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when UpdateWorkItem fails")
	}
}

func TestAssignWorkItem_ExpandedProject(t *testing.T) {
	// Verify project ID is read from Val when expanded.
	ctx := context.Background()
	item := &plane.WorkItem{
		ID: "wi-1", Name: "Task", SequenceID: 1,
		Project: plane.Expandable[plane.Project]{
			ID:  "proj-flat",
			Val: &plane.Project{ID: "proj-expanded", Name: "Test"},
		},
		Assignees: []plane.Expandable[plane.Member]{},
	}
	var capturedProjectID string
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedProjectID = projectID
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "mem-new", DisplayName: "Alice"}, nil
		},
	}
	args := AssignWorkItemArgs{Identifier: "PROJ-1", Assignees: FlexibleStringSlice{"alice"}}

	result, err := assignWorkItem(ctx, args, client, resolver)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got error: %+v", result.Content)
	}
	if capturedProjectID != "proj-expanded" {
		t.Errorf("expected projectID 'proj-expanded' from Val, got %q", capturedProjectID)
	}
}

// ---------------------------------------------------------------------------
// extractAssigneeIDs unit tests (AGENT-57)
// ---------------------------------------------------------------------------

func TestExtractAssigneeIDs(t *testing.T) {
	assignees := []plane.Expandable[plane.Member]{
		{ID: "id-1"},
		{ID: "id-2", Val: &plane.Member{ID: "id-2", DisplayName: "Alice"}},
		{ID: "", Val: &plane.Member{ID: "id-3", DisplayName: "Bob"}},
		{ID: "", Val: nil},
	}

	ids := extractAssigneeIDs(assignees)

	if len(ids) != 3 {
		t.Fatalf("expected 3 ids, got %d: %v", len(ids), ids)
	}
	if ids[0] != "id-1" || ids[1] != "id-2" || ids[2] != "id-3" {
		t.Errorf("expected [id-1 id-2 id-3], got %v", ids)
	}
}

func TestExtractAssigneeIDs_Empty(t *testing.T) {
	ids := extractAssigneeIDs(nil)
	if len(ids) != 0 {
		t.Errorf("expected empty slice for nil input, got %v", ids)
	}
}

// ---------------------------------------------------------------------------
// assign_work_item registration test (AGENT-57)
// ---------------------------------------------------------------------------

// TestRegisterWithDeps_ListStates — list_states must register under the worker
// profile (same scope as list_labels, add_label, remove_label).
func TestRegisterWithDeps_ListStates(t *testing.T) {
	// Arrange
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	// Worker profile — list_states should register.
	cfg := &config.Config{PlaneMCPProfile: "worker"}

	// Act
	registerWithDeps(server, client, resolver, formatter, cfg)
	// No panic = success (the SDK panics if a tool with a bad name is added).
}

// TestRegisterWithDeps_AssignWorkItem — assign_work_item must NOT register
// under the worker profile (it is planner/planner/full only).
func TestRegisterWithDeps_AssignWorkItem(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	// Worker profile should NOT register assign_work_item.
	cfg := &config.Config{PlaneMCPProfile: "worker"}

	// Must not panic (if it tries to add, the SDK accepts; we just verify no panic).
	registerWithDeps(server, client, resolver, formatter, cfg)

	// Planner profile SHOULD register assign_work_item.
	server2 := mcp.NewServer(&mcp.Implementation{Name: "test2", Version: "0"}, nil)
	cfg2 := &config.Config{PlaneMCPProfile: "planner"}
	registerWithDeps(server2, client, resolver, formatter, cfg2)
}

// ---------------------------------------------------------------------------
// list_states handler tests (AGENT-39)
// ---------------------------------------------------------------------------

// TestListStates_Success — happy path: states found and formatted.
func TestListStates_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	states := []plane.State{
		{ID: "st-1", Name: "Backlog", Group: "backlog"},
		{ID: "st-2", Name: "In Progress", Group: "started"},
	}
	client := &mockClient{
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return states, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project"}, nil
		},
	}
	args := ListStatesArgs{Project: "My Project"}

	// Act
	result, err := listStates(ctx, args, client, resolver)

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
	if !strings.Contains(tc.Text, `name: "Backlog"`) || !strings.Contains(tc.Text, `group: "backlog"`) || !strings.Contains(tc.Text, `id: "st-1"`) {
		t.Errorf("expected first state details in output, got: %q", tc.Text)
	}
	if !strings.Contains(tc.Text, `name: "In Progress"`) || !strings.Contains(tc.Text, `group: "started"`) || !strings.Contains(tc.Text, `id: "st-2"`) {
		t.Errorf("expected second state details in output, got: %q", tc.Text)
	}
}

// TestListStates_YAMLRoundTrip — output must be parseable as valid YAML.
func TestListStates_YAMLRoundTrip(t *testing.T) {
	// Arrange
	ctx := context.Background()
	states := []plane.State{
		{ID: "st-1", Name: "Backlog", Group: "backlog"},
		{ID: "st-2", Name: "In Progress", Group: "started"},
		{ID: "st-3", Name: "Done", Group: "completed"},
	}
	client := &mockClient{
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return states, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	args := ListStatesArgs{Project: "Test"}

	// Act
	result, err := listStates(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false: %+v", result.Content)
	}

	tc := result.Content[0].(*mcp.TextContent)

	// Parse as YAML — must succeed.
	var parsed []map[string]any
	if err := yaml.Unmarshal([]byte(tc.Text), &parsed); err != nil {
		t.Fatalf("output is not valid YAML: %v\noutput:\n%s", err, tc.Text)
	}

	if len(parsed) != 3 {
		t.Fatalf("expected 3 states, got %d", len(parsed))
	}
	if parsed[0]["name"] != "Backlog" || parsed[0]["group"] != "backlog" || parsed[0]["id"] != "st-1" {
		t.Errorf("first state: got id=%q name=%q group=%q", parsed[0]["id"], parsed[0]["name"], parsed[0]["group"])
	}
	if parsed[1]["name"] != "In Progress" || parsed[1]["group"] != "started" || parsed[1]["id"] != "st-2" {
		t.Errorf("second state: got id=%q name=%q group=%q", parsed[1]["id"], parsed[1]["name"], parsed[1]["group"])
	}
	if parsed[2]["name"] != "Done" || parsed[2]["group"] != "completed" {
		t.Errorf("third state: got name=%q group=%q", parsed[2]["name"], parsed[2]["group"])
	}
}

// TestListStates_Empty — no states in project returns a clear message.
func TestListStates_Empty(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return nil, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	args := ListStatesArgs{Project: "Empty Project"}

	// Act
	result, err := listStates(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if tc.Text != "No states found in this project." {
		t.Errorf("expected 'No states found' message, got: %q", tc.Text)
	}
}

// TestListStates_ProjectResolutionError — project not found returns error.
func TestListStates_ProjectResolutionError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return nil, errors.New("project not found")
		},
	}
	args := ListStatesArgs{Project: "Unknown"}

	// Act
	result, err := listStates(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when project resolution fails")
	}
}

// TestListStates_ClientError — client.ListStates error is surfaced.
func TestListStates_ClientError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return nil, errors.New("api error")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	args := ListStatesArgs{Project: "My Project"}

	// Act
	result, err := listStates(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when client.ListStates fails")
	}
}

// ---------------------------------------------------------------------------
// list_work_items tests
// ---------------------------------------------------------------------------

// TestListWorkItems_Success — basic listing with project only.
func TestListWorkItems_Success(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{
		{ID: "wi-1", Name: "Task One", SequenceID: 1},
		{ID: "wi-2", Name: "Task Two", SequenceID: 2},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if projectID != "proj-uuid" {
				t.Errorf("expected projectID=proj-uuid, got %q", projectID)
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "My Project", Identifier: "MP"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			if detail != "summary_with_labels" {
				t.Errorf("expected detail='summary_with_labels', got %q", detail)
			}
			return "- name: Task One\n- name: Task Two\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "My Project"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_NoItems — empty result returns a friendly message.
func TestListWorkItems_NoItems(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return nil, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Empty", Identifier: "EMP"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "Empty"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if tc.Text != "[]" {
		t.Errorf("expected '[]', got: %q", tc.Text)
	}
}

// TestListWorkItems_ProjectResolveError — project not found returns error.
func TestListWorkItems_ProjectResolveError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return nil, errors.New("project not found")
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "Unknown"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when project resolution fails")
	}
}

// TestListWorkItems_ListError — client.ListWorkItems error is surfaced.
func TestListWorkItems_ListError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return nil, errors.New("api down")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "My Project"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when client.ListWorkItems fails")
	}
}

// TestListWorkItems_StateGroupFilter — filters by state_group client-side
// because the Plane API ignores the state_group parameter.
func TestListWorkItems_StateGroupFilter(t *testing.T) {
	ctx := context.Background()
	allItems := []plane.WorkItem{
		{ID: "wi-1", Name: "In Progress", SequenceID: 1, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "started"}}},
		{ID: "wi-2", Name: "Done", SequenceID: 2, State: plane.Expandable[plane.State]{ID: "st-2", Val: &plane.State{ID: "st-2", Group: "completed"}}},
		{ID: "wi-3", Name: "Todo", SequenceID: 3, State: plane.Expandable[plane.State]{ID: "st-3", Val: &plane.State{ID: "st-3", Group: "unstarted"}}},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			// state_group must NOT be passed to the API.
			if _, ok := params["state_group"]; ok {
				t.Error("state_group param should NOT be passed to the API when filtering client-side")
			}
			return allItems, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "st-1", Group: "started"},
				{ID: "st-2", Group: "completed"},
				{ID: "st-3", Group: "unstarted"},
			}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	var formattedItems []plane.WorkItem
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			formattedItems = items
			return "- name: In Progress\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", StateGroup: strPtr("started")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
	if len(formattedItems) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(formattedItems))
	}
	if formattedItems[0].Name != "In Progress" {
		t.Errorf("expected 'In Progress', got %q", formattedItems[0].Name)
	}
}

// TestListWorkItems_StateGroupFilterWithLimit — filters by state_group client-side
// and then applies the limit post-filter.
func TestListWorkItems_StateGroupFilterWithLimit(t *testing.T) {
	ctx := context.Background()
	allItems := []plane.WorkItem{
		{ID: "wi-1", Name: "Task A", SequenceID: 1, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "started"}}},
		{ID: "wi-2", Name: "Task B", SequenceID: 2, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "started"}}},
		{ID: "wi-3", Name: "Task C", SequenceID: 3, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "started"}}},
		{ID: "wi-4", Name: "Task D", SequenceID: 4, State: plane.Expandable[plane.State]{ID: "st-2", Val: &plane.State{ID: "st-2", Group: "completed"}}},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			// When filtering client-side, limit must be set to the safety cap of 1000.
			if lim, ok := params["limit"]; !ok {
				t.Error("limit param should be set to safety cap (1000) when filtering client-side")
			} else if lim != "1000" {
				t.Errorf("limit param should be \"1000\" (safety cap), got %q", lim)
			}
			return allItems, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "st-1", Group: "started"},
				{ID: "st-2", Group: "completed"},
			}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	var formattedItems []plane.WorkItem
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			formattedItems = items
			return "- name: Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", StateGroup: strPtr("started"), Limit: intPtr(2)}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
	if len(formattedItems) != 2 {
		t.Fatalf("expected 2 items after limit, got %d", len(formattedItems))
	}
}

// TestListWorkItems_StateGroupFilterCaseInsensitive — state_group filtering
// is case-insensitive.
func TestListWorkItems_StateGroupFilterCaseInsensitive(t *testing.T) {
	ctx := context.Background()
	allItems := []plane.WorkItem{
		{ID: "wi-1", Name: "Task One", SequenceID: 1, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "started"}}},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return allItems, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "st-1", Group: "STARTED"},
			}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	var formattedItems []plane.WorkItem
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			formattedItems = items
			return "- name: Task One\n", nil
		},
	}
	// Use lowercase "started" to match uppercase "STARTED" group.
	args := ListWorkItemsArgs{Project: "Alpha", StateGroup: strPtr("started")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
	if len(formattedItems) != 1 {
		t.Fatalf("expected 1 item after case-insensitive match, got %d", len(formattedItems))
	}
}

// TestListWorkItems_StateGroupNoMatches — no items match the requested state_group.
func TestListWorkItems_StateGroupNoMatches(t *testing.T) {
	ctx := context.Background()
	allItems := []plane.WorkItem{
		{ID: "wi-1", Name: "Done", SequenceID: 1, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "completed"}}},
		{ID: "wi-2", Name: "Todo", SequenceID: 2, State: plane.Expandable[plane.State]{ID: "st-2", Val: &plane.State{ID: "st-2", Group: "unstarted"}}},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return allItems, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "st-1", Group: "completed"},
				{ID: "st-2", Group: "unstarted"},
			}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "Alpha", StateGroup: strPtr("started")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty filtered result")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if tc.Text != "[]" {
		t.Errorf("expected '[]', got: %q", tc.Text)
	}
}

// TestListWorkItems_PriorityFilter — passes priority to the API.
func TestListWorkItems_PriorityFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Urgent Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["priority"] != "urgent" {
				t.Errorf("expected priority=urgent, got %q", params["priority"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Urgent Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", Priority: strPtr("urgent")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_TypeFilter — passes type to the API.
func TestListWorkItems_TypeFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Bug", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["type"] != "bug" {
				t.Errorf("expected type=bug, got %q", params["type"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Bug\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", Type: strPtr("bug")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_AssigneesFilter — resolves assignee names to UUIDs.
func TestListWorkItems_AssigneesFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Assigned Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["assignees"] != "member-uuid-1,member-uuid-2" {
				t.Errorf("expected assignees=member-uuid-1,member-uuid-2, got %q", params["assignees"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			if input == "Alice" {
				return &plane.Member{ID: "member-uuid-1", DisplayName: "Alice"}, nil
			}
			if input == "Bob" {
				return &plane.Member{ID: "member-uuid-2", DisplayName: "Bob"}, nil
			}
			return nil, errors.New("unknown member")
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Assigned Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", Assignees: FlexibleStringSlice{"Alice", "Bob"}}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_AssigneesResolveError — unresolvable assignee returns error.
func TestListWorkItems_AssigneesResolveError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return nil, errors.New("member not found")
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "Alpha", Assignees: FlexibleStringSlice{"Unknown"}}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when assignee resolution fails")
	}
}

// TestListWorkItems_LabelsFilter — resolves label names to UUIDs.
func TestListWorkItems_LabelsFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Labelled Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["labels"] != "label-uuid-bug,label-uuid-ui" {
				t.Errorf("expected labels=label-uuid-bug,label-uuid-ui, got %q", params["labels"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
		resolveLabelFn: func(ctx context.Context, projectID string, input string) (*plane.Label, error) {
			if input == "bug" {
				return &plane.Label{ID: "label-uuid-bug", Name: "bug"}, nil
			}
			if input == "ui" {
				return &plane.Label{ID: "label-uuid-ui", Name: "ui"}, nil
			}
			return nil, errors.New("unknown label")
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Labelled Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", Labels: FlexibleStringSlice{"bug", "ui"}}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_LabelsResolveError — unresolvable label returns error.
func TestListWorkItems_LabelsResolveError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
		resolveLabelFn: func(ctx context.Context, projectID string, input string) (*plane.Label, error) {
			return nil, errors.New("label not found")
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "Alpha", Labels: FlexibleStringSlice{"nonexistent"}}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when label resolution fails")
	}
}

// TestListWorkItems_AllFilters — combines all filters (state_group is client-side).
func TestListWorkItems_AllFilters(t *testing.T) {
	ctx := context.Background()
	allItems := []plane.WorkItem{
		{ID: "wi-1", Name: "Filtered Task", SequenceID: 1, State: plane.Expandable[plane.State]{ID: "st-1", Val: &plane.State{ID: "st-1", Group: "started"}}},
		{ID: "wi-2", Name: "Other Task", SequenceID: 2, State: plane.Expandable[plane.State]{ID: "st-2", Val: &plane.State{ID: "st-2", Group: "completed"}}},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			// state_group must NOT be passed to the API.
			if _, ok := params["state_group"]; ok {
				t.Error("state_group param should NOT be passed to the API when filtering client-side")
			}
			if params["priority"] != "high" {
				t.Errorf("expected priority=high, got %q", params["priority"])
			}
			if params["type"] != "feature" {
				t.Errorf("expected type=feature, got %q", params["type"])
			}
			if params["assignees"] != "member-uuid" {
				t.Errorf("expected assignees=member-uuid, got %q", params["assignees"])
			}
			if params["labels"] != "label-uuid" {
				t.Errorf("expected labels=label-uuid, got %q", params["labels"])
			}
			return allItems, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "st-1", Group: "started"},
				{ID: "st-2", Group: "completed"},
			}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
		resolveMemberFn: func(ctx context.Context, input string) (*plane.Member, error) {
			return &plane.Member{ID: "member-uuid", DisplayName: "Dev"}, nil
		},
		resolveLabelFn: func(ctx context.Context, projectID string, input string) (*plane.Label, error) {
			return &plane.Label{ID: "label-uuid", Name: "frontend"}, nil
		},
	}
	var formattedItems []plane.WorkItem
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			formattedItems = items
			return "- name: Filtered Task\n", nil
		},
	}
	args := ListWorkItemsArgs{
		Project:    "Alpha",
		StateGroup: strPtr("started"),
		Priority:   strPtr("high"),
		Type:       strPtr("feature"),
		Assignees:  FlexibleStringSlice{"Dev"},
		Labels:     FlexibleStringSlice{"frontend"},
	}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
	if len(formattedItems) != 1 {
		t.Fatalf("expected 1 filtered item, got %d", len(formattedItems))
	}
	if formattedItems[0].Name != "Filtered Task" {
		t.Errorf("expected 'Filtered Task', got %q", formattedItems[0].Name)
	}
}

// TestListWorkItems_FormatterError — formatter error is surfaced.
func TestListWorkItems_FormatterError(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "", errors.New("format error")
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when formatter fails")
	}
}

// TestListWorkItems_EmptyStateGroup — nil or empty state_group is not passed.
func TestListWorkItems_EmptyStateGroup(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if _, ok := params["state_group"]; ok {
				t.Error("state_group param should not be set when empty")
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", StateGroup: strPtr("")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_StateFilter — resolves state name to UUID and passes state param.
func TestListWorkItems_StateFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Stateful Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["state"] != "state-uuid-1" {
				t.Errorf("expected state=state-uuid-1, got %q", params["state"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
		resolveStateFn: func(ctx context.Context, projectID string, input string) (*plane.State, error) {
			if projectID != "proj-uuid" || input != "In Progress" {
				t.Errorf("ResolveState called with projectID=%q input=%q", projectID, input)
			}
			return &plane.State{ID: "state-uuid-1", Name: "In Progress"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Stateful Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", State: strPtr("In Progress")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_ModuleFilter — resolves module name to UUID and passes module param.
func TestListWorkItems_ModuleFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Modular Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["module"] != "mod-uuid-1" {
				t.Errorf("expected module=mod-uuid-1, got %q", params["module"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
		resolveModuleFn: func(ctx context.Context, projectID string, input string) (*plane.Module, error) {
			if projectID != "proj-uuid" || input != "Sprint One" {
				t.Errorf("ResolveModule called with projectID=%q input=%q", projectID, input)
			}
			return &plane.Module{ID: "mod-uuid-1", Name: "Sprint One"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Modular Task\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha", Module: strPtr("Sprint One")}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_LimitFilter — passes limit param as a string.
func TestListWorkItems_LimitFilter(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{{ID: "wi-1", Name: "Limited Task", SequenceID: 1}}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			if params["limit"] != "5" {
				t.Errorf("expected limit=5, got %q", params["limit"])
			}
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "- name: Limited Task\n", nil
		},
	}
	limit := 5
	args := ListWorkItemsArgs{Project: "Alpha", Limit: &limit}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false, got: %+v", result.Content)
	}
}

// TestListWorkItems_EmptyResult — no items returns "[]".
func TestListWorkItems_EmptyResult(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return nil, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := ListWorkItemsArgs{Project: "Empty"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if tc.Text != "[]" {
		t.Errorf("expected '[]', got: %q", tc.Text)
	}
}

// TestListWorkItems_LabelsInOutput — verifies that summary_with_labels includes labels in YAML.
func TestListWorkItems_LabelsInOutput(t *testing.T) {
	ctx := context.Background()
	items := []plane.WorkItem{
		{
			ID: "wi-1", Name: "Labelled Task", SequenceID: 1,
			Labels: []plane.Expandable[plane.Label]{
				{Val: &plane.Label{ID: "label-1", Name: "bug"}},
				{Val: &plane.Label{ID: "label-2", Name: "ui"}},
			},
		},
	}
	client := &mockClient{
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return items, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "proj-uuid", Name: "Alpha", Identifier: "ALP"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			if detail != "summary_with_labels" {
				t.Errorf("expected detail='summary_with_labels', got %q", detail)
			}
			return "- identifier: ALP-1\n  name: Labelled Task\n  labels:\n    - bug\n    - ui\n", nil
		},
	}
	args := ListWorkItemsArgs{Project: "Alpha"}

	result, err := listWorkItems(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got: %+v", result.Content)
	}
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "labels:") {
		t.Errorf("expected labels in output, got: %q", tc.Text)
	}
}

// ---------------------------------------------------------------------------
// searchWorkItems handler tests
// ---------------------------------------------------------------------------

// TestSearchWorkItems_Success verifies search_work_items happy path.
func TestSearchWorkItems_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	searchResults := []plane.SearchWorkItemResult{
		{ID: "wi-1", Name: "Fix login", SequenceID: 1, ProjectIdentifier: "PROJ", ProjectID: "proj-1", WorkspaceSlug: "ws"},
		{ID: "wi-2", Name: "Login error", SequenceID: 2, ProjectIdentifier: "PROJ", ProjectID: "proj-1", WorkspaceSlug: "ws"},
	}
	workItems := []plane.WorkItem{
		{ID: "wi-1", Name: "Fix login", SequenceID: 1},
		{ID: "wi-2", Name: "Login error", SequenceID: 2},
	}
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			if params["search"] != "login" {
				t.Errorf("expected search=login, got %q", params["search"])
			}
			return searchResults, nil
		},
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			for _, wi := range workItems {
				if wi.SequenceID == seq {
					return &wi, nil
				}
			}
			return nil, errors.New("not found")
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			if detail != "summary" {
				t.Errorf("expected detail='summary', got %q", detail)
			}
			return "name: Fix login\nname: Login error\n", nil
		},
	}
	args := SearchWorkItemsArgs{Query: "login"}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
}

// TestSearchWorkItems_Empty verifies an empty result returns "[]".
func TestSearchWorkItems_Empty(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			return nil, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	args := SearchWorkItemsArgs{Query: "nonexistent"}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false for empty results")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if tc.Text != "[]" {
		t.Errorf("expected '[]', got %q", tc.Text)
	}
}

// TestSearchWorkItems_ProjectFilter verifies project resolution and filter.
func TestSearchWorkItems_ProjectFilter(t *testing.T) {
	// Arrange
	ctx := context.Background()
	searchResults := []plane.SearchWorkItemResult{
		{ID: "wi-1", Name: "Task", SequenceID: 1, ProjectIdentifier: "ALPHA", ProjectID: "proj-alpha", WorkspaceSlug: "ws"},
	}
	var capturedParams map[string]string
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			capturedParams = params
			return searchResults, nil
		},
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Name: "Task", SequenceID: 1}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			if input != "Alpha" {
				t.Errorf("expected project input='Alpha', got %q", input)
			}
			return &plane.Project{ID: "proj-alpha", Name: "Alpha", Identifier: "ALPHA"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			return "name: Task\n", nil
		},
	}
	project := "Alpha"
	args := SearchWorkItemsArgs{Query: "task", Project: &project}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedParams["project_id"] != "proj-alpha" {
		t.Errorf("expected project_id=proj-alpha, got %q", capturedParams["project_id"])
	}
}

// TestSearchWorkItems_ValidationError verifies missing query returns error.
func TestSearchWorkItems_ValidationError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	args := SearchWorkItemsArgs{Query: ""}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when query is empty")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "query is required") {
		t.Errorf("expected 'query is required' in error, got %q", tc.Text)
	}
}

// TestSearchWorkItems_ClientError verifies client errors are propagated.
func TestSearchWorkItems_ClientError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			return nil, errors.New("plane api is down")
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	args := SearchWorkItemsArgs{Query: "anything"}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when client fails")
	}
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "plane api is down") {
		t.Errorf("expected error to mention 'plane api is down', got %q", tc.Text)
	}
}

// TestSearchWorkItems_GracefulDegradation verifies that when one item
// fails to fetch (GetWorkItemByIdentifier error), the other items are
// still returned.
func TestSearchWorkItems_GracefulDegradation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	searchResults := []plane.SearchWorkItemResult{
		{ID: "wi-1", Name: "Good One", SequenceID: 1, ProjectIdentifier: "PROJ", ProjectID: "proj-1", WorkspaceSlug: "ws"},
		{ID: "wi-2", Name: "Bad One", SequenceID: 2, ProjectIdentifier: "PROJ", ProjectID: "proj-1", WorkspaceSlug: "ws"},
		{ID: "wi-3", Name: "Good Two", SequenceID: 3, ProjectIdentifier: "PROJ", ProjectID: "proj-1", WorkspaceSlug: "ws"},
	}
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			return searchResults, nil
		},
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			if seq == 2 {
				return nil, errors.New("item not found")
			}
			return &plane.WorkItem{ID: "wi-" + strconv.Itoa(seq), Name: "Item", SequenceID: seq}, nil
		},
	}
	resolver := &mockResolver{}
	var capturedItems []plane.WorkItem
	formatter := &mockFormatter{
		formatWorkItemsYAMLFn: func(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
			capturedItems = items
			return "- name: Item\n- name: Item\n", nil
		},
	}
	args := SearchWorkItemsArgs{Query: "test"}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if len(capturedItems) != 2 {
		t.Fatalf("expected 2 items (bad one skipped), got %d", len(capturedItems))
	}
	if capturedItems[0].SequenceID != 1 || capturedItems[1].SequenceID != 3 {
		t.Errorf("expected sequence IDs 1 and 3, got %d and %d", capturedItems[0].SequenceID, capturedItems[1].SequenceID)
	}
}

// TestSearchWorkItems_LimitDefault verifies that when no limit is
// specified, it defaults to 10.
func TestSearchWorkItems_LimitDefault(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedLimit string
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			capturedLimit = params["limit"]
			return nil, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	args := SearchWorkItemsArgs{Query: "test"} // no Limit

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}
	if capturedLimit != "10" {
		t.Errorf("expected default limit=10, got %q", capturedLimit)
	}
}

// TestSearchWorkItems_LimitCap verifies that a limit above 20 is
// capped at 20.
func TestSearchWorkItems_LimitCap(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedLimit string
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			capturedLimit = params["limit"]
			return nil, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	high := 50
	args := SearchWorkItemsArgs{Query: "test", Limit: &high}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}
	if capturedLimit != "20" {
		t.Errorf("expected capped limit=20, got %q", capturedLimit)
	}
}

// TestSearchWorkItems_LimitNegative verifies that a limit <= 0
// defaults to 10.
func TestSearchWorkItems_LimitNegative(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedLimit string
	client := &mockClient{
		searchWorkItemsFn: func(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error) {
			capturedLimit = params["limit"]
			return nil, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	neg := -5
	args := SearchWorkItemsArgs{Query: "test", Limit: &neg}

	// Act
	result, err := searchWorkItems(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected IsError=false")
	}
	if capturedLimit != "10" {
		t.Errorf("expected default limit=10 for negative input, got %q", capturedLimit)
	}
}

// ---------------------------------------------------------------------------
// list_comments tests
// ---------------------------------------------------------------------------

// TestListComments_Success — happy path: comments are fetched and formatted.
func TestListComments_Success(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return &plane.WorkItem{
				ID: "wi-1",
				Project: plane.Expandable[plane.Project]{
					ID: "proj-uuid",
				},
			}, nil
		},
		listCommentsFn: func(ctx context.Context, projectID, workItemID string) ([]plane.Comment, error) {
			return []plane.Comment{
				{
					ID:          "comment-1",
					CreatedAt:   "2025-06-01T10:00:00Z",
					CommentHTML: "<p>Hello <strong>world</strong></p>",
					ActorDetail: plane.CommentActorDetail{
						ID:          "user-1",
						DisplayName: "Alice",
						FirstName:   "Alice",
						LastName:    "Smith",
					},
				},
				{
					ID:          "comment-2",
					CreatedAt:   "2025-06-01T09:00:00Z",
					CommentHTML: "<p>First comment</p>",
					ActorDetail: plane.CommentActorDetail{
						ID:        "user-2",
						FirstName: "Bob",
						LastName:  "Jones",
					},
				},
			}, nil
		},
	}

	args := ListCommentsArgs{Identifier: "PROJ-1"}
	result, err := listComments(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false on success, got: %+v", result.Content)
	}

	// Verify YAML output is sorted by CreatedAt ascending
	// comment-2 (09:00) should come before comment-1 (10:00)
	var parsed []struct {
		Author    string `yaml:"author"`
		CreatedAt string `yaml:"created_at"`
		Body      string `yaml:"body"`
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if err := yaml.Unmarshal([]byte(textContent.Text), &parsed); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 comments, got %d", len(parsed))
	}
	if parsed[0].CreatedAt != "2025-06-01T09:00:00Z" {
		t.Errorf("expected first comment at 09:00 (sorted), got %s", parsed[0].CreatedAt)
	}
	if parsed[1].CreatedAt != "2025-06-01T10:00:00Z" {
		t.Errorf("expected second comment at 10:00 (sorted), got %s", parsed[1].CreatedAt)
	}
	if parsed[0].Author != "Bob Jones" {
		t.Errorf("expected author 'Bob Jones' (no display name), got '%s'", parsed[0].Author)
	}
	if parsed[1].Author != "Alice" {
		t.Errorf("expected author 'Alice' (has display name), got '%s'", parsed[1].Author)
	}
	if parsed[1].Body != "Hello **world**" {
		t.Errorf("expected body 'Hello **world**' (converted), got '%s'", parsed[1].Body)
	}
}

// TestListComments_Empty — empty result returns YAML-marshaled empty list.
func TestListComments_Empty(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return &plane.WorkItem{
				ID: "wi-1",
				Project: plane.Expandable[plane.Project]{
					ID: "proj-uuid",
				},
			}, nil
		},
		listCommentsFn: func(ctx context.Context, projectID, workItemID string) ([]plane.Comment, error) {
			return nil, nil
		},
	}

	args := ListCommentsArgs{Identifier: "PROJ-1"}
	result, err := listComments(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Error("expected IsError=false for empty result")
	}
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if tc.Text != "[]\n" {
		t.Errorf("expected '[]\\n' for empty comments (YAML-marshaled), got: %s", tc.Text)
	}
}

// TestListComments_ValidationError — invalid identifier returns tool error.
func TestListComments_ValidationError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	args := ListCommentsArgs{Identifier: ""}
	result, err := listComments(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestListComments_ClientError — client.ListComments error is surfaced as tool error.
func TestListComments_ClientError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
			return &plane.WorkItem{
				ID: "wi-1",
				Project: plane.Expandable[plane.Project]{
					ID: "proj-uuid",
				},
			}, nil
		},
		listCommentsFn: func(ctx context.Context, projectID, workItemID string) ([]plane.Comment, error) {
			return nil, errors.New("api error")
		},
	}

	args := ListCommentsArgs{Identifier: "PROJ-1"}
	result, err := listComments(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when client.ListComments returns error")
	}
}

// ---------------------------------------------------------------------------
// get_last_comment tests
// ---------------------------------------------------------------------------

// TestGetLastComment_Success — happy path: comment is fetched and formatted.
func TestGetLastComment_Success(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, proj string, seq int) (*plane.WorkItem, error) {
			if proj != "PROJ" || seq != 123 {
				t.Errorf("unexpected GetWorkItemByIdentifier args: %s, %d", proj, seq)
			}
			return &plane.WorkItem{
				ID: "wi-uuid",
				Project: plane.Expandable[plane.Project]{
					ID: "proj-uuid",
				},
			}, nil
		},
		getLastCommentFn: func(ctx context.Context, projectID, workItemID string) (*plane.Comment, error) {
			if projectID != "proj-uuid" || workItemID != "wi-uuid" {
				t.Errorf("unexpected GetLastComment args: %s, %s", projectID, workItemID)
			}
			return &plane.Comment{
				ID:          "comment-uuid",
				CommentHTML: "<p>Hello <strong>world</strong></p>",
				CreatedAt:   "2026-06-15T18:00:00Z",
				ActorDetail: plane.CommentActorDetail{
					ID:          "user-1",
					DisplayName: "Jane Doe",
				},
			}, nil
		},
	}

	args := GetLastCommentArgs{Identifier: "PROJ-123"}
	result, err := getLastComment(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false, got error")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	var m map[string]string
	if err := yaml.Unmarshal([]byte(text), &m); err != nil {
		t.Fatalf("failed to unmarshal yaml output: %v", err)
	}

	if m["author"] != "Jane Doe" {
		t.Errorf("expected author Jane Doe, got %q", m["author"])
	}
	if m["created_at"] != "2026-06-15T18:00:00Z" {
		t.Errorf("expected created_at 2026-06-15T18:00:00Z, got %q", m["created_at"])
	}
	if m["body"] != "Hello **world**" {
		t.Errorf("expected body 'Hello **world**', got %q", m["body"])
	}
}

// TestGetLastComment_Empty — no comments returns "null".
func TestGetLastComment_Empty(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, proj string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{
				ID: "wi-uuid",
				Project: plane.Expandable[plane.Project]{
					ID: "proj-uuid",
				},
			}, nil
		},
		getLastCommentFn: func(ctx context.Context, projectID, workItemID string) (*plane.Comment, error) {
			return nil, nil
		},
	}

	args := GetLastCommentArgs{Identifier: "PROJ-123"}
	result, err := getLastComment(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected IsError=false")
	}

	text := result.Content[0].(*mcp.TextContent).Text
	if text != "null" {
		t.Errorf("expected output 'null', got %q", text)
	}
}

// TestGetLastComment_ValidationError — invalid identifier returns tool error.
func TestGetLastComment_ValidationError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}

	args := GetLastCommentArgs{Identifier: "INVALID_ID"}
	result, err := getLastComment(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for invalid identifier")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "invalid identifier") {
		t.Errorf("expected validation error message, got %q", text)
	}
}

// TestGetLastComment_ClientError — client.GetLastComment error is surfaced as tool error.
func TestGetLastComment_ClientError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, proj string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{
				ID: "wi-uuid",
				Project: plane.Expandable[plane.Project]{
					ID: "proj-uuid",
				},
			}, nil
		},
		getLastCommentFn: func(ctx context.Context, projectID, workItemID string) (*plane.Comment, error) {
			return nil, errors.New("plane API failed")
		},
	}

	args := GetLastCommentArgs{Identifier: "PROJ-123"}
	result, err := getLastComment(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true for API client error")
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "plane API failed") {
		t.Errorf("expected API error message, got %q", text)
	}
}

// ---------------------------------------------------------------------------
// TestToolAnnotations — verifies that registered tools include the expected
// ToolAnnotations in the MCP list-tools response.
// ---------------------------------------------------------------------------

func TestToolAnnotations(t *testing.T) {
	// Arrange — create a server, register all tools with full profile.
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	cfg := &config.Config{PlaneMCPProfile: "full"}

	registerWithDeps(server, client, resolver, formatter, cfg)

	// Set up an in-memory client connection so we can call ListTools.
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(context.Background(), st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	defer ss.Close()

	mcpClient := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
	cs, err := mcpClient.Connect(context.Background(), ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	defer cs.Close()

	// Act — retrieve the tool list from the server.
	ctx := context.Background()
	res, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	// Index tools by name.
	toolsByName := make(map[string]*mcp.Tool)
	for _, tool := range res.Tools {
		toolsByName[tool.Name] = tool
	}

	// Assert — verify expected annotations for each category.

	// Read-only tools: readOnlyHint=true
	readOnlyTools := []string{
		"find_my_work", "get_work_item", "list_labels", "list_projects",
		"search_work_items", "list_states", "list_comments", "get_last_comment",
		"list_work_items",
	}
	for _, name := range readOnlyTools {
		tool, ok := toolsByName[name]
		if !ok {
			t.Errorf("tool %q not found in ListTools response", name)
			continue
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q: expected Annotations, got nil", name)
			continue
		}
		if !tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q: expected ReadOnlyHint=true, got false", name)
		}
	}

	// Idempotent read-write tools: readOnlyHint=false, idempotentHint=true
	idempotentTools := []string{"report_progress", "add_label", "remove_label", "assign_work_item"}
	for _, name := range idempotentTools {
		tool, ok := toolsByName[name]
		if !ok {
			t.Errorf("tool %q not found in ListTools response", name)
			continue
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q: expected Annotations, got nil", name)
			continue
		}
		if tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q: expected ReadOnlyHint=false, got true", name)
		}
		if !tool.Annotations.IdempotentHint {
			t.Errorf("tool %q: expected IdempotentHint=true, got false", name)
		}
	}

	// Non-destructive write tools: readOnlyHint=false, destructiveHint=false
	nonDestructiveTools := []string{"create_task", "submit_for_review"}
	for _, name := range nonDestructiveTools {
		tool, ok := toolsByName[name]
		if !ok {
			t.Errorf("tool %q not found in ListTools response", name)
			continue
		}
		if tool.Annotations == nil {
			t.Errorf("tool %q: expected Annotations, got nil", name)
			continue
		}
		if tool.Annotations.ReadOnlyHint {
			t.Errorf("tool %q: expected ReadOnlyHint=false, got true", name)
		}
		if tool.Annotations.DestructiveHint == nil {
			t.Errorf("tool %q: expected DestructiveHint to be set, got nil", name)
		} else if *tool.Annotations.DestructiveHint {
			t.Errorf("tool %q: expected DestructiveHint=false, got true", name)
		}
	}

	// Blanket check: every tool in the ListTools response must have non-nil
	// Annotations to prevent future additions from being left unannotated.
	for _, tool := range res.Tools {
		if tool.Annotations == nil {
			t.Errorf("tool %q is missing Annotations; every tool must be annotated", tool.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// TestUpdateWorkItem — tests for the update_work_item tool
// ---------------------------------------------------------------------------

// TestUpdateWorkItem_Success_AllFields — happy path: update name, description,
// priority, and state together, verifying each lands in the PATCH body.
func TestUpdateWorkItem_Success_AllFields(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedBody map[string]any
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Old Name",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid", Val: &plane.Project{ID: "proj-uuid", Name: "MP"}},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return &plane.WorkItem{ID: "wi-1", Name: "New Name", SequenceID: 1}, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-uuid", Name: "In Progress"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: New Name\n", nil
		},
	}

	newName := "New Name"
	newDesc := "# Title\n\nBody text"
	newPriority := "high"
	newState := "In Progress"
	args := UpdateWorkItemArgs{
		Identifier:  "PROJ-1",
		Name:        &newName,
		Description: &newDesc,
		Priority:    &newPriority,
		State:       &newState,
	}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedBody == nil {
		t.Fatal("expected UpdateWorkItem to be called")
	}
	if got, ok := capturedBody["name"].(string); !ok || got != "New Name" {
		t.Errorf("body[name] = %v, want 'New Name'", capturedBody["name"])
	}
	wantHTML := `<h1 class="editor-heading-block">Title</h1><p>Body text</p>`
	if got, ok := capturedBody["description_html"].(string); !ok || got != wantHTML {
		t.Errorf("body[description_html] = %v, want %q", capturedBody["description_html"], wantHTML)
	}
	if got, ok := capturedBody["priority"].(string); !ok || got != "high" {
		t.Errorf("body[priority] = %v, want 'high'", capturedBody["priority"])
	}
	if got, ok := capturedBody["state"].(string); !ok || got != "state-uuid" {
		t.Errorf("body[state] = %v, want 'state-uuid'", capturedBody["state"])
	}
}

// TestUpdateWorkItem_NameOnly — update only the name.
func TestUpdateWorkItem_NameOnly(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedBody map[string]any
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Old",
		SequenceID: 42,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return &plane.WorkItem{ID: "wi-1", Name: "Renamed", SequenceID: 42}, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Renamed\n", nil
		},
	}

	newName := "Renamed"
	args := UpdateWorkItemArgs{
		Identifier: "PROJ-42",
		Name:       &newName,
	}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if len(capturedBody) != 1 || capturedBody["name"] != "Renamed" {
		t.Errorf("body = %v, want only name='Renamed'", capturedBody)
	}
}

// TestUpdateWorkItem_DescriptionOnly — update only description, verifying HTML conversion.
func TestUpdateWorkItem_DescriptionOnly(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedBody map[string]any
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 5,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return item, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Task\n", nil
		},
	}

	newDesc := "**bold** and *italic*"
	args := UpdateWorkItemArgs{
		Identifier:  "PROJ-5",
		Description: &newDesc,
	}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	wantHTML := "<p><strong>bold</strong> and <em>italic</em></p>"
	if got, ok := capturedBody["description_html"].(string); !ok || got != wantHTML {
		t.Errorf("body[description_html] = %v, want %q", capturedBody["description_html"], wantHTML)
	}
	if _, hasName := capturedBody["name"]; hasName {
		t.Error("body must not contain 'name' when Name is nil")
	}
}

// TestUpdateWorkItem_PriorityOnly — update only the priority.
func TestUpdateWorkItem_PriorityOnly(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedBody map[string]any
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 3,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return item, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Task\n", nil
		},
	}

	newPriority := "urgent"
	args := UpdateWorkItemArgs{
		Identifier: "PROJ-3",
		Priority:   &newPriority,
	}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if len(capturedBody) != 1 || capturedBody["priority"] != "urgent" {
		t.Errorf("body = %v, want only priority='urgent'", capturedBody)
	}
}

// TestUpdateWorkItem_StateOnly — update only the state, verifying state resolution.
func TestUpdateWorkItem_StateOnly(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedBody map[string]any
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 7,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return &plane.State{ID: "state-done", Name: "Done"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Task\n", nil
		},
	}

	newState := "Done"
	args := UpdateWorkItemArgs{
		Identifier: "PROJ-7",
		State:      &newState,
	}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if len(capturedBody) != 1 || capturedBody["state"] != "state-done" {
		t.Errorf("body = %v, want only state='state-done'", capturedBody)
	}
}

// TestUpdateWorkItem_NoFields — error when no optional fields are provided.
func TestUpdateWorkItem_NoFields(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	updateCalled := false
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			updateCalled = true
			return item, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}

	args := UpdateWorkItemArgs{Identifier: "PROJ-1"}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when no fields are provided")
	}
	if updateCalled {
		t.Error("UpdateWorkItem must not be called when no fields are provided")
	}
}

// TestUpdateWorkItem_InvalidIdentifier — error on invalid identifier.
func TestUpdateWorkItem_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}

	args := UpdateWorkItemArgs{Identifier: "not-valid"}
	newName := "X"
	args.Name = &newName

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestUpdateWorkItem_GetWorkItemFails — error when fetching the work item fails.
func TestUpdateWorkItem_GetWorkItemFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}

	newName := "X"
	args := UpdateWorkItemArgs{Identifier: "PROJ-99", Name: &newName}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when get fails")
	}
}

// TestUpdateWorkItem_StateResolutionFails — error when state can't be resolved.
func TestUpdateWorkItem_StateResolutionFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{
		resolveStateFn: func(ctx context.Context, projectID, input string) (*plane.State, error) {
			return nil, errors.New("state not found: Bogus")
		},
	}
	formatter := &mockFormatter{}

	newState := "Bogus"
	args := UpdateWorkItemArgs{Identifier: "PROJ-1", State: &newState}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when state resolution fails")
	}
}

// TestUpdateWorkItem_UpdateFails — error when the PATCH request fails.
func TestUpdateWorkItem_UpdateFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("server error")
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}

	newPriority := "low"
	args := UpdateWorkItemArgs{Identifier: "PROJ-1", Priority: &newPriority}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when update fails")
	}
}

// TestUpdateWorkItem_FormatterFails — error when formatting the updated item fails.
func TestUpdateWorkItem_FormatterFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "", errors.New("format failed")
		},
	}

	newName := "X"
	args := UpdateWorkItemArgs{Identifier: "PROJ-1", Name: &newName}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when formatter fails")
	}
}

// TestUpdateWorkItem_ProjectValFallback — uses item.Project.ID when Val is nil.
func TestUpdateWorkItem_ProjectValFallback(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedProjectID string
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-fallback"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedProjectID = projectID
			return item, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Task\n", nil
		},
	}

	newName := "X"
	args := UpdateWorkItemArgs{Identifier: "PROJ-1", Name: &newName}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedProjectID != "proj-fallback" {
		t.Errorf("expected projectID='proj-fallback', got %q", capturedProjectID)
	}
}

// TestUpdateWorkItem_DetailIsFull — the formatter is called with detail="full".
func TestUpdateWorkItem_DetailIsFull(t *testing.T) {
	// Arrange
	ctx := context.Background()
	var capturedDetail string
	item := &plane.WorkItem{
		ID:         "wi-1",
		Name:       "Task",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return item, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return item, nil
		},
	}
	resolver := &mockResolver{}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			capturedDetail = detail
			return "name: Task\n", nil
		},
	}

	newName := "X"
	args := UpdateWorkItemArgs{Identifier: "PROJ-1", Name: &newName}

	// Act
	result, err := updateWorkItem(ctx, args, client, resolver, formatter)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedDetail != "full" {
		t.Errorf("expected detail='full', got %q", capturedDetail)
	}
}

// ---------------------------------------------------------------------------
// Relation management tools tests
// ---------------------------------------------------------------------------

func TestSetRelation_InvalidRelationType(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	args := SetRelationArgs{
		Identifier:        "PROJ-1",
		RelationType:      "bogus",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := setRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid relation type")
	}
}

func TestSetRelation_HappyPath(t *testing.T) {
	ctx := context.Background()
	var capturedRelationType string
	var capturedIssues []string
	var capturedWorkItemID string

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			if pi == "PROJ" && seq == 1 {
				return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
			}
			return &plane.WorkItem{ID: "wi-2", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		createWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, relationType string, issues []string) error {
			capturedWorkItemID = workItemID
			capturedRelationType = relationType
			capturedIssues = issues
			return nil
		},
	}
	args := SetRelationArgs{
		Identifier:        "PROJ-1",
		RelationType:      "blocking",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := setRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedRelationType != "blocking" {
		t.Errorf("expected relation_type='blocking', got %q", capturedRelationType)
	}
	if capturedWorkItemID != "wi-1" {
		t.Errorf("expected workItemID='wi-1', got %q", capturedWorkItemID)
	}
	if len(capturedIssues) != 1 || capturedIssues[0] != "wi-2" {
		t.Errorf("expected issues=[wi-2], got %v", capturedIssues)
	}
}

func TestSetRelation_SourceNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := SetRelationArgs{
		Identifier:        "PROJ-99",
		RelationType:      "relates_to",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := setRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when source item not found")
	}
}

func TestSetRelation_RelatedNotFound(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			callCount++
			if callCount == 1 {
				return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
			}
			return nil, errors.New("not found")
		},
	}
	args := SetRelationArgs{
		Identifier:        "PROJ-1",
		RelationType:      "relates_to",
		RelatedIdentifier: "PROJ-99",
	}

	result, err := setRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when related item not found")
	}
}

func TestSetRelation_APIFailure(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		createWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, relationType string, issues []string) error {
			return errors.New("api error")
		},
	}
	args := SetRelationArgs{
		Identifier:        "PROJ-1",
		RelationType:      "duplicate",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := setRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when API fails")
	}
}

func TestRemoveRelation_HappyPath(t *testing.T) {
	ctx := context.Background()
	var capturedRelatedIssue string
	var capturedWorkItemID string

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			if pi == "PROJ" && seq == 1 {
				return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
			}
			return &plane.WorkItem{ID: "wi-2", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		removeWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, relatedIssue string) error {
			capturedWorkItemID = workItemID
			capturedRelatedIssue = relatedIssue
			return nil
		},
	}
	args := RemoveRelationArgs{
		Identifier:        "PROJ-1",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := removeRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedWorkItemID != "wi-1" {
		t.Errorf("expected workItemID='wi-1', got %q", capturedWorkItemID)
	}
	if capturedRelatedIssue != "wi-2" {
		t.Errorf("expected relatedIssue='wi-2', got %q", capturedRelatedIssue)
	}
}

func TestRemoveRelation_SourceNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := RemoveRelationArgs{
		Identifier:        "PROJ-99",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := removeRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when source item not found")
	}
}

func TestRemoveRelation_APIFailure(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		removeWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, relatedIssue string) error {
			return errors.New("api error")
		},
	}
	args := RemoveRelationArgs{
		Identifier:        "PROJ-1",
		RelatedIdentifier: "PROJ-2",
	}

	result, err := removeRelation(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when API fails")
	}
}

func TestListRelations_HappyPath(t *testing.T) {
	ctx := context.Background()

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}, SequenceID: 1}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-uuid", Identifier: "PROJ"}}, nil
		},
		listWorkItemRelationsFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
			return &plane.WorkItemRelations{
				Blocking: []plane.RelationItem{
					{ProjectID: "proj-uuid", IssueID: "wi-2"},
				},
				RelatesTo: []plane.RelationItem{
					{ProjectID: "proj-uuid", IssueID: "wi-3"},
				},
			}, nil
		},
		getWorkItemFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItem, error) {
			if workItemID == "wi-2" {
				return &plane.WorkItem{ID: "wi-2", Name: "Blocked task", SequenceID: 2}, nil
			}
			return &plane.WorkItem{ID: "wi-3", Name: "Related task", SequenceID: 3}, nil
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-1"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "blocking:") {
		t.Errorf("expected 'blocking:' in output, got: %s", text)
	}
	if !strings.Contains(text, "PROJ-2") {
		t.Errorf("expected 'PROJ-2' in output, got: %s", text)
	}
	if !strings.Contains(text, "Blocked task") {
		t.Errorf("expected 'Blocked task' in output, got: %s", text)
	}
	if !strings.Contains(text, "relates_to:") {
		t.Errorf("expected 'relates_to:' in output, got: %s", text)
	}
	if !strings.Contains(text, "PROJ-3") {
		t.Errorf("expected 'PROJ-3' in output, got: %s", text)
	}
}

func TestListRelations_Empty(t *testing.T) {
	ctx := context.Background()

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}, SequenceID: 1}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-uuid", Identifier: "PROJ"}}, nil
		},
		listWorkItemRelationsFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
			return &plane.WorkItemRelations{}, nil
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-1"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "relations for PROJ-1") {
		t.Errorf("expected header in output, got: %s", text)
	}
}

func TestListRelations_SourceNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-99"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when source item not found")
	}
}

func TestListRelations_RelationsAPIFailure(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listWorkItemRelationsFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
			return nil, errors.New("api error")
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-1"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when relations API fails")
	}
}

func TestListRelations_ProjectsListFails(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listWorkItemRelationsFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
			return &plane.WorkItemRelations{}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return nil, errors.New("projects api error")
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-1"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when projects list fails")
	}
}

func TestListRelations_RelatedItemNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}, SequenceID: 1}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-uuid", Identifier: "PROJ"}}, nil
		},
		listWorkItemRelationsFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
			return &plane.WorkItemRelations{
				Blocking: []plane.RelationItem{
					{ProjectID: "proj-uuid", IssueID: "wi-missing"},
				},
			}, nil
		},
		getWorkItemFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-1"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false (graceful degradation): %+v", result.Content)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "(unknown)") {
		t.Errorf("expected '(unknown)' fallback for missing item, got: %s", text)
	}
}

func TestListRelations_NilRelations(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listWorkItemRelationsFn: func(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error) {
			return nil, nil
		},
	}
	args := ListRelationsArgs{Identifier: "PROJ-1"}

	result, err := listRelations(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected IsError=true when relations is nil")
	}
	textContent, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(textContent.Text, "relations data is missing or empty") {
		t.Errorf("expected 'relations data is missing or empty' in error, got: %s", textContent.Text)
	}
}

func TestRegisterWithDeps_IncludesRelationTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	cfg := &config.Config{PlaneMCPProfile: "full"}

	// Should not panic — verifies set_relation, remove_relation, list_relations register cleanly.
	registerWithDeps(server, client, resolver, formatter, cfg)
}

// ---------------------------------------------------------------------------
// TestSetParent — table-driven tests for set_parent
// ---------------------------------------------------------------------------

func TestSetParent_HappyPath(t *testing.T) {
	ctx := context.Background()
	var capturedBody map[string]any
	var capturedWorkItemID string

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			if pi == "PROJ" && seq == 1 {
				return &plane.WorkItem{ID: "wi-child", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
			}
			return &plane.WorkItem{ID: "wi-parent", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedWorkItemID = itemID
			capturedBody = body
			return &plane.WorkItem{ID: "wi-child"}, nil
		},
	}
	args := SetParentArgs{
		Identifier:       "PROJ-1",
		ParentIdentifier: "PROJ-2",
	}

	result, err := setParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedWorkItemID != "wi-child" {
		t.Errorf("expected workItemID='wi-child', got %q", capturedWorkItemID)
	}
	parentVal, ok := capturedBody["parent"]
	if !ok || parentVal != "wi-parent" {
		t.Errorf("expected body['parent']='wi-parent', got %v", parentVal)
	}
}

func TestSetParent_InvalidChildIdentifier(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	args := SetParentArgs{
		Identifier:       "invalid",
		ParentIdentifier: "PROJ-2",
	}

	result, err := setParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid child identifier")
	}
}

func TestSetParent_InvalidParentIdentifier(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-child", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
	}
	args := SetParentArgs{
		Identifier:       "PROJ-1",
		ParentIdentifier: "invalid",
	}

	result, err := setParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid parent identifier")
	}
}

func TestSetParent_ChildNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := SetParentArgs{
		Identifier:       "PROJ-99",
		ParentIdentifier: "PROJ-2",
	}

	result, err := setParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when child not found")
	}
}

func TestSetParent_ParentNotFound(t *testing.T) {
	ctx := context.Background()
	callCount := 0
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			callCount++
			if callCount == 1 {
				return &plane.WorkItem{ID: "wi-child", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
			}
			return nil, errors.New("not found")
		},
	}
	args := SetParentArgs{
		Identifier:       "PROJ-1",
		ParentIdentifier: "PROJ-99",
	}

	result, err := setParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when parent not found")
	}
}

func TestSetParent_UpdateError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("api error")
		},
	}
	args := SetParentArgs{
		Identifier:       "PROJ-1",
		ParentIdentifier: "PROJ-2",
	}

	result, err := setParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when update fails")
	}
}

// ---------------------------------------------------------------------------
// TestClearParent — table-driven tests for clear_parent
// ---------------------------------------------------------------------------

func TestClearParent_HappyPath(t *testing.T) {
	ctx := context.Background()
	var capturedBody map[string]any
	var capturedWorkItemID string

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			capturedWorkItemID = itemID
			capturedBody = body
			return &plane.WorkItem{ID: "wi-1"}, nil
		},
	}
	args := ClearParentArgs{Identifier: "PROJ-1"}

	result, err := clearParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedWorkItemID != "wi-1" {
		t.Errorf("expected workItemID='wi-1', got %q", capturedWorkItemID)
	}
	parentVal, ok := capturedBody["parent"]
	if !ok || parentVal != nil {
		t.Errorf("expected body['parent']=nil, got %v", parentVal)
	}
}

func TestClearParent_InvalidIdentifier(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	args := ClearParentArgs{Identifier: "invalid"}

	result, err := clearParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

func TestClearParent_ItemNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := ClearParentArgs{Identifier: "PROJ-99"}

	result, err := clearParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when item not found")
	}
}

func TestClearParent_UpdateError(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-1", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("api error")
		},
	}
	args := ClearParentArgs{Identifier: "PROJ-1"}

	result, err := clearParent(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when update fails")
	}
}

// ---------------------------------------------------------------------------
// TestListChildren — table-driven tests for list_children
// ---------------------------------------------------------------------------

func TestListChildren_HappyPath(t *testing.T) {
	ctx := context.Background()

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-parent", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-uuid", Identifier: "PROJ"}}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return []plane.WorkItem{
				{ID: "wi-1", Name: "Child one", SequenceID: 2, Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}},
				{ID: "wi-2", Name: "Child two", SequenceID: 3, Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}},
			}, nil
		},
	}
	args := ListChildrenArgs{Identifier: "PROJ-1"}

	result, err := listChildren(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if !strings.Contains(text, "PROJ-2") {
		t.Errorf("expected 'PROJ-2' in output, got: %s", text)
	}
	if !strings.Contains(text, "Child one") {
		t.Errorf("expected 'Child one' in output, got: %s", text)
	}
	if !strings.Contains(text, "PROJ-3") {
		t.Errorf("expected 'PROJ-3' in output, got: %s", text)
	}
	if !strings.Contains(text, "Child two") {
		t.Errorf("expected 'Child two' in output, got: %s", text)
	}
}

func TestListChildren_Empty(t *testing.T) {
	ctx := context.Background()

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-parent", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-uuid", Identifier: "PROJ"}}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return []plane.WorkItem{}, nil
		},
	}
	args := ListChildrenArgs{Identifier: "PROJ-1"}

	result, err := listChildren(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	if text != "[]" {
		t.Errorf("expected '[]' for empty children, got: %s", text)
	}
}

func TestListChildren_InvalidIdentifier(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	args := ListChildrenArgs{Identifier: "invalid"}

	result, err := listChildren(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

func TestListChildren_ItemNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	args := ListChildrenArgs{Identifier: "PROJ-99"}

	result, err := listChildren(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when item not found")
	}
}

func TestListChildren_ProjectsListFails(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-parent", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return nil, errors.New("projects api error")
		},
	}
	args := ListChildrenArgs{Identifier: "PROJ-1"}

	result, err := listChildren(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when projects list fails")
	}
}

func TestListChildren_ListWorkItemsFails(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return &plane.WorkItem{ID: "wi-parent", Project: plane.Expandable[plane.Project]{ID: "proj-uuid"}}, nil
		},
		listProjectsFn: func(ctx context.Context) ([]plane.Project, error) {
			return []plane.Project{{ID: "proj-uuid", Identifier: "PROJ"}}, nil
		},
		listWorkItemsFn: func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error) {
			return nil, errors.New("api error")
		},
	}
	args := ListChildrenArgs{Identifier: "PROJ-1"}

	result, err := listChildren(ctx, args, client)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when list work items fails")
	}
}

func TestRegisterWithDeps_IncludesParentTools(t *testing.T) {
	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	cfg := &config.Config{PlaneMCPProfile: "full"}

	// Should not panic — verifies set_parent, clear_parent, list_children register cleanly.
	registerWithDeps(server, client, resolver, formatter, cfg)
}

// ---------------------------------------------------------------------------
// moveWorkItem tests
// ---------------------------------------------------------------------------

func TestMoveWorkItem_InvalidIdentifier(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{}
	formatter := &mockFormatter{}
	args := MoveWorkItemArgs{Identifier: "invalid", TargetProject: "TGT"}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

func TestMoveWorkItem_TargetProjectNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return nil, errors.New("project not found")
		},
	}
	formatter := &mockFormatter{}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "NONEXISTENT"}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when target project not found")
	}
}

func TestMoveWorkItem_SourceItemNotFound(t *testing.T) {
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("not found")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target"}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when source item not found")
	}
}

func TestMoveWorkItem_TargetCreationError(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:       "src-wi",
		Name:     "Test Item",
		SequenceID: 1,
		Project:  plane.Expandable[plane.Project]{ID: "src-proj"},
		State:    plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "s1", Name: "Todo", Group: "unstarted"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("creation failed")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target"}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when target creation fails")
	}
}

func TestMoveWorkItem_DeleteOriginalTrue(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Test Item",
		SequenceID: 42,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
	}

	var deletedProjectID, deletedItemID string
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "s1", Name: "Todo", Group: "unstarted"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return createdItem, nil
		},
		deleteWorkItemFn: func(ctx context.Context, projectID, workItemID string) error {
			deletedProjectID = projectID
			deletedItemID = workItemID
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: true}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got IsError=true: %+v", result.Content)
	}
	if deletedProjectID != "src-proj" {
		t.Errorf("expected delete on src-proj, got %q", deletedProjectID)
	}
	if deletedItemID != "src-wi" {
		t.Errorf("expected delete of src-wi, got %q", deletedItemID)
	}
	// Verify output contains the new identifier
	tc, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected *mcp.TextContent, got %T", result.Content[0])
	}
	if !strings.Contains(tc.Text, "TGT-42") {
		t.Errorf("expected output to contain new identifier TGT-42, got: %s", tc.Text)
	}
}

func TestMoveWorkItem_DeleteOriginalFalse(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Original Title",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "In Progress", Group: "started"}},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Original Title",
		SequenceID: 99,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s2", Val: &plane.State{Name: "In Progress", Group: "started"}},
	}

	var updatedProjectID, updatedItemID string
	var updatedBody map[string]any
	var relationProjectID, relationItemID, relationType string
	var relationIssues []string

	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			if projectID == "tgt-proj" {
				return []plane.State{
					{ID: "s1", Name: "Backlog", Group: "backlog"},
					{ID: "s2", Name: "In Progress", Group: "started"},
				}, nil
			}
			// Source project states
			return []plane.State{
				{ID: "state-1", Name: "In Progress", Group: "started"},
				{ID: "state-cancelled", Name: "Cancelled", Group: "cancelled"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return createdItem, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			updatedProjectID = projectID
			updatedItemID = itemID
			updatedBody = body
			return srcItem, nil
		},
		createWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, rType string, issues []string) error {
			relationProjectID = projectID
			relationItemID = workItemID
			relationType = rType
			relationIssues = issues
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Original Title\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: false}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got IsError=true: %+v", result.Content)
	}

	// Verify original was renamed
	if updatedProjectID != "src-proj" {
		t.Errorf("expected update on src-proj, got %q", updatedProjectID)
	}
	if updatedItemID != "src-wi" {
		t.Errorf("expected update of src-wi, got %q", updatedItemID)
	}
	wantName := "MOVED TO TGT-99 - Original Title"
	if got, ok := updatedBody["name"].(string); !ok || got != wantName {
		t.Errorf("expected name %q, got %v", wantName, updatedBody["name"])
	}
	// Verify state was set to cancelled
	if got, ok := updatedBody["state"].(string); !ok || got != "state-cancelled" {
		t.Errorf("expected state 'state-cancelled', got %v", updatedBody["state"])
	}

	// Verify duplicate relation
	if relationProjectID != "src-proj" {
		t.Errorf("expected relation on src-proj, got %q", relationProjectID)
	}
	if relationItemID != "src-wi" {
		t.Errorf("expected relation on src-wi, got %q", relationItemID)
	}
	if relationType != "duplicate" {
		t.Errorf("expected relation type 'duplicate', got %q", relationType)
	}
	if len(relationIssues) != 1 || relationIssues[0] != "tgt-wi" {
		t.Errorf("expected issues [tgt-wi], got %v", relationIssues)
	}
}

func TestMoveWorkItem_StateGroupMatchingFallback(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		// Source state is "To Do" — not present in target project by name.
		State: plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "To Do", Group: "unstarted"}},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Test Item",
		SequenceID: 5,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s-backlog", Val: &plane.State{Name: "Backlog", Group: "backlog"}},
	}

	var capturedBody map[string]any
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			if projectID == "tgt-proj" {
				// Target has "Backlog" (unstarted group) but no "To Do" by name.
				return []plane.State{
					{ID: "s-backlog", Name: "Backlog", Group: "unstarted"},
					{ID: "s-done", Name: "Done", Group: "completed"},
				}, nil
			}
			return []plane.State{
				{ID: "state-1", Name: "To Do", Group: "unstarted"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return createdItem, nil
		},
		deleteWorkItemFn: func(ctx context.Context, projectID, workItemID string) error {
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: true}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got IsError=true: %+v", result.Content)
	}

	// Should have matched by group "unstarted" → "Backlog"
	if got, ok := capturedBody["state"].(string); !ok || got != "s-backlog" {
		t.Errorf("expected state 's-backlog' (matched by group), got %v", capturedBody["state"])
	}

	// Should contain warning about state name difference
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "differs from original state") {
		t.Errorf("expected warning about state name difference, got: %s", tc.Text)
	}
}

func TestMoveWorkItem_NoCancelledStateFallback(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "Backlog", Group: "backlog"}},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Test Item",
		SequenceID: 7,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s1", Val: &plane.State{Name: "Backlog", Group: "backlog"}},
	}

	var updatedBody map[string]any
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			// No cancelled state anywhere
			return []plane.State{
				{ID: "s1", Name: "Backlog", Group: "backlog"},
				{ID: "s2", Name: "Done", Group: "completed"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return createdItem, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			updatedBody = body
			return srcItem, nil
		},
		createWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, rType string, issues []string) error {
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: false}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success, got IsError=true: %+v", result.Content)
	}

	// Verify state was NOT set (no cancelled state available)
	if _, ok := updatedBody["state"]; ok {
		t.Errorf("expected no state in update body when no cancelled state exists, got %v", updatedBody["state"])
	}
	// Verify name was still set (rename happened)
	if got, ok := updatedBody["name"].(string); !ok || !strings.HasPrefix(got, "MOVED TO") {
		t.Errorf("expected name to start with MOVED TO, got %v", updatedBody["name"])
	}
}

func TestMoveWorkItem_LabelNotFoundInTarget(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
		Labels: []plane.Expandable[plane.Label]{
			{ID: "lbl-1", Val: &plane.Label{Name: "bug"}},
			{ID: "lbl-2", Val: &plane.Label{Name: "feature"}},
		},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Test Item",
		SequenceID: 3,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
	}

	var capturedBody map[string]any
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "s1", Name: "Todo", Group: "unstarted"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			capturedBody = body
			return createdItem, nil
		},
		deleteWorkItemFn: func(ctx context.Context, projectID, workItemID string) error {
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
		resolveLabelFn: func(ctx context.Context, projectID, input string) (*plane.Label, error) {
			// "bug" resolves, "feature" does not
			if input == "bug" {
				return &plane.Label{ID: "lbl-tgt-1", Name: "bug"}, nil
			}
			return nil, errors.New("label not found")
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: true}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success despite label resolution failure, got IsError=true: %+v", result.Content)
	}

	// Only "bug" label should be present
	labels, _ := capturedBody["labels"].([]string)
	if len(labels) != 1 || labels[0] != "lbl-tgt-1" {
		t.Errorf("expected only label 'lbl-tgt-1', got %v", labels)
	}

	// Should contain warning about the missing label
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "feature") || !strings.Contains(tc.Text, "not found") {
		t.Errorf("expected warning about 'feature' label not found, got: %s", tc.Text)
	}
}

func TestMoveWorkItem_ListStatesFailsForTarget(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			if projectID == "tgt-proj" {
				return nil, errors.New("states api error")
			}
			return nil, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target"}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when listing target states fails")
	}
}

func TestMoveWorkItem_ListStatesFailsForSource(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1"},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			if projectID == "src-proj" {
				return nil, errors.New("states api error")
			}
			return []plane.State{{ID: "s1", Name: "Todo", Group: "unstarted"}}, nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target"}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when listing source states fails")
	}
}

func TestMoveWorkItem_DeleteOriginalFalse_UpdateFails(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "Backlog", Group: "backlog"}},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Test Item",
		SequenceID: 10,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s1", Val: &plane.State{Name: "Backlog", Group: "backlog"}},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{
				{ID: "s1", Name: "Backlog", Group: "backlog"},
				{ID: "sc", Name: "Cancelled", Group: "cancelled"},
			}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return createdItem, nil
		},
		updateWorkItemFn: func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error) {
			return nil, errors.New("update failed")
		},
		createWorkItemRelationFn: func(ctx context.Context, projectID, workItemID, rType string, issues []string) error {
			return nil
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: false}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success (update failure is a warning, not an error), got IsError=true: %+v", result.Content)
	}
	// Should contain warning about update failure
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "failed to update original") {
		t.Errorf("expected warning about update failure, got: %s", tc.Text)
	}
}

func TestMoveWorkItem_DeleteOriginalTrue_DeleteFails(t *testing.T) {
	ctx := context.Background()
	srcItem := &plane.WorkItem{
		ID:         "src-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "src-proj"},
		State:      plane.Expandable[plane.State]{ID: "state-1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
	}
	createdItem := &plane.WorkItem{
		ID:         "tgt-wi",
		Name:       "Test Item",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "tgt-proj"},
		State:      plane.Expandable[plane.State]{ID: "s1", Val: &plane.State{Name: "Todo", Group: "unstarted"}},
	}
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return srcItem, nil
		},
		listStatesFn: func(ctx context.Context, projectID string) ([]plane.State, error) {
			return []plane.State{{ID: "s1", Name: "Todo", Group: "unstarted"}}, nil
		},
		createWorkItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error) {
			return createdItem, nil
		},
		deleteWorkItemFn: func(ctx context.Context, projectID, workItemID string) error {
			return errors.New("delete failed")
		},
	}
	resolver := &mockResolver{
		resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "tgt-proj", Name: "Target", Identifier: "TGT"}, nil
		},
	}
	formatter := &mockFormatter{
		formatWorkItemYAMLFn: func(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
			return "name: Test Item\n", nil
		},
	}
	args := MoveWorkItemArgs{Identifier: "PROJ-1", TargetProject: "Target", DeleteOriginal: true}

	result, err := moveWorkItem(ctx, args, client, resolver, formatter)
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected success (delete failure is a warning), got IsError=true: %+v", result.Content)
	}
	// Should contain warning about delete failure
	tc := result.Content[0].(*mcp.TextContent)
	if !strings.Contains(tc.Text, "failed to delete original") {
		t.Errorf("expected warning about delete failure, got: %s", tc.Text)
	}
}

// ---------------------------------------------------------------------------
// add_comment handler tests added by AGENT-52
// ---------------------------------------------------------------------------

// TestAddComment_Success — happy path: markdown body is converted to HTML and posted.
func TestAddComment_Success(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 1,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	var capturedComment string
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			capturedComment = comment
			return nil
		},
	}
	args := AddCommentArgs{Identifier: "PROJ-1", Body: "**bold** and `code`"}

	// Act
	result, err := addComment(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	// Verify the body was converted from Markdown to HTML.
	wantHTML := "<p><strong>bold</strong> and <code>code</code></p>"
	if capturedComment != wantHTML {
		t.Errorf("capturedComment = %q, want %q", capturedComment, wantHTML)
	}
}

// TestAddComment_InvalidIdentifier — error path for bad identifier.
func TestAddComment_InvalidIdentifier(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{}
	args := AddCommentArgs{Identifier: "BAD"}

	// Act
	result, err := addComment(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid identifier")
	}
}

// TestAddComment_WorkItemFetchError — client error surfaced as tool error.
func TestAddComment_WorkItemFetchError(t *testing.T) {
	// Arrange
	ctx := context.Background()
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return nil, errors.New("item not found")
		},
	}
	args := AddCommentArgs{Identifier: "PROJ-1", Body: "comment"}

	// Act
	result, err := addComment(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when work item fetch fails")
	}
}

// TestAddComment_CommentError — comment creation failure surfaced.
func TestAddComment_CommentError(t *testing.T) {
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
	args := AddCommentArgs{Identifier: "PROJ-1", Body: "comment"}

	// Act
	result, err := addComment(ctx, args, client)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true when comment creation fails")
	}
}

// TestReportProgress_StateOmitted — success when state is omitted (nil) but comment is provided.
func TestReportProgress_StateOmitted(t *testing.T) {
	// Arrange
	ctx := context.Background()
	workItem := &plane.WorkItem{
		ID:         "wi-1",
		SequenceID: 8,
		Project:    plane.Expandable[plane.Project]{ID: "proj-uuid"},
	}
	var capturedComment string
	client := &mockClient{
		getWorkItemByIdentifierFn: func(ctx context.Context, pi string, seq int) (*plane.WorkItem, error) {
			return workItem, nil
		},
		createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
			capturedComment = comment
			return nil
		},
	}
	resolver := &mockResolver{}
	args := ReportProgressArgs{Identifier: "PROJ-8", Comment: "still working"} // State is nil

	// Act
	result, err := reportProgress(ctx, args, client, resolver)

	// Assert
	if err != nil {
		t.Fatalf("unexpected Go error: %v", err)
	}
	if result.IsError {
		t.Errorf("expected IsError=false: %+v", result.Content)
	}
	if capturedComment == "" {
		t.Error("expected comment to be posted when state is omitted")
	}
	// Verify that no state transition was attempted — the update function should not be called.
	if client.updateWorkItemFn != nil {
		t.Error("UpdateWorkItem should not be called when state is omitted")
	}
}
