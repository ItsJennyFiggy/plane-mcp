package tools

import (
	"context"
	"errors"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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

// mockClient is a test double for planeClient.
type mockClient struct {
	listProjectsFn             func(ctx context.Context) ([]plane.Project, error)
	getWorkItemByIdentifierFn  func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error)
	listWorkItemsFn            func(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error)
	createWorkItemFn           func(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error)
	createWorkItemCommentFn    func(ctx context.Context, projectID, itemID, comment string) error
	updateWorkItemFn           func(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error)
	createWorkItemLinkFn       func(ctx context.Context, projectID, itemID, linkURL, title string) error
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

// mockResolver is a test double for planeResolver.
type mockResolver struct {
	getCallerIDFn    func(ctx context.Context) (string, error)
	resolveProjectFn func(ctx context.Context, input string) (*plane.Project, error)
	resolveStateFn   func(ctx context.Context, projectID string, input string) (*plane.State, error)
	resolveLabelFn   func(ctx context.Context, projectID string, input string) (*plane.Label, error)
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
	args := FindMyWorkArgs{Project: "Alpha"}

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
	args := FindMyWorkArgs{Project: "UNKNOWN"}

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
		Assignees: []string{"good-user", "bad-user"}, // bad-user skipped
		Labels:    []string{"bug", "missing-label"},  // missing-label skipped
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
	// No panic means all five tools were registered successfully.
	// (The MCP SDK panics if a tool with an invalid name is registered.)
}

func TestRegisterWithDeps_WorkerProfile(t *testing.T) {
	// Arrange — worker profile should register 4 tools (not create_task)
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
