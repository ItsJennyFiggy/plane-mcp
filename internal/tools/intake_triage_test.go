package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ---------------------------------------------------------------------------
// Intake triage action tests (AGENT-182)
// ---------------------------------------------------------------------------

// newIntakeTriageRecord builds the shared pending ASBX-10 intake record used
// by triage handler tests.
func newIntakeTriageRecord(status int) *plane.IntakeWorkItem {
	return &plane.IntakeWorkItem{
		ID:     "intake-10",
		Status: status,
		Issue:  plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{ID: "issue-10", SequenceID: 10, Name: "Idea"}},
	}
}

// newIntakeTriageResolver resolves any input to the ASBX project fixture.
func newIntakeTriageResolver() *mockResolver {
	return &mockResolver{resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
		return &plane.Project{ID: "project-1", Identifier: "ASBX"}, nil
	}}
}

// newIntakeTriageFormatter returns a formatter echoing a static YAML doc while
// capturing the items it was asked to format.
func newIntakeTriageFormatter(captured *[]plane.IntakeWorkItem) *mockFormatter {
	return &mockFormatter{formatIntakeWorkItemsYAMLFn: func(ctx context.Context, items []plane.IntakeWorkItem) (string, error) {
		if captured != nil {
			*captured = items
		}
		return "- identifier: ASBX-10\n", nil
	}}
}

// baseIntakeListFn exposes a single record through the intake list route.
func baseIntakeListFn(record *plane.IntakeWorkItem) func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
	return func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
		return []plane.IntakeWorkItem{*record}, nil
	}
}

// visibleTargetFn makes the underlying issue visible in normal work items.
func visibleTargetFn() func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
	return func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
		return &plane.WorkItem{ID: "issue-10", SequenceID: sequenceID}, nil
	}
}

// defaultStateFn returns the project's states; used to assert missing-default
// cause classification on acceptance.
func statesFn(states []plane.State) func(ctx context.Context, projectID string) ([]plane.State, error) {
	return func(ctx context.Context, projectID string) ([]plane.State, error) {
		return states, nil
	}
}

