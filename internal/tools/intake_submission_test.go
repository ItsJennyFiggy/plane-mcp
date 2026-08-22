package tools

import (
	"context"
	"errors"
	"fmt"
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

// realIntakeFormatter delegates to the production formatter so assertions can
// cover the actual serialized output rather than a static echo.
func realIntakeFormatter(captured *[]plane.IntakeWorkItem) *mockFormatter {
	return &mockFormatter{formatIntakeWorkItemsYAMLFn: func(ctx context.Context, items []plane.IntakeWorkItem) (string, error) {
		if captured != nil {
			*captured = items
		}
		return plane.FormatIntakeWorkItemsYAML(ctx, items)
	}}
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

// resultString renders an arbitrary body field for assertions.
func resultString(value any) string {
	s, _ := value.(string)
	return s
}

func TestCreateIntakeWorkItem(t *testing.T) {
	resolver := newIntakeTriageResolver()

	t.Run("success submits nested issue body and reports identifiers", func(t *testing.T) {
		var gotProjectID string
		var gotBody map[string]any
		client := &mockClient{
			createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				gotProjectID = projectID
				gotBody = body
				return newCreatedIntakeRecord(), nil
			},
		}

		result, err := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project:     "ASBX",
			Name:        "Quick idea",
			Description: "A simple idea",
			Priority:    "high",
		}, client, resolver, realIntakeFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("create failed: %v %+v", err, result)
		}

		if gotProjectID != "project-1" {
			t.Errorf("expected resolved project UUID, got %q", gotProjectID)
		}
		if gotBody["name"] != "Quick idea" || gotBody["priority"] != "high" {
			t.Errorf("unexpected issue body: %+v", gotBody)
		}
		html, _ := gotBody["description_html"].(string)
		if html == "" || !strings.HasPrefix(html, "<") {
			t.Errorf("expected HTML description, got %q", html)
		}

		// The serialized output must carry both advertised identifiers plus
		// canonical status and attribution.
		out := resultText(t, result)
		for _, want := range []string{
			"intake_id:", "intake-11",
			"issue_id:", "issue-uuid-11",
			"identifier: ASBX-11",
			"status: pending",
			"source: IN_APP",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("serialized output missing %q:\n%s", want, out)
			}
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
		}, client, resolver, realIntakeFormatter(nil))
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
			Source: "IN_APP",
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
		}, client, resolver, realIntakeFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("create failed: %v %+v", err, result)
		}
		if listCalls != 1 {
			t.Fatalf("expected one reconcile list read, got %d", listCalls)
		}
		if out := resultText(t, result); !strings.Contains(out, "identifier: ASBX-11") {
			t.Errorf("reconciled identifier missing from output:\n%s", out)
		}
	})

	t.Run("unresolvable identifier fails loudly instead of guessing", func(t *testing.T) {
		unexpanded := &plane.IntakeWorkItem{
			ID:     "intake-11",
			Status: plane.IntakeStatusPending,
			Source: "IN_APP",
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
		}, client, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "could not be confirmed in the queue") {
			t.Fatalf("expected reconcile failure error, got: %+v", result)
		}
	})

	t.Run("fails closed on contract-violating responses", func(t *testing.T) {
		cases := []struct {
			name   string
			record *plane.IntakeWorkItem
			want   string
		}{
			{
				name: "non-pending status",
				record: func() *plane.IntakeWorkItem {
					r := newCreatedIntakeRecord()
					r.Status = plane.IntakeStatusAccepted
					return r
				}(),
				want: "expected canonical pending status",
			},
			{
				name: "missing source attribution",
				record: func() *plane.IntakeWorkItem {
					r := newCreatedIntakeRecord()
					r.Source = ""
					return r
				}(),
				want: "expected IN_APP source attribution",
			},
			{
				name: "wrong source attribution",
				record: func() *plane.IntakeWorkItem {
					r := newCreatedIntakeRecord()
					r.Source = "EMAIL"
					return r
				}(),
				want: "expected IN_APP source attribution",
			},
			{
				name: "missing underlying issue UUID",
				record: func() *plane.IntakeWorkItem {
					// Expanded issue with a sequence but an empty ID: the
					// identifier derives fine, so validation must catch the
					// unusable output contract.
					r := newCreatedIntakeRecord()
					r.Issue = plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{
						Name: "Quick idea", SequenceID: 11,
					}}
					return r
				}(),
				want: "no underlying issue UUID",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				client := &mockClient{
					createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
						return tc.record, nil
					},
					listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
						return nil, nil
					},
				}

				result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
					Project: "ASBX", Name: "Quick idea",
				}, client, resolver, realIntakeFormatter(nil))
				if !result.IsError || !strings.Contains(resultText(t, result), tc.want) {
					t.Fatalf("expected fail-closed error containing %q, got: %+v", tc.want, result)
				}
			})
		}
	})

	t.Run("fails closed on empty or unusable creation responses", func(t *testing.T) {
		cases := []struct {
			name   string
			record *plane.IntakeWorkItem
			want   string
		}{
			{name: "nil creation response", record: nil, want: "Plane returned an empty response"},
			{name: "empty intake record id", record: &plane.IntakeWorkItem{}, want: "Plane returned an empty response"},
			{
				// Reconciliation finds the new record but its issue carries
				// no positive sequence: both identifiers can never be
				// established, so the call must not serialize a success.
				name: "reconciled record without usable sequence",
				record: func() *plane.IntakeWorkItem {
					return &plane.IntakeWorkItem{
						ID:     "intake-11",
						Status: plane.IntakeStatusPending,
						Source: "IN_APP",
						Issue:  plane.Expandable[plane.IntakeIssue]{ID: "issue-uuid-11"},
					}
				}(),
				want: "could not derive the canonical identifier",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				client := &mockClient{
					createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
						return tc.record, nil
					},
					listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
						if tc.record == nil {
							return nil, nil
						}
						matched := *tc.record
						matched.Issue = plane.Expandable[plane.IntakeIssue]{Val: &plane.IntakeIssue{
							ID: "issue-uuid-11", Name: "Quick idea", SequenceID: 0,
						}}
						return []plane.IntakeWorkItem{matched}, nil
					},
				}

				result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
					Project: "ASBX", Name: "Quick idea",
				}, client, resolver, realIntakeFormatter(nil))
				if !result.IsError || !strings.Contains(resultText(t, result), tc.want) {
					t.Fatalf("expected fail-closed error containing %q, got: %+v", tc.want, result)
				}
			})
		}
	})

	t.Run("malformed project identifier cannot produce a pseudo-canonical identifier", func(t *testing.T) {
		// A resolver returning an empty identifier would otherwise yield
		// "-11"; the derived value must be rejected by the canonical parser.
		badResolver := &mockResolver{resolveProjectFn: func(ctx context.Context, input string) (*plane.Project, error) {
			return &plane.Project{ID: "project-1", Identifier: ""}, nil
		}}
		client := &mockClient{
			createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return newCreatedIntakeRecord(), nil
			},
			listIntakeWorkItemsFn: func(ctx context.Context, projectID string) ([]plane.IntakeWorkItem, error) {
				return []plane.IntakeWorkItem{*newCreatedIntakeRecord()}, nil
			},
		}

		result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project: "ASBX", Name: "Quick idea",
		}, client, badResolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "could not derive the canonical identifier") {
			t.Fatalf("expected malformed-prefix failure, got: %+v", result)
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
		}, client, resolver, realIntakeFormatter(nil))
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

		for _, bad := range []string{"critical", "-2", "asap"} {
			result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
				Project: "ASBX", Name: "Idea", Priority: bad,
			}, client, resolver, realIntakeFormatter(nil))
			if !result.IsError || !strings.Contains(resultText(t, result), fmt.Sprintf("invalid priority %q", bad)) {
				t.Fatalf("priority %q: expected validation error, got: %+v", bad, result)
			}
		}
		if calls != 0 {
			t.Fatalf("expected no API call for invalid priorities, got %d", calls)
		}
	})

	t.Run("priority values are normalized case-insensitively", func(t *testing.T) {
		for input, want := range map[string]string{"HIGH": "high", " Urgent ": "urgent"} {
			got, err := normalizeIntakePriority(input)
			if err != nil || got != want {
				t.Errorf("normalizeIntakePriority(%q) = %q, %v; want %q, nil", input, got, err, want)
			}
		}
	})

	t.Run("API failure surfaces as tool error", func(t *testing.T) {
		client := &mockClient{createIntakeItemFn: func(ctx context.Context, projectID string, body map[string]any) (*plane.IntakeWorkItem, error) {
			return nil, errors.New("400 Bad Request")
		}}

		result, _ := createIntakeWorkItem(context.Background(), CreateIntakeWorkItemArgs{
			Project: "ASBX", Name: "Idea", Priority: "high",
		}, client, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "failed to create intake work item") {
			t.Fatalf("expected creation failure, got: %+v", result)
		}
	})
}

