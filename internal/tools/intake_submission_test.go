package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Intake submission & enrichment tests (AGENT-179)
// ---------------------------------------------------------------------------

// newCreatedIntakeRecord returns the intake record a successful POST is
// expected to produce: pending status, IN_APP source, expanded issue.
func newCreatedIntakeRecord() *plane.IntakeWorkItem {
	return &plane.IntakeWorkItem{
		ID:     "intake-11",
		Status: plane.IntakeStatusPending,
		Source: "IN_APP",
		Issue: plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{
			ID:         "issue-uuid-11",
			Name:       "Quick idea",
			SequenceID: 11,
			Priority:   "high",
		}},
	}
}

func resultText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return text.Text
}

func TestCreateIntakeWorkItem(t *testing.T) {
	resolver := newIntakeTriageResolver()

	t.Run("success submits nested issue body and reports identifiers", func(t *testing.T) {
		var gotProjectID string
		var gotBody map[string]any
		var formatted []plane.IntakeWorkItem
		client := &mockClient{
			createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				gotProjectID = projectID
				gotBody = body
				return newCreatedIntakeRecord(), nil
			},
		}
		formatter := newIntakeTriageFormatter(&formatted)

		result, err := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project:     "ASBX",
			Name:        "Quick idea",
			Description: "A simple idea",
			Priority:    "high",
		}, client, resolver, formatter)
		if err != nil || result.IsError {
			t.Fatalf("create failed: %v %+v", err, result)
		}

		if gotProjectID != "project-1" {
			t.Errorf("expected resolved project UUID, got %q", gotProjectID)
		}
		if gotBody["name"] != "Quick idea" {
			t.Errorf("expected name in issue body, got %+v", gotBody)
		}
		if gotBody["priority"] != "high" {
			t.Errorf("expected priority in issue body, got %+v", gotBody)
		}
		html, _ := gotBody["description_html"].(string)
		if html == "" || !strings.HasPrefix(html, "<") {
			t.Errorf("expected HTML description, got %q", html)
		}
		if len(formatted) != 1 {
			t.Fatalf("expected one formatted item, got %d", len(formatted))
		}
		item := formatted[0]
		if item.ID != "intake-11" || item.Status != plane.IntakeStatusPending || item.Source != "IN_APP" {
			t.Errorf("unexpected formatted record: %+v", item)
		}
		if item.UnderlyingIssueID() != "issue-uuid-11" {
			t.Errorf("expected underlying issue UUID, got %q", item.UnderlyingIssueID())
		}
	})

	t.Run("description markdown is converted to Plane rich text", func(t *testing.T) {
		var gotBody map[string]any
		client := &mockClient{
			createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				gotBody = body
				return newCreatedIntakeRecord(), nil
			},
		}

		result, err := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project:     "ASBX",
			Name:        "Idea",
			Description: "Plain words here",
		}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("create failed: %v %+v", err, result)
		}
		if got, want := resultString(gotBody["description_html"]), "<p>Plain words here</p>"; got != want {
			t.Errorf("description_html = %q, want %q", got, want)
		}
	})

	t.Run("reconciles identifier from queue when POST response is unexpanded", func(t *testing.T) {
		unexpanded := &plane.IntakeWorkItem{
			ID:     "intake-11",
			Status: plane.IntakeStatusPending,
			Issue:  plane.Expandable[plane.IntakeIssue]{ID: "issue-uuid-11"},
		}
		listCalls := 0
		client := &mockClient{
			createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return unexpanded, nil
			},
			listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
				listCalls++
				return []plane.IntakeWorkItem{*newCreatedIntakeRecord()}, nil
			},
		}

		result, err := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project: "ASBX", Name: "Quick idea",
		}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("create failed: %v %+v", err, result)
		}
		if listCalls != 1 {
			t.Fatalf("expected one reconcile list read, got %d", listCalls)
		}
	})

	t.Run("unresolvable identifier fails loudly instead of guessing", func(t *testing.T) {
		unexpanded := &plane.IntakeWorkItem{
			ID:     "intake-11",
			Status: plane.IntakeStatusPending,
			Issue:  plane.Expandable[plane.IntakeIssue]{ID: "issue-uuid-11"},
		}
		client := &mockClient{
			createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return unexpanded, nil
			},
			listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
				return nil, errors.New("boom")
			},
		}

		result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project: "ASBX", Name: "Quick idea",
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "could not be confirmed in the queue") {
			t.Fatalf("expected reconcile failure error, got: %+v", result)
		}
	})

	t.Run("empty name is rejected before any API call", func(t *testing.T) {
		calls := 0
		client := &mockClient{createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
			calls++
			return nil, nil
		}}

		result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project: "ASBX", Name: "   ",
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "name is required") {
			t.Fatalf("expected name-required error, got: %+v", result)
		}
		if calls != 0 {
			t.Fatalf("expected no API call, got %d", calls)
		}
	})

	t.Run("invalid priority mirrors server-side validation", func(t *testing.T) {
		calls := 0
		client := &mockClient{createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
			calls++
			return nil, nil
		}}

		for _, bad := range []string{"critical", "HIGH", "-2"} {
			result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
				Project: "ASBX", Name: "Idea", Priority: "critical",
			}, client, resolver, newIntakeTriageFormatter(nil))
			if !result.IsError || !strings.Contains(resultText(t, result), "invalid priority") {
				t.Fatalf("priority %q: expected validation error, got: %+v", bad, result)
			}
		}
		if calls != 0 {
			t.Fatalf("expected no API call for invalid priorities, got %d", calls)
		}
	})

	t.Run("API failure surfaces as tool error", func(t *testing.T) {
		client := &mockClient{createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
			return nil, errors.New("400 Bad Request")
		}}

		result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project: "ASBX", Name: "Idea", Priority: "high",
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "failed to create intake work item") {
			t.Fatalf("expected creation failure, got: %+v", result)
		}
	})
}