func TestAcceptIntakeWorkItem(t *testing.T) {
	record := newIntakeTriageRecord(plane.IntakeStatusPending)
	resolver := newIntakeTriageResolver()

	t.Run("success verifies applied transition and annotates visibility", func(t *testing.T) {
		patchCalls := 0
		var patchedUUID string
		var patchedBody map[string]any
		var formatted []plane.IntakeWorkItem
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				patchedUUID = issueID
				patchedBody = body
				accepted := *record
				accepted.Status = plane.IntakeStatusAccepted
				return &accepted, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				accepted := *record
				accepted.Status = plane.IntakeStatusAccepted
				return &accepted, nil
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
			listStatesFn:              statesFn([]plane.State{{ID: "st-default", Name: "Todo", Default: true}}),
		}
		formatter := newIntakeTriageFormatter(&formatted)

		result, err := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, formatter)
		if err != nil || result.IsError {
			t.Fatalf("accept failed: %v %+v", err, result)
		}
		if patchCalls != 1 || patchedUUID != "issue-10" || patchedBody["status"] != plane.IntakeStatusAccepted {
			t.Fatalf("unexpected PATCH: calls=%d uuid=%q body=%+v", patchCalls, patchedUUID, patchedBody)
		}
		if len(formatted) != 1 || formatted[0].Status != plane.IntakeStatusAccepted || formatted[0].ResolvedIdentifier != "ASBX-10" || !formatted[0].VisibleInWorkItems {
			t.Fatalf("expected verified accepted visible item, got %+v", formatted)
		}
	})

	t.Run("idempotent no-op skips PATCH when already accepted", func(t *testing.T) {
		accepted := newIntakeTriageRecord(plane.IntakeStatusAccepted)
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(accepted),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}
		formatter := newIntakeTriageFormatter(nil)

		result, err := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, formatter)
		if err != nil || result.IsError {
			t.Fatalf("idempotent accept failed: %v %+v", err, result)
		}
		if patchCalls != 0 {
			t.Fatalf("expected no PATCH for already-accepted record, got %d", patchCalls)
		}
		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "No change needed") {
			t.Errorf("expected no-change notice, got: %s", text)
		}
	})

	t.Run("HTTP 200 no-op yields typed transition_not_applied with permission cause", func(t *testing.T) {
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				stillPending := *record
				return &stillPending, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				stillPending := *record
				return &stillPending, nil
			},
			listStatesFn: statesFn([]plane.State{{ID: "st-default", Name: "Todo", Default: true}}),
		}

		result, err := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil {
			t.Fatalf("handler returned Go error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected an MCP tool error for unapplied transition")
		}
		text := result.Content[0].(*mcp.TextContent).Text
		for _, want := range []string{"transition_not_applied", `"pending"`, `"accepted"`, "insufficient"} {
			if !strings.Contains(text, want) {
				t.Errorf("error text missing %q: %s", want, text)
			}
		}
	})

	t.Run("missing default state is reported as a cause for failed acceptance", func(t *testing.T) {
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				stillPending := *record
				return &stillPending, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				stillPending := *record
				return &stillPending, nil
			},
			listStatesFn: statesFn([]plane.State{{ID: "st-1", Name: "Todo"}}),
		}

		result, _ := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, newIntakeTriageFormatter(nil))
		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "no default state configured") {
			t.Errorf("expected missing-default-state cause, got: %s", text)
		}
	})

	t.Run("unknown identifier reports active-queue miss", func(t *testing.T) {
		client := &mockClient{listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
			return nil, nil
		}}

		result, _ := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-99"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "not found in the active intake queue") {
			t.Fatalf("unexpected result for unknown identifier: %+v", result)
		}
	})

	t.Run("missing underlying issue UUID fails before PATCH", func(t *testing.T) {
		uuidless := &plane.IntakeWorkItem{
			ID:     "intake-10",
			Status: plane.IntakeStatusPending,
			Issue:  plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{SequenceID: 10}},
		}
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(uuidless),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
		}

		result, _ := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "no underlying issue UUID") {
			t.Fatalf("unexpected result for missing UUID: %+v", result)
		}
		if patchCalls != 0 {
			t.Errorf("PATCH must not fire without an issue UUID, got %d calls", patchCalls)
		}
	})

	t.Run("PATCH API failure surfaces as tool error", func(t *testing.T) {
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return nil, errors.New("intake PATCH failed")
			},
		}

		result, _ := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "failed to transition") {
			t.Fatalf("unexpected result for API failure: %+v", result)
		}
	})

	t.Run("verification re-read failure surfaces as tool error", func(t *testing.T) {
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				accepted := *record
				accepted.Status = plane.IntakeStatusAccepted
				return &accepted, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				return nil, errors.New("verification read failed")
			},
		}

		result, _ := acceptIntakeWorkItem(context.Background(), AcceptIntakeWorkItemArgs{Identifier: "ASBX-10"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "failed to verify") {
			t.Fatalf("unexpected result for verification failure: %+v", result)
		}
	})
}

func TestDeclineIntakeWorkItem(t *testing.T) {
	record := newIntakeTriageRecord(plane.IntakeStatusPending)
	resolver := newIntakeTriageResolver()
	newDeclinedClient := func(commentErr error, order *[]string) *mockClient {
		return &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
				*order = append(*order, "comment")
				return commentErr
			},
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				*order = append(*order, "patch")
				declined := *record
				declined.Status = plane.IntakeStatusDeclined
				return &declined, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				declined := *record
				declined.Status = plane.IntakeStatusDeclined
				return &declined, nil
			},
			getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
				return nil, &plane.APIError{StatusCode: 404, Body: "not found"}
			},
		}
	}

	t.Run("success transitions to declined without a reason", func(t *testing.T) {
		var order []string
		result, err := declineIntakeWorkItem(context.Background(), DeclineIntakeWorkItemArgs{Identifier: "ASBX-10"}, newDeclinedClient(nil, &order), resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("decline failed: %v %+v", err, result)
		}
		if len(order) != 1 || order[0] != "patch" {
			t.Errorf("expected exactly one PATCH and no comment, got order %v", order)
		}
	})

	t.Run("reason posts as a comment before transitioning", func(t *testing.T) {
		var order []string
		var capturedComment string
		client := newDeclinedClient(nil, &order)
		commentFn := client.createWorkItemCommentFn
		client.createWorkItemCommentFn = func(ctx context.Context, projectID, itemID, comment string) error {
			capturedComment = comment
			return commentFn(ctx, projectID, itemID, comment)
		}

		result, err := declineIntakeWorkItem(context.Background(), DeclineIntakeWorkItemArgs{Identifier: "ASBX-10", Reason: strPtr("Out of scope for Q3")}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("decline with reason failed: %v %+v", err, result)
		}
		if capturedComment == "" || !strings.Contains(capturedComment, "Out of scope for Q3") {
			t.Errorf("expected reason recorded as comment, got %q", capturedComment)
		}
		if len(order) < 2 || order[0] != "comment" || order[1] != "patch" {
			t.Errorf("expected comment before patch, got order %v", order)
		}
	})

	t.Run("comment failure aborts before declining", func(t *testing.T) {
		var order []string
		client := newDeclinedClient(errors.New("comment API down"), &order)

		result, _ := declineIntakeWorkItem(context.Background(), DeclineIntakeWorkItemArgs{Identifier: "ASBX-10", Reason: strPtr("nope")}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError {
			t.Fatal("expected tool error when the reason comment cannot be posted")
		}
		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "NOT declined") {
			t.Errorf("expected explicit not-declined notice, got: %s", text)
		}
		for _, step := range order {
			if step == "patch" {
				t.Error("PATCH must not run when the reason comment fails")
			}
		}
	})

	t.Run("reason is not re-posted on an already-declined record", func(t *testing.T) {
		declined := newIntakeTriageRecord(plane.IntakeStatusDeclined)
		patchCalls := 0
		commentCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(declined),
			createWorkItemCommentFn: func(ctx context.Context, projectID, itemID, comment string) error {
				commentCalls++
				return nil
			},
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}

		result, err := declineIntakeWorkItem(context.Background(), DeclineIntakeWorkItemArgs{Identifier: "ASBX-10", Reason: strPtr("Already handled")}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("idempotent decline failed: %v %+v", err, result)
		}
		if patchCalls != 0 || commentCalls != 0 {
			t.Errorf("expected no writes for already-declined record, got patch=%d comment=%d", patchCalls, commentCalls)
		}
		if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "No change needed") {
			t.Error("expected no-change notice")
		}
	})
}