func TestUpdateIntakeWorkItem(t *testing.T) {
	resolver := newIntakeTriageResolver()

	newPendingRecord := func(name string, priority string) *plane.IntakeWorkItem {
		record := newIntakeTriageRecord(plane.IntakeStatusPending)
		record.Issue.Val.Name = name
		record.Issue.Val.Priority = priority
		return record
	}

	// serveCurrent returns a getIntakeWorkItemFn serving deep-ish copies of
	// the caller-owned current record, so PATCH callbacks can mutate it and
	// subsequent reads observe the mutation.
	serveCurrent := func(current *plane.IntakeWorkItem, reads *int) func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
		return func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
			if reads != nil {
				*reads++
			}
			cp := *current
			if current.Issue.Val != nil {
				iv := *current.Issue.Val
				cp.Issue.Val = &iv
			}
			return &cp, nil
		}
	}

	t.Run("success verifies stored fields including description and annotates visibility", func(t *testing.T) {
		current := newPendingRecord("Idea", "none")
		var patchedUUID string
		var patchedBody map[string]any
		var formatted []plane.IntakeWorkItem
		detailReads := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchedUUID = issueID
				patchedBody = body
				current.Issue.Val = &plane.IntakeIssue{
					ID: "issue-10", SequenceID: 10, Name: "Renamed idea", Priority: "high",
					DescriptionHTML: "<p>Rich text</p>",
				}
				cp := *current
				return &cp, nil
			},
			getIntakeWorkItemFn:       serveCurrent(current, &detailReads),
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}

		name := "Renamed idea"
		desc := "Rich text"
		priority := "high"
		result, err := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name, Description: &desc, Priority: &priority,
		}, client, resolver, realIntakeFormatter(&formatted))
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
		if got, want := resultString(issueFields["description_html"]), "<p>Rich text</p>"; got != want {
			t.Errorf("PATCH description_html = %q, want %q", got, want)
		}
		if _, hasStatus := patchedBody["status"]; hasStatus {
			t.Errorf("enrichment must not touch triage status, body was %+v", patchedBody)
		}
		// Two detail reads are mandatory: the pre-write freshness read and
		// the post-write verification read.
		if detailReads != 2 {
			t.Errorf("expected two detail reads (freshness + verification), got %d", detailReads)
		}
		if len(formatted) != 1 || formatted[0].UnderlyingIssue().Name != "Renamed idea" || formatted[0].ResolvedIdentifier != "ASBX-10" {
			t.Fatalf("unexpected verified output: %+v", formatted)
		}
	})

	t.Run("silently ignored fields are reported as enrichment_not_applied per field", func(t *testing.T) {
		cases := []struct {
			name     string
			args     func() UpdateIntakeWorkItemArgs
			wantTerm string
		}{
			{
				name: "name ignored",
				args: func() UpdateIntakeWorkItemArgs {
					v := "Renamed idea"
					return UpdateIntakeWorkItemArgs{Identifier: "ASBX-10", Name: &v}
				},
				wantTerm: "name",
			},
			{
				name: "description ignored",
				args: func() UpdateIntakeWorkItemArgs {
					v := "New words"
					return UpdateIntakeWorkItemArgs{Identifier: "ASBX-10", Description: &v}
				},
				wantTerm: "description",
			},
			{
				name: "priority ignored",
				args: func() UpdateIntakeWorkItemArgs {
					v := "urgent"
					return UpdateIntakeWorkItemArgs{Identifier: "ASBX-10", Priority: &v}
				},
				wantTerm: "priority",
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				current := newPendingRecord("Idea", "none")
				client := &mockClient{
					listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
					transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
						// The write is a no-op on the server side: the
						// verification read still sees the old values.
						cp := *current
						return &cp, nil
					},
					getIntakeWorkItemFn: serveCurrent(current, nil),
				}

				result, _ := updateIntakeWorkItem(context.Background(), tc.args(), client, resolver, realIntakeFormatter(nil))
				if !result.IsError {
					t.Fatal("expected MCP tool error for ignored enrichment")
				}
				text := resultText(t, result)
				for _, want := range []string{"enrichment_not_applied", tc.wantTerm, "silently drops", "may be insufficient"} {
					if !strings.Contains(text, want) {
						t.Errorf("error text missing %q: %s", want, text)
					}
				}
			})
		}
	})

	t.Run("status change during enrichment is reported as an error", func(t *testing.T) {
		// A concurrent actor transitions the record between the write and
		// the verification read: freshness read sees pending, verification
		// read sees accepted with the requested field applied.
		current := newPendingRecord("Idea", "none")
		detailReads := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				cp := *current
				return &cp, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				detailReads++
				cp := *current
				if current.Issue.Val != nil {
					iv := *current.Issue.Val
					cp.Issue.Val = &iv
				}
				if detailReads >= 2 {
					// Concurrent triage transition lands before verification.
					cp.Status = plane.IntakeStatusAccepted
					cp.Issue.Val.Name = "Renamed idea"
				}
				return &cp, nil
			},
		}

		name := "Renamed idea"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name,
		}, client, resolver, realIntakeFormatter(nil))
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

	t.Run("enrichment is restricted to pending records", func(t *testing.T) {
		disposed := []struct {
			status int
			label  string
		}{
			{plane.IntakeStatusAccepted, "accepted"},
			{plane.IntakeStatusDeclined, "declined"},
			{plane.IntakeStatusSnoozed, "snoozed"},
			{plane.IntakeStatusDuplicate, "duplicate"},
		}
		for _, tc := range disposed {
			t.Run(tc.label, func(t *testing.T) {
				current := newPendingRecord("Idea", "none")
				patchCalled := false
				client := &mockClient{
					listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
					transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
						patchCalled = true
						cp := *current
						return &cp, nil
					},
					getIntakeWorkItemFn: serveCurrent(current, nil),
				}
				current.Status = tc.status

				name := "Whatever"
				result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
					Identifier: "ASBX-10", Name: &name,
				}, client, resolver, realIntakeFormatter(nil))
				if !result.IsError {
					t.Fatalf("%s: expected rejection", tc.label)
				}
				text := resultText(t, result)
				if !strings.Contains(text, fmt.Sprintf("%q; enrichment is restricted to pending", tc.label)) {
					t.Errorf("%s: unexpected error text: %s", tc.label, text)
				}
				if patchCalled {
					t.Errorf("%s: enrichment PATCHed a disposed record", tc.label)
				}
			})
		}
	})

	t.Run("identical values are still written idempotently and verified", func(t *testing.T) {
		// Every success must flow through write + read-after-write
		// verification; no response may rely on pre-write reads alone.
		current := newPendingRecord("Idea", "high")
		patchCalls := 0
		detailReads := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "high")),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalls++
				cp := *current
				return &cp, nil
			},
			getIntakeWorkItemFn:       serveCurrent(current, &detailReads),
			getWorkItemByIdentifierFn: visibleTargetFn(),
		}

		name := "Idea"
		priority := "high"
		result, err := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name, Priority: &priority,
		}, client, resolver, realIntakeFormatter(nil))
		if err != nil || result.IsError {
			t.Fatalf("idempotent update failed: %v %+v", err, result)
		}
		if patchCalls != 1 {
			t.Fatalf("expected exactly one idempotent PATCH, got %d", patchCalls)
		}
		if detailReads != 2 {
			t.Fatalf("expected freshness + verification reads, got %d", detailReads)
		}
	})

	t.Run("pending gate uses the fresh detail read, not the stale queue list", func(t *testing.T) {
		// The queue list claims the record is still pending; only the fresh
		// detail read reveals it was concurrently disposed.
		patchCalled := false
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				patchCalled = true
				return newPendingRecord("Idea", "none"), nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				disposed := newPendingRecord("Idea", "none")
				disposed.Status = plane.IntakeStatusAccepted
				return disposed, nil
			},
		}

		name := "Fresh name"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name,
		}, client, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), `has status "accepted"`) {
			t.Fatalf("expected disposed-record rejection from fresh read, got: %+v", result)
		}
		if patchCalled {
			t.Fatal("enrichment PATCHed a concurrently disposed record")
		}
	})

	t.Run("PATCH API failure surfaces as tool error", func(t *testing.T) {
		current := newPendingRecord("Idea", "none")
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(newPendingRecord("Idea", "none")),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				return nil, errors.New("500 Internal Server Error")
			},
			getIntakeWorkItemFn: serveCurrent(current, nil),
		}

		name := "Renamed idea"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name,
		}, client, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "failed to update intake work item") {
			t.Fatalf("expected PATCH failure error, got: %+v", result)
		}
	})

	t.Run("verification-read failure surfaces as tool error", func(t *testing.T) {
		pending := newPendingRecord("Idea", "none")
		detailReads := 0
		client := &mockClient{
			listIntakeWorkItemsFn: baseIntakeListFn(pending),
			transitionIntakeItemFn: func(ctx context.Context, projectID, issueID string, body map[string]any) (*plane.IntakeWorkItem, error) {
				renamed := *pending
				renamed.Issue.Val.Name = "Renamed idea"
				return &renamed, nil
			},
			getIntakeWorkItemFn: func(ctx context.Context, projectID, issueID string) (*plane.IntakeWorkItem, error) {
				detailReads++
				if detailReads == 1 {
					return newPendingRecord("Idea", "none"), nil // freshness read OK
				}
				return nil, errors.New("503 Service Unavailable") // verification read fails
			},
		}

		name := "Renamed idea"
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &name,
		}, client, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "failed to verify intake update") {
			t.Fatalf("expected verification-read failure error, got: %+v", result)
		}
	})

	t.Run("no fields provided is rejected", func(t *testing.T) {
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10",
		}, &mockClient{}, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "nothing to update") {
			t.Fatalf("expected nothing-to-update error, got: %+v", result)
		}
	})

	t.Run("empty replacement name is rejected before any API call", func(t *testing.T) {
		empty := "  "
		result, _ := updateIntakeWorkItem(context.Background(), UpdateIntakeWorkItemArgs{
			Identifier: "ASBX-10", Name: &empty,
		}, &mockClient{}, resolver, realIntakeFormatter(nil))
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
		}, client, resolver, realIntakeFormatter(nil))
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
		}, client, resolver, realIntakeFormatter(nil))
		if !result.IsError || !strings.Contains(resultText(t, result), "not found in the active intake queue") {
			t.Fatalf("unexpected result for unknown identifier: %+v", result)
		}
	})
}