// resultString renders an arbitrary body field for assertions.
func resultString(value any) string {
	s, _ := value.(string)
	return s
}

func TestUpdateIntakeWorkItem(t *testing.T) {
	resolver := newIntakeTriageResolver()

	newPendingRecord := func(name string, priority string) *plane.IntakeWorkItem {
		record := newIntakeTriageRecord(plane.IntakeStatusPending)
		record.Issue.Val.Name = name
		record.Issue.Val.Priority = priority
		return record
	}

	t.Run("success verifies stored fields and annotates visibility", func(t *testing.T) {
		pending := newPendingRecord("Idea", "none")
		var patchedUUID string
		var patchedBody map[string]any
		var formatted []plane.IntakeWorkItem
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(pending),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchedUUID = issueID
				patchedBody = body
				updated := *pending
				updated.Issue = plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{
					ID: "issue-10", SequenceID: 10, Name: "Renamed idea", Priority: "high",
					DescriptionHTML: "<p>Rich text</p>",
				}}
				return &updated, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				updated := *pending
				updated.Issue = plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{
					ID: "issue-10", SequenceID: 10, Name: "Renamed idea", Priority: "high",
					DescriptionHTML: "<p>Rich text</p>",
				}}
				return &updated, nil
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}
		formatter := newIntakeTriageFormatter(&formatted)

		name := "Renamed idea"
		desc := "Rich text"
		priority := "high"
		result, err := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name, Description: &desc, Priority: &priority,
		}, client, resolver, formatter)
		if err != nil || result.IsError {
			t.Fatalf("update failed: %v %+v", err, result)
		}

		if patchedUUID != "issue-10" {
			t.Errorf("expected PATCH on underlying issue UUID, got %q", patchedUUID)
		}
		issueFields, ok := patchedBody["issue"].(map[string]any)
		if !ok {
			t.Fatalf("expected nested issue fields in PATCH body, got %+v", patchedBody)
		}
		if issueFields["name"] != "Renamed idea" || issueFields["priority"] != "high" {
			t.Errorf("unexpected issue fields: %+v", issueFields)
		}
		if _, hasStatus := patchedBody["status"]; hasStatus {
			t.Errorf("enrichment must not touch triage status, body was %+v", patchedBody)
		}
		if len(formatted) != 1 || formatted[0].UnderlyingIssue().Name != "Renamed idea" || formatted[0].ResolvedIdentifier != "ASBX-10" {
			t.Fatalf("unexpected verified output: %+v", formatted)
		}
	})

	t.Run("silently ignored fields are reported as enrichment_not_applied", func(t *testing.T) {
		pending := newPendingRecord("Idea", "none")
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(pending),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				stillOld := *pending
				return &stillOld, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				stillOld := *pending
				return &stillOld, nil
			},
		}

		name := "Renamed idea"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name,
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError {
			t.Fatal("expected MCP tool error for ignored enrichment")
		}
		text := resultText(t, result)
		for _, want := range []string{"enrichment_not_applied", "name", "silently drops"} {
			if !strings.Contains(text, want) {
				t.Errorf("error text missing %q: %s", want, text)
			}
		}
	})

	t.Run("status change during enrichment is reported as an error", func(t *testing.T) {
		pending := newPendingRecord("Idea", "none")
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(pending),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				moved := *pending
				moved.Status = plane.IntakeStatusAccepted
				return &moved, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				moved := *pending
				moved.Status = plane.IntakeStatusAccepted
				moved.Issue.Val.Name = "Renamed idea"
				return &moved, nil
			},
		}

		name := "Renamed idea"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name,
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError {
			t.Fatal("expected MCP tool error when enrichment changed triage status")
		}
		text := resultText(t, result)
		for _, want := range []string{"enrichment_not_applied", `"pending"`, `"accepted"`} {
			if !strings.Contains(text, want) {
				t.Errorf("error text missing %q: %s", want, text)
			}
		}
	})

	t.Run("idempotent no-op skips PATCH when values already match", func(t *testing.T) {
		pending := newPendingRecord("Idea", "high")
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(pending),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
		}

		name := "Idea"
		priority := "high"
		result, err := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name, Priority: &priority,
		}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("idempotent update failed: %v %+v", err, result)
		}
		if patchCalls != 0 {
			t.Fatalf("expected no PATCH for matching values, got %d", patchCalls)
		}
		if !strings.Contains(resultText(t, result), "No change needed") {
			t.Errorf("expected no-change notice, got: %s", resultText(t, result))
		}
	})

	t.Run("no fields provided is rejected", func(t *testing.T) {
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10",
		}, &mockClient{}, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "nothing to update") {
			t.Fatalf("expected nothing-to-update error, got: %+v", result)
		}
	})

	t.Run("empty replacement name is rejected", func(t *testing.T) {
		empty := "  "
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &empty,
		}, &mockClient{}, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "name cannot be empty") {
			t.Fatalf("expected empty-name error, got: %+v", result)
		}
	})

	t.Run("invalid replacement priority is rejected before any write", func(t *testing.T) {
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, nil
			},
		}

		bad := "critical"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Priority: &bad,
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "invalid priority") {
			t.Fatalf("expected invalid-priority error, got: %+v", result)
		}
		if patchCalls != 0 {
			t.Fatalf("expected no PATCH, got %d", patchCalls)
		}
	})

	t.Run("unknown identifier reports active-queue miss", func(t *testing.T) {
		client := &mockClient{listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
			return nil, nil
		}}

		name := "Whatever"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-99", Name: &name,
		}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "not found in the active intake queue") {
			t.Fatalf("unexpected result for unknown identifier: %+v", result)
		}
	})
}

func TestIntakeSubmissionProfileBoundaries(t *testing.T) {
	cfgFor := func(profile string) *config.Config {
		return &config.Config{PlaneMCPProfile: profile}
	}

	for _, profile := range []string{"worker", "planner", "full", "reviewer"} {
		if !shouldRegister("create_intake_work_item", workerPlannerFullReviewer, cfgFor(profile)) {
			t.Errorf("create_intake_work_item must register under %q profile", profile)
		}
	}

	for _, profile := range []string{"planner", "full"} {
		if !shouldRegister("update_intake_work_item", plannerFull, cfgFor(profile)) {
			t.Errorf("update_intake_work_item must register under %q profile", profile)
		}
	}
	for _, profile := range []string{"worker", "reviewer"} {
		if shouldRegister("update_intake_work_item", plannerFull, cfgFor(profile)) {
			t.Errorf("update_intake_work_item must NOT register under %q profile", profile)
		}
	}
}