func TestSnoozeIntakeWorkItem(t *testing.T) {
	record := newIntakeTriageRecord(plane.IntakeStatusPending)
	resolver := newIntakeTriageResolver()

	t.Run("success normalizes deadline to UTC and verifies storage", func(t *testing.T) {
		var patchedBody map[string]any
		var formatted []plane.IntakeWorkItem
		snoozed := newIntakeTriageRecord(plane.IntakeStatusSnoozed)
		snoozed.SnoozedTill = strPtr("2026-08-29T18:42:39Z")
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchedBody = body
				return snoozeCopy(record), nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				return cloneIntake(snoozed), nil
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}

		result, err := snoozeIntakeWorkItem(context.Background(), SnoozeIntakeWorkItemArgs{Identifier: "ASBX-10", SnoozedTill: "2026-08-29T20:42:39+02:00"}, client, resolver, newIntakeTriageFormatter(&formatted))
		if err != nil || result.IsError {
			t.Fatalf("snooze failed: %v %+v", err, result)
		}
		if patchedBody["status"] != plane.IntakeStatusSnoozed || patchedBody["snoozed_till"] != "2026-08-29T18:42:39Z" {
			t.Fatalf("unexpected snooze body: %+v", patchedBody)
		}
		if len(formatted) != 1 || formatted[0].SnoozedTill == nil || *formatted[0].SnoozedTill != "2026-08-29T18:42:39Z" {
			t.Fatalf("expected verified snoozed_till in output, got %+v", formatted)
		}
	})

	t.Run("invalid RFC3339 timestamp rejected upfront", func(t *testing.T) {
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
		}

		result, _ := snoozeIntakeWorkItem(context.Background(), SnoozeIntakeWorkItemArgs{Identifier: "ASBX-10", SnoozedTill: "tomorrow"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "invalid snoozed_till") {
			t.Fatalf("unexpected result for invalid timestamp: %+v", result)
		}
		if patchCalls != 0 {
			t.Errorf("PATCH must not fire for invalid timestamps, got %d", patchCalls)
		}
	})

	t.Run("expired timestamp rejected because Plane hides expired records", func(t *testing.T) {
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
		}

		result, _ := snoozeIntakeWorkItem(context.Background(), SnoozeIntakeWorkItemArgs{Identifier: "ASBX-10", SnoozedTill: "2020-01-01T00:00:00Z"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError {
			t.Fatal("expected expired snoozed_till rejection")
		}
		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "expired") || !strings.Contains(text, "hides expired snoozed records") {
			t.Errorf("expected expiry rationale, got: %s", text)
		}
		if patchCalls != 0 {
			t.Errorf("PATCH must not fire for expired timestamps, got %d", patchCalls)
		}
	})

	t.Run("stored deadline mismatch yields transition_not_applied", func(t *testing.T) {
		stillPendingWithWrongDeadline := newIntakeTriageRecord(plane.IntakeStatusSnoozed)
		stillPendingWithWrongDeadline.SnoozedTill = strPtr("2026-09-01T00:00:00Z")
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return cloneIntake(stillPendingWithWrongDeadline), nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				return cloneIntake(stillPendingWithWrongDeadline), nil
			},
		}

		result, _ := snoozeIntakeWorkItem(context.Background(), SnoozeIntakeWorkItemArgs{Identifier: "ASBX-10", SnoozedTill: "2030-01-01T00:00:00Z"}, client, resolver, newIntakeTriageFormatter(nil))
		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "transition_not_applied") || !strings.Contains(text, "did not store the requested snoozed_till") {
			t.Errorf("expected typed no-op error mentioning snoozed_till, got: %s", text)
		}
	})

	t.Run("already snoozed at requested deadline short-circuits idempotently", func(t *testing.T) {
		snoozed := newIntakeTriageRecord(plane.IntakeStatusSnoozed)
		snoozed.SnoozedTill = strPtr("2030-01-01T00:00:00Z")
		patchCalls := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(snoozed),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return nil, errors.New("must not be called")
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}

		result, err := snoozeIntakeWorkItem(context.Background(), SnoozeIntakeWorkItemArgs{Identifier: "ASBX-10", SnoozedTill: "2030-01-01T00:00:00Z"}, client, resolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("idempotent snooze failed: %v %+v", err, result)
		}
		if patchCalls != 0 {
			t.Errorf("expected no PATCH, got %d", patchCalls)
		}
		if !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "No change needed") {
			t.Error("expected no-change notice")
		}
	})

	t.Run("re-snoozing an active snooze to a later deadline patches again", func(t *testing.T) {
		active := newIntakeTriageRecord(plane.IntakeStatusSnoozed)
		active.SnoozedTill = strPtr("2026-09-01T00:00:00Z")
		patchCalls := 0
		later := newIntakeTriageRecord(plane.IntakeStatusSnoozed)
		later.SnoozedTill = strPtr("2030-01-01T00:00:00Z")
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(active),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				return cloneIntake(later), nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				return cloneIntake(later), nil
			},
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}

		if _, err := snoozeIntakeWorkItem(context.Background(), SnoozeIntakeWorkItemArgs{Identifier: "ASBX-10", SnoozedTill: "2030-01-01T00:00:00Z"}, client, resolver, newIntakeTriageFormatter(nil)); err != nil {
			t.Fatalf("extend-snooze failed: %v", err)
		}
		if patchCalls != 1 {
			t.Errorf("expected exactly one PATCH for deadline extension, got %d", patchCalls)
		}
	})
}