// TestRegisterWithDeps_IntakeSubmissionTools exercises the actual MCP
// registration surface over an in-memory client session: presence/absence per
// profile plus the annotations and schemas of the two new tools.
func TestRegisterWithDeps_IntakeSubmissionTools(t *testing.T) {
	ctx := context.Background()

	listTools := func(t *testing.T, profile string) map[string]*mcp.Tool {
		t.Helper()
		ct, st := mcp.NewInMemoryTransports()
		server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "0"}, nil)
		registerWithDeps(server, &mockClient{}, &mockResolver{}, &mockFormatter{}, &config.Config{PlaneMCPProfile: profile})
		if _, err := server.Connect(ctx, st, nil); err != nil {
			t.Fatalf("server connect failed: %v", err)
		}
		client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0"}, nil)
		cs, err := client.Connect(ctx, ct, nil)
		if err != nil {
			t.Fatalf("client connect failed: %v", err)
		}
		defer cs.Close()

		tools := map[string]*mcp.Tool{}
		for tool, err := range cs.Tools(ctx, nil) {
			if err != nil {
				t.Fatalf("tools listing failed: %v", err)
			}
			tools[tool.Name] = tool
		}
		return tools
	}

	expectations := map[string]map[string]bool{
		"worker":   {"create_intake_work_item": true, "update_intake_work_item": false},
		"reviewer": {"create_intake_work_item": true, "update_intake_work_item": false},
		"planner":  {"create_intake_work_item": true, "update_intake_work_item": true},
		"full":     {"create_intake_work_item": true, "update_intake_work_item": true},
	}
	for profile, expected := range expectations {
		t.Run(profile, func(t *testing.T) {
			tools := listTools(t, profile)
			for name, want := range expected {
				_, got := tools[name]
				if got != want {
					t.Errorf("profile %s: tool %s registered=%v, want %v", profile, name, got, want)
				}
			}
		})
	}

	// rawSchema converts the wire-format tool schema (a JSON-shaped map)
	// into accessor helpers for contract assertions.
	rawSchema := func(t *testing.T, input any) (props map[string]map[string]any, required []string) {
		t.Helper()
		m, ok := input.(map[string]any)
		if !ok {
			t.Fatalf("input schema unexpected type: %T", input)
		}
		if p, ok := m["properties"].(map[string]any); ok {
			props = map[string]map[string]any{}
			for name, v := range p {
				pm, ok := v.(map[string]any)
				if !ok {
					t.Fatalf("property %q unexpected type: %T", name, v)
				}
				props[name] = pm
			}
		}
		if r, ok := m["required"].([]any); ok {
			for _, v := range r {
				if s, ok := v.(string); ok {
					required = append(required, s)
				}
			}
		}
		return props, required
	}

	// assertPriorityEnum checks a priority property carries exactly the
	// canonical urgent/high/medium/low/none values.
	assertPriorityEnum := func(t *testing.T, prop map[string]any) {
		t.Helper()
		enumValues, _ := prop["enum"].([]any)
		want := []string{"urgent", "high", "medium", "low", "none"}
		if len(enumValues) != len(want) {
			t.Fatalf("priority enum incomplete: %v", enumValues)
		}
		got := map[string]bool{}
		for _, v := range enumValues {
			s, ok := v.(string)
			if !ok {
				t.Fatalf("priority enum non-string value: %#v", v)
			}
			got[s] = true
		}
		for _, w := range want {
			if !got[w] {
				t.Errorf("priority enum missing %q: %v", w, enumValues)
			}
		}
	}

	// assertExactProps fails when the property set is not exactly expected.
	assertExactProps := func(t *testing.T, props map[string]map[string]any, want []string) {
		t.Helper()
		if len(props) != len(want) {
			t.Errorf("property set = %v, want exactly %v", props, want)
		}
		for _, w := range want {
			if _, ok := props[w]; !ok {
				t.Errorf("schema missing property %q", w)
			}
		}
	}

	t.Run("annotations and schemas lock the submission/enrichment contracts", func(t *testing.T) {
		tools := listTools(t, "full")

		create := tools["create_intake_work_item"]
		if create == nil {
			t.Fatal("create_intake_work_item missing under full profile")
		}
		if create.Annotations == nil ||
			create.Annotations.ReadOnlyHint ||
			create.Annotations.DestructiveHint == nil || *create.Annotations.DestructiveHint ||
			create.Annotations.IdempotentHint ||
			create.Annotations.OpenWorldHint == nil || *create.Annotations.OpenWorldHint {
			t.Errorf("create annotations must be read/write, non-destructive, non-idempotent, closed-world: %+v", create.Annotations)
		}
		props, required := rawSchema(t, create.InputSchema)
		assertExactProps(t, props, []string{"project", "name", "description", "priority"})
		if len(required) != 2 || required[0] != "project" || required[1] != "name" {
			t.Errorf("create required = %v, want [project name]", required)
		}
		assertPriorityEnum(t, props["priority"])
		if got := fmt.Sprint(props["priority"]["default"]); got != "none" {
			t.Errorf("create priority default = %v, want none", props["priority"]["default"])
		}
		for prop, wantDesc := range map[string]string{
			"name":     "Short idea title.",
			"priority": "Optional priority; defaults to none.",
		} {
			if desc, _ := props[prop]["description"].(string); !strings.Contains(desc, wantDesc) {
				t.Errorf("create %q description = %q, want to contain %q", prop, desc, wantDesc)
			}
		}

		update := tools["update_intake_work_item"]
		if update == nil {
			t.Fatal("update_intake_work_item missing under full profile")
		}
		if update.Annotations == nil ||
			update.Annotations.ReadOnlyHint ||
			update.Annotations.DestructiveHint == nil || !*update.Annotations.DestructiveHint ||
			!update.Annotations.IdempotentHint ||
			update.Annotations.OpenWorldHint == nil || *update.Annotations.OpenWorldHint {
			t.Errorf("update annotations must be read/write, destructive, idempotent, closed-world: %+v", update.Annotations)
		}
		uprops, urequired := rawSchema(t, update.InputSchema)
		assertExactProps(t, uprops, []string{"identifier", "name", "description", "priority"})
		if len(urequired) != 1 || urequired[0] != "identifier" {
			t.Errorf("update required = %v, want [identifier]", urequired)
		}
		assertPriorityEnum(t, uprops["priority"])
		for prop, wantDesc := range map[string]string{
			"identifier": "Project-prefixed identifier of a pending Intake item",
			"name":       "replacement short title",
			"priority":   "Replacement priority.",
		} {
			desc, _ := uprops[prop]["description"].(string)
			if !strings.Contains(strings.ToLower(desc), strings.ToLower(wantDesc)) {
				t.Errorf("update %q description = %q, want to contain %q", prop, desc, wantDesc)
			}
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