func TestMarkIntakeDuplicate(t *testing.T) {
	record := newIntakeTriageRecord(plane.IntakeStatusPending)
	resolver := newIntakeTriageResolver()

	newDuplicateClient := func(targetSeq int, target *plane.WorkItem, targetErr error, appliedDuplicateTo *string, patchedUUIDs *[]string) *mockClient {
		return &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
				if sequenceID == targetSeq {
					return target, targetErr
				}
				return &plane.WorkItem{ID: "issue-10", SequenceID: sequenceID}, nil
			},
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				*patchedUUIDs = append(*patchedUUIDs, issueID)
				marked := *record
				marked.Status = plane.IntakeStatusDuplicate
				if appliedDuplicateTo != nil {
					*appliedDuplicateTo = body["duplicate_to"].(string)
					marked.DuplicateTo = appliedDuplicateTo
				}
				return &marked, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				marked := *record
				marked.Status = plane.IntakeStatusDuplicate
				if appliedDuplicateTo != nil {
					marked.DuplicateTo = appliedDuplicateTo
				}
				return &marked, nil
			},
		}
	}

	t.Run("success resolves canonical target and never patches it", func(t *testing.T) {
		applied := ""
		var patchedUUIDs []string
		var formatted []plane.IntakeWorkItem
		client := newDuplicateClient(12, &plane.WorkItem{ID: "issue-target", SequenceID: 12}, nil, &applied, &patchedUUIDs)

		result, err := markIntakeDuplicate(context.Background(), MarkIntakeDuplicateArgs{Identifier: "ASBX-10", DuplicateTo: "ASBX-12"}, client, resolver, newIntakeTriageFormatter(&formatted))
		if err != nil || result.IsError {
			t.Fatalf("mark duplicate failed: %v %+v", err, result)
		}
		if applied != "issue-target" {
			t.Errorf("expected duplicate_to=issue-target, got %q", applied)
		}
		if len(patchedUUIDs) != 1 || patchedUUIDs[0] != "issue-10" {
			t.Errorf("only the source record may be patched, patched: %v", patchedUUIDs)
		}
		if len(formatted) != 1 || formatted[0].DuplicateTo == nil || *formatted[0].DuplicateTo != "issue-target" || formatted[0].Status != plane.IntakeStatusDuplicate {
			t.Fatalf("expected verified duplicate in output, got %+v", formatted)
		}
	})

	t.Run("cross-project target resolves through its own project prefix", func(t *testing.T) {
		applied := ""
		var patchedUUIDs []string
		var lookupPrefixes []string
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
				if sequenceID == 10 {
					return &plane.WorkItem{ID: "issue-10", SequenceID: sequenceID}, nil
				}
				lookupPrefixes = append(lookupPrefixes, projectIdentifier)
				return &plane.WorkItem{ID: "issue-core-12", SequenceID: sequenceID}, nil
			},
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchedUUIDs = append(patchedUUIDs, issueID)
				marked := *record
				marked.Status = plane.IntakeStatusDuplicate
				applied = body["duplicate_to"].(string)
				marked.DuplicateTo = strPtr(applied)
				return &marked, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				marked := *record
				marked.Status = plane.IntakeStatusDuplicate
				marked.DuplicateTo = strPtr("issue-core-12")
				return &marked, nil
			},
		}
		multiResolver := &mockResolver{resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			switch input {
			case "CORE":
				return &plane.Project{ID: "project-core", Identifier: "CORE"}, nil
			default:
				return &plane.Project{ID: "project-1", Identifier: "ASBX"}, nil
			}
		}}

		result, err := markIntakeDuplicate(context.Background(), MarkIntakeDuplicateArgs{Identifier: "ASBX-10", DuplicateTo: "CORE-12"}, client, multiResolver, newIntakeTriageFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("cross-project duplicate failed: %v %+v", err, result)
		}
		if len(lookupPrefixes) != 1 || lookupPrefixes[0] != "CORE" {
			t.Errorf("target must be looked up in project CORE, lookups: %v", lookupPrefixes)
		}
		if applied != "issue-core-12" {
			t.Errorf("expected duplicate_to=issue-core-12, got %q", applied)
		}
		if len(patchedUUIDs) != 1 || patchedUUIDs[0] != "issue-10" {
			t.Errorf("only the source record may be patched, patched: %v", patchedUUIDs)
		}
	})

	t.Run("self-duplicate rejected before PATCH", func(t *testing.T) {
		var patchedUUIDs []string
		client := newDuplicateClient(10, &plane.WorkItem{ID: "issue-10", SequenceID: 10}, nil, nil, &patchedUUIDs)

		result, _ := markIntakeDuplicate(context.Background(), MarkIntakeDuplicateArgs{Identifier: "ASBX-10", DuplicateTo: "ASBX-10"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "duplicate of itself") {
			t.Fatalf("unexpected self-duplicate result: %+v", result)
		}
		if len(patchedUUIDs) != 0 {
			t.Errorf("PATCH must not fire for self-duplicates, got %v", patchedUUIDs)
		}
	})

	t.Run("missing target rejected with guidance", func(t *testing.T) {
		var patchedUUIDs []string
		targetErr := &plane.APIError{StatusCode: 404, Body: "not found"}
		client := newDuplicateClient(99, nil, targetErr, nil, &patchedUUIDs)

		result, _ := markIntakeDuplicate(context.Background(), MarkIntakeDuplicateArgs{Identifier: "ASBX-10", DuplicateTo: "ASBX-99"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "was not found") {
			t.Fatalf("unexpected missing-target result: %+v", result)
		}
		if len(patchedUUIDs) != 0 {
			t.Errorf("PATCH must not fire without a target, got %v", patchedUUIDs)
		}
	})

	t.Run("malformed target identifier rejected", func(t *testing.T) {
		var patchedUUIDs []string
		client := newDuplicateClient(12, &plane.WorkItem{ID: "issue-target"}, nil, nil, &patchedUUIDs)

		result, _ := markIntakeDuplicate(context.Background(), MarkIntakeDuplicateArgs{Identifier: "ASBX-10", DuplicateTo: "not-an-identifier"}, client, resolver, newIntakeTriageFormatter(nil))
		if !result.IsError || !strings.Contains(result.Content[0].(*mcp.TextContent).Text, "invalid duplicate_to") {
			t.Fatalf("unexpected malformed-target result: %+v", result)
		}
		if len(patchedUUIDs) != 0 {
			t.Errorf("PATCH must not fire for malformed targets, got %v", patchedUUIDs)
		}
	})

	t.Run("stored duplicate_to mismatch yields transition_not_applied", func(t *testing.T) {
		unapplied := newIntakeTriageRecord(plane.IntakeStatusPending)
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(record),
			getWorkItemByIdentifierFn: func(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error) {
				return &plane.WorkItem{ID: "issue-target", SequenceID: 12}, nil
			},
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return cloneIntake(unapplied), nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				return cloneIntake(unapplied), nil
			},
		}

		result, _ := markIntakeDuplicate(context.Background(), MarkIntakeDuplicateArgs{Identifier: "ASBX-10", DuplicateTo: "ASBX-12"}, client, resolver, newIntakeTriageFormatter(nil))
		text := result.Content[0].(*mcp.TextContent).Text
		if !strings.Contains(text, "transition_not_applied") || !strings.Contains(text, "did not store the requested duplicate_to") {
			t.Errorf("expected typed no-op error mentioning duplicate_to, got: %s", text)
		}
	})
}

// snoozeClone helpers -------------------------------------------------------

func cloneIntake(item *plane.IntakeWorkItem) *plane.IntakeWorkItem {
	clone := *item
	return &clone
}

func snoozeCopy(record *plane.IntakeWorkItem) *plane.IntakeWorkItem {
	clone := cloneIntake(record)
	clone.Status = plane.IntakeStatusSnoozed
	clone.SnoozedTill = strPtr("2026-08-29T18:42:39Z")
	return clone
}

// TestIntakeTriageToolsRegistrationAndAnnotations asserts profile gating,
// mutation/idempotency annotations, and required schema fields for the four
// triage tools.
func TestIntakeTriageToolsRegistrationAndAnnotations(t *testing.T) {
	triageToolNames := []string{
		"accept_intake_work_item",
		"decline_intake_work_item",
		"snooze_intake_work_item",
		"mark_intake_duplicate",
	}

	cfgFor := func(profile string) *config.Config { return &config.Config{PlaneMCPProfile: profile} }

	for _, name := range triageToolNames {
		if !shouldRegister(name, plannerFull, cfgFor("full")) {
			t.Errorf("%s should register for the full profile", name)
		}
		if shouldRegister(name, plannerFull, cfgFor("reviewer")) {
			t.Errorf("%s must NOT register for reviewer profiles", name)
		}
		if shouldRegister(name, plannerFull, cfgFor("worker")) {
			t.Errorf("%s must NOT register for worker profiles", name)
		}
	}

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
	client := &mockClient{}
	registerWithDeps(server, client, newIntakeTriageResolver(), newIntakeTriageFormatter(nil), cfgFor("full"))

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

	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	toolsByName := make(map[string]*mcp.Tool, len(tools.Tools))
	for _, tool := range tools.Tools {
		toolsByName[tool.Name] = tool
	}

	requiredByTool := map[string][]string{
		"accept_intake_work_item":  {"identifier"},
		"decline_intake_work_item": {"identifier"},
		"snooze_intake_work_item":  {"identifier", "snoozed_till"},
		"mark_intake_duplicate":    {"identifier", "duplicate_to"},
	}
	for _, name := range triageToolNames {
		tool, ok := toolsByName[name]
		if !ok {
			t.Fatalf("%s was not registered under the full profile", name)
		}
		if tool.Annotations == nil {
			t.Fatalf("%s has no annotations", name)
		}
		if tool.Annotations.ReadOnlyHint {
			t.Errorf("%s mutates state and must not claim read-only", name)
		}
		if tool.Annotations.IdempotentHint != true {
			t.Errorf("%s should be marked idempotent", name)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Errorf("%s should be marked non-destructive", name)
		}
		schemaJSON, err := json.Marshal(tool.InputSchema)
		if err != nil {
			t.Fatalf("marshal %s schema: %v", name, err)
		}
		for _, required := range requiredByTool[name] {
			if !strings.Contains(string(schemaJSON), `"`+required+`"`) {
				t.Errorf("%s schema missing %q: %s", name, required, schemaJSON)
			}
		}
	}
}
