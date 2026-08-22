package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
)

// Retry policy constants for transparently recovering from downstream Plane API
// HTTP 429 responses. Kept zero-configuration: no public knobs are exposed.
const (
	// defaultRetryMaxAttempts is the total number of requests made for a single
	// call, including the initial attempt. The retry budget is exhausted after
	// this many requests.
	defaultRetryMaxAttempts = 4
	// defaultRetryBaseDelay is the initial exponential backoff delay applied when
	// no usable Retry-After value is present.
	defaultRetryBaseDelay = 200 * time.Millisecond
	// defaultRetryMaxDelay caps the delay for any single retry, whether derived
	// from Retry-After or the exponential backoff fallback.
	defaultRetryMaxDelay = 5 * time.Second
	// defaultRetryJitterMax is the upper bound of the uniform jitter added to the
	// exponential backoff fallback to avoid thundering-herd retries.
	defaultRetryJitterMax = 100 * time.Millisecond
)

// Expandable represents a field that can be either a string UUID or a fully expanded object.
type Expandable[T any] struct {
	ID  string
	Val *T
}

// UnmarshalJSON customizes unmarshaling to handle string UUIDs or full objects.
func (e *Expandable[T]) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return nil
	}

	// 1. Handle JSON string (UUID)
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		e.ID = s
		e.Val = nil
		return nil
	}

	// 2. Handle JSON null
	if string(data) == "null" {
		e.ID = ""
		e.Val = nil
		return nil
	}

	// 3. Handle JSON object
	var val T
	if err := json.Unmarshal(data, &val); err != nil {
		return err
	}
	e.Val = &val

	// Attempt to extract the "id" field from the object
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err == nil {
		if id, ok := m["id"].(string); ok {
			e.ID = id
		}
	}
	return nil
}

// MarshalJSON customizes marshaling.
func (e Expandable[T]) MarshalJSON() ([]byte, error) {
	if e.Val != nil {
		return json.Marshal(e.Val)
	}
	if e.ID != "" {
		return json.Marshal(e.ID)
	}
	return []byte("null"), nil
}

// Project model
type Project struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

// State model
type State struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Group    string  `json:"group"`
	Color    string  `json:"color"`
	Sequence float64 `json:"sequence"`
	Default  bool    `json:"default"`
}

// Label model
type Label struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Module model
type Module struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Member (UserLite) model
type Member struct {
	ID          string `json:"id"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
	Email       string `json:"email"`
	Avatar      string `json:"avatar"`
	AvatarURL   string `json:"avatar_url"`
	DisplayName string `json:"display_name"`
	Role        int    `json:"role"`
}

// SearchWorkItemResult represents a single result from the work-items/search endpoint.
type SearchWorkItemResult struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	SequenceID        int    `json:"sequence_id"`
	ProjectIdentifier string `json:"project__identifier"`
	ProjectID         string `json:"project_id"`
	WorkspaceSlug     string `json:"workspace__slug"`
}

// WorkItem model
type WorkItem struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	DescriptionHTML     string               `json:"description_html,omitempty"`
	DescriptionStripped string               `json:"description_stripped,omitempty"`
	Priority            string               `json:"priority,omitempty"`
	StartDate           string               `json:"start_date,omitempty"`
	TargetDate          string               `json:"target_date,omitempty"`
	SequenceID          int                  `json:"sequence_id"`
	SortOrder           float64              `json:"sort_order"`
	CompletedAt         string               `json:"completed_at,omitempty"`
	ArchivedAt          string               `json:"archived_at,omitempty"`
	IsDraft             bool                 `json:"is_draft"`
	Project             Expandable[Project]  `json:"project"`
	Workspace           string               `json:"workspace"`
	Parent              *string              `json:"parent,omitempty"`
	State               Expandable[State]    `json:"state"`
	EstimatePoint       *int                 `json:"estimate_point,omitempty"`
	Type                *string              `json:"type,omitempty"`
	Assignees           []Expandable[Member] `json:"assignees"`
	Labels              []Expandable[Label]  `json:"labels"`
}

// IntakeIssue is the expanded work item embedded in an IntakeWorkItem.
// Plane's intake serializer exposes more fields than the discovery tools need;
// the commonly useful fields are modeled here while preserving flexible
// description data from the API.
type IntakeIssue struct {
	ID                  string               `json:"id"`
	Name                string               `json:"name"`
	Description         any                  `json:"description,omitempty"`
	DescriptionHTML     string               `json:"description_html,omitempty"`
	DescriptionStripped string               `json:"description_stripped,omitempty"`
	Priority            string               `json:"priority,omitempty"`
	StartDate           string               `json:"start_date,omitempty"`
	TargetDate          string               `json:"target_date,omitempty"`
	SequenceID          int                  `json:"sequence_id"`
	CompletedAt         string               `json:"completed_at,omitempty"`
	ArchivedAt          string               `json:"archived_at,omitempty"`
	Project             Expandable[Project]  `json:"project"`
	State               Expandable[State]    `json:"state"`
	Assignees           []Expandable[Member] `json:"assignees,omitempty"`
	Labels              []Expandable[Label]  `json:"labels,omitempty"`
}

// IntakeWorkItem is a Plane intake queue record. The API's issue field is an
// underlying work-item UUID unless expand=issue is requested; issue_detail is
// also returned by recent Plane versions with the issue expanded.
type IntakeWorkItem struct {
	ID             string                  `json:"id"`
	Issue          Expandable[IntakeIssue] `json:"issue"`
	IssueDetail    *IntakeIssue            `json:"issue_detail,omitempty"`
	Inbox          string                  `json:"inbox,omitempty"`
	CreatedAt      string                  `json:"created_at,omitempty"`
	UpdatedAt      string                  `json:"updated_at,omitempty"`
	DeletedAt      string                  `json:"deleted_at,omitempty"`
	Status         int                     `json:"status"`
	SnoozedTill    *string                 `json:"snoozed_till,omitempty"`
	Source         string                  `json:"source,omitempty"`
	SourceEmail    string                  `json:"source_email,omitempty"`
	ExternalSource string                  `json:"external_source,omitempty"`
	ExternalID     string                  `json:"external_id,omitempty"`
	Extra          any                     `json:"extra,omitempty"`
	CreatedBy      string                  `json:"created_by,omitempty"`
	UpdatedBy      string                  `json:"updated_by,omitempty"`
	Project        Expandable[Project]     `json:"project"`
	Workspace      string                  `json:"workspace,omitempty"`
	Intake         string                  `json:"intake,omitempty"`
	DuplicateTo    *string                 `json:"duplicate_to,omitempty"`

	// ResolvedIdentifier and VisibleInWorkItems are populated by the tool
	// handler and are intentionally not part of the API model.
	ResolvedIdentifier string `json:"-"`
	VisibleInWorkItems bool   `json:"-"`
}

// UnderlyingIssue returns the expanded issue, preferring the explicit
// issue_detail field when the server supplied it.
func (i *IntakeWorkItem) UnderlyingIssue() *IntakeIssue {
	if i == nil {
		return nil
	}
	if i.IssueDetail != nil {
		return i.IssueDetail
	}
	return i.Issue.Val
}

// UnderlyingIssueID returns the UUID used by the intake detail route.
func (i *IntakeWorkItem) UnderlyingIssueID() string {
	if i == nil {
		return ""
	}
	if i.Issue.ID != "" {
		return i.Issue.ID
	}
	if issue := i.UnderlyingIssue(); issue != nil {
		return issue.ID
	}
	return ""
}

const (
	IntakeStatusPending   = -2
	IntakeStatusDeclined  = -1
	IntakeStatusSnoozed   = 0
	IntakeStatusAccepted  = 1
	IntakeStatusDuplicate = 2
)

// IntakeStatusName returns the stable, human-readable status name used by the
// MCP discovery tools.
func IntakeStatusName(status int) string {
	switch status {
	case IntakeStatusPending:
		return "pending"
	case IntakeStatusDeclined:
		return "declined"
	case IntakeStatusSnoozed:
		return "snoozed"
	case IntakeStatusAccepted:
		return "accepted"
	case IntakeStatusDuplicate:
		return "duplicate"
	default:
		return fmt.Sprintf("unknown(%d)", status)
	}
}

// ParseIntakeStatus accepts either a canonical status name or its Plane
// integer value for client-side status filtering.
func ParseIntakeStatus(input string) (int, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	switch input {
	case "pending":
		return IntakeStatusPending, nil
	case "declined", "rejected":
		return IntakeStatusDeclined, nil
	case "snoozed":
		return IntakeStatusSnoozed, nil
	case "accepted":
		return IntakeStatusAccepted, nil
	case "duplicate":
		return IntakeStatusDuplicate, nil
	}
	status, err := strconv.Atoi(input)
	if err == nil && status >= IntakeStatusPending && status <= IntakeStatusDuplicate {
		return status, nil
	}
	return 0, fmt.Errorf("invalid intake status %q: use pending, declined, snoozed, accepted, duplicate, or a Plane status value from -2 to 2", input)
}

// APIError represents a non-successful Plane API response.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("request failed with status %d: %s", e.StatusCode, e.Body)
}

// IsNotFoundError reports whether err represents an HTTP 404 response.
func IsNotFoundError(err error) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusNotFound
}

// TransitionNotAppliedError reports a 2xx PATCH response that did not apply
// the requested Intake transition. Plane returns HTTP 200 for permission
// no-ops and configuration failures instead of an error status, so callers
// must verify semantic state after every mutation. The error carries the
// observed status and likely causes.
type TransitionNotAppliedError struct {
	Identifier string   // project-prefixed work-item identifier (e.g. ASBX-10)
	IssueUUID  string   // underlying issue UUID used by the intake detail route
	Requested  string   // canonical status name that was requested
	Observed   string   // canonical status name observed after re-read
	Causes     []string // likely causes, most likely first
}

func (e *TransitionNotAppliedError) Error() string {
	return fmt.Sprintf(
		"transition_not_applied: intake record %s still has status %q after requesting %q (HTTP 2xx but Plane did not apply the transition); likely causes: %s",
		e.Identifier, e.Observed, e.Requested, strings.Join(e.Causes, "; "),
	)
}

// EnrichmentNotAppliedError reports a 2xx intake PATCH carrying nested issue
// enrichment fields that Plane did not store. The Intake serializer silently
// drops unsupported issue fields instead of rejecting them, so callers must
// re-read and compare; this error names every field that failed verification
// and flags unintended triage-status changes.
type EnrichmentNotAppliedError struct {
	Identifier    string   // project-prefixed work-item identifier (e.g. ASBX-10)
	IssueUUID     string   // underlying issue UUID used by the intake detail route
	IgnoredFields []string // requested fields absent from the re-read record (verified field names)
	StatusChanged bool     // true when the enrichment moved the record out of its prior status
	Causes        []string // likely causes, most likely first
}

func (e *EnrichmentNotAppliedError) Error() string {
	var unstored []string
	if len(e.IgnoredFields) > 0 {
		unstored = append(unstored, fmt.Sprintf("fields %s", strings.Join(e.IgnoredFields, ", ")))
	}
	if e.StatusChanged {
		unstored = append(unstored, "the triage status changed unexpectedly")
	}
	if len(unstored) == 0 {
		unstored = append(unstored, "requested values")
	}
	return fmt.Sprintf(
		"enrichment_not_applied: intake record %s does not reflect the requested update (HTTP 2xx but Plane did not store %s); likely causes: %s",
		e.Identifier, strings.Join(unstored, "; "), strings.Join(e.Causes, "; "),
	)
}

// CommentActorDetail represents the actor (user) that authored a comment.
type CommentActorDetail struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	FirstName   string `json:"first_name"`
	LastName    string `json:"last_name"`
}

// RelationItem represents a single related work item reference.
type RelationItem struct {
	ProjectID string `json:"project_id"`
	IssueID   string `json:"issue_id"`
}

// WorkItemRelations holds all relations for a work item, grouped by relation type.
type WorkItemRelations struct {
	Blocking     []RelationItem `json:"blocking"`
	BlockedBy    []RelationItem `json:"blocked_by"`
	Duplicate    []RelationItem `json:"duplicate"`
	RelatesTo    []RelationItem `json:"relates_to"`
	StartAfter   []RelationItem `json:"start_after"`
	StartBefore  []RelationItem `json:"start_before"`
	FinishAfter  []RelationItem `json:"finish_after"`
	FinishBefore []RelationItem `json:"finish_before"`
}

// Comment represents a comment on a work item.
type Comment struct {
	ID          string             `json:"id"`
	CreatedAt   string             `json:"created_at"`
	CommentHTML string             `json:"comment_html"`
	ActorDetail CommentActorDetail `json:"actor_detail"`
}

// Client to interact with Plane REST API
type Client struct {
	BaseURL              string
	APIKey               string
	WorkspaceSlug        string
	HTTPClient           *http.Client
	CFAccessClientID     string
	CFAccessClientSecret string

	// now is the clock used by retry-backoff computation. Overridable in tests.
	now func() time.Time
	// sleep is the wait primitive used between retry attempts. It must respect
	// context cancellation. Overridable in tests.
	sleep func(ctx context.Context, d time.Duration) error
	// jitter returns the jitter applied to the exponential backoff fallback.
	// Overridable in tests.
	jitter func() time.Duration
}

// NewClient initializes a client from configuration
func NewClient(cfg *config.Config) *Client {
	c := &Client{
		BaseURL:              strings.TrimSuffix(cfg.PlaneBaseURL, "/"),
		APIKey:               cfg.PlaneAPIKey,
		WorkspaceSlug:        cfg.PlaneWorkspaceSlug,
		HTTPClient:           &http.Client{Timeout: 30 * time.Second},
		CFAccessClientID:     cfg.CFAccessClientID,
		CFAccessClientSecret: cfg.CFAccessClientSecret,
	}
	c.now = time.Now
	c.jitter = func() time.Duration {
		return time.Duration(rand.Int63n(int64(defaultRetryJitterMax)))
	}
	c.sleep = c.sleepWithContext
	return c
}

// sleepWithContext pauses for d, aborting early when ctx is cancelled or
// reaches its deadline. Returning ctx.Err() lets the retry loop stop promptly
// when the caller has given up.
func (c *Client) sleepWithContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryDelay computes the delay before the next attempt. A valid Retry-After
// value (delay-seconds or HTTP-date) wins, capped at defaultRetryMaxDelay;
// otherwise bounded exponential backoff with jitter is used.
func (c *Client) retryDelay(retryAfter string, attempt int) time.Duration {
	if d, ok := parseRetryAfter(retryAfter, c.now()); ok {
		return d
	}
	base := defaultRetryBaseDelay << attempt
	if base > defaultRetryMaxDelay || base <= 0 {
		base = defaultRetryMaxDelay
	}
	if c.jitter != nil {
		base += c.jitter()
	}
	return base
}

// parseRetryAfter parses a Retry-After header in either delay-seconds or
// HTTP-date form, capped at defaultRetryMaxDelay. It returns ok=false when the
// header is empty or malformed so the caller can fall back to backoff.
func parseRetryAfter(header string, now time.Time) (time.Duration, bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, false
	}
	if secs, err := strconv.Atoi(header); err == nil {
		d := time.Duration(secs) * time.Second
		if d > defaultRetryMaxDelay {
			d = defaultRetryMaxDelay
		}
		return d, true
	}
	if t, err := http.ParseTime(header); err == nil {
		d := t.Sub(now)
		if d < 0 {
			d = 0
		}
		if d > defaultRetryMaxDelay {
			d = defaultRetryMaxDelay
		}
		return d, true
	}
	return 0, false
}

// request helper handles headers, method, URL, and JSON serialization/deserialization
func (c *Client) request(ctx context.Context, method, path string, queryParams map[string]string, body interface{}, responseVal interface{}) error {
	u, err := url.Parse(c.BaseURL + path)
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	if len(queryParams) > 0 {
		q := u.Query()
		for k, v := range queryParams {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}

	var reqBody []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = b
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, u.String(), bytes.NewReader(reqBody))
		if err != nil {
			return fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("X-API-Key", c.APIKey)
		req.Header.Set("Content-Type", "application/json")

		// Apply Cloudflare Access headers if configured
		if c.CFAccessClientID != "" && c.CFAccessClientSecret != "" {
			req.Header.Set("CF-Access-Client-Id", c.CFAccessClientID)
			req.Header.Set("CF-Access-Client-Secret", c.CFAccessClientSecret)
		}

		resp, err := c.HTTPClient.Do(req)
		if err != nil {
			return fmt.Errorf("request failed: %w", err)
		}

		if resp.StatusCode != http.StatusTooManyRequests {
			defer resp.Body.Close()

			if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				respBody, _ := io.ReadAll(resp.Body)
				return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
			}

			if responseVal != nil {
				respBody, err := io.ReadAll(resp.Body)
				if err != nil {
					return fmt.Errorf("failed to read response body: %w", err)
				}

				if err := json.Unmarshal(respBody, responseVal); err != nil {
					return fmt.Errorf("failed to unmarshal response: %w (body: %s)", err, string(respBody))
				}
			}

			return nil
		}

		// HTTP 429: the server explicitly asked us to retry. Retry within the
		// bounded policy; transport errors and 5xx responses are not retried.
		if attempt >= defaultRetryMaxAttempts-1 {
			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return &APIError{StatusCode: resp.StatusCode, Body: string(respBody)}
		}

		delay := c.retryDelay(resp.Header.Get("Retry-After"), attempt)
		resp.Body.Close()

		if err := c.sleep(ctx, delay); err != nil {
			return err
		}
	}
}

// parseListResponse handles parsing API responses that might be either raw arrays or paginated envelopes.
func parseListResponse[T any](data []byte) ([]T, string, bool, error) {
	// 1. Try raw array unmarshal
	var rawList []T
	if err := json.Unmarshal(data, &rawList); err == nil {
		return rawList, "", false, nil
	}

	// 2. Try paginated envelope unmarshal
	var paginated struct {
		Results         []T    `json:"results"`
		NextCursor      string `json:"next_cursor"`
		NextPageResults bool   `json:"next_page_results"`
	}
	if err := json.Unmarshal(data, &paginated); err == nil {
		return paginated.Results, paginated.NextCursor, paginated.NextPageResults, nil
	}

	return nil, "", false, fmt.Errorf("failed to parse response as list or paginated object (body: %s)", string(data))
}

const maxPaginationPages = 1000

// listAllGeneric handles auto-pagination for list endpoints.
func listAllGeneric[T any](ctx context.Context, c *Client, path string, queryParams map[string]string) ([]T, error) {
	var allResults []T
	cursor := ""
	seenCursors := make(map[string]struct{})
	pageCount := 0

	// Parse limit from query params, then remove it so it's not forwarded.
	limit := 0
	if limitStr, ok := queryParams["limit"]; ok {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
		delete(queryParams, "limit")
	}

	for {
		if pageCount >= maxPaginationPages {
			return nil, fmt.Errorf("pagination exceeded maximum page count of %d", maxPaginationPages)
		}
		pageCount++
		if cursor != "" {
			if _, seen := seenCursors[cursor]; seen {
				return nil, fmt.Errorf("repeated pagination cursor %q", cursor)
			}
			seenCursors[cursor] = struct{}{}
		}

		params := make(map[string]string)
		for k, v := range queryParams {
			params[k] = v
		}
		params["per_page"] = "100"
		if cursor != "" {
			params["cursor"] = cursor
		}

		// Read raw bytes to use parseListResponse
		var raw json.RawMessage
		err := c.request(ctx, "GET", path, params, nil, &raw)
		if err != nil {
			return nil, err
		}

		results, nextCursor, hasMore, err := parseListResponse[T](raw)
		if err != nil {
			return nil, err
		}

		allResults = append(allResults, results...)

		// Apply limit: if we've reached or exceeded it, slice and stop.
		if limit > 0 && len(allResults) >= limit {
			allResults = allResults[:limit]
			break
		}

		if !hasMore || nextCursor == "" {
			break
		}
		if _, seen := seenCursors[nextCursor]; seen {
			return nil, fmt.Errorf("repeated pagination cursor %q", nextCursor)
		}
		cursor = nextCursor
	}

	return allResults, nil
}

// ListProjects retrieves all projects in the workspace
func (c *Client) ListProjects(ctx context.Context) ([]Project, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/", c.WorkspaceSlug)
	return listAllGeneric[Project](ctx, c, path, nil)
}

// ListStates retrieves all states for a specific project
func (c *Client) ListStates(ctx context.Context, projectID string) ([]State, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/states/", c.WorkspaceSlug, projectID)
	return listAllGeneric[State](ctx, c, path, nil)
}

// ListLabels retrieves all labels for a specific project
func (c *Client) ListLabels(ctx context.Context, projectID string) ([]Label, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/labels/", c.WorkspaceSlug, projectID)
	return listAllGeneric[Label](ctx, c, path, nil)
}

// ListModules retrieves all modules for a specific project
func (c *Client) ListModules(ctx context.Context, projectID string) ([]Module, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/", c.WorkspaceSlug, projectID)
	return listAllGeneric[Module](ctx, c, path, nil)
}

// ListWorkspaceMembers retrieves all members in the workspace
func (c *Client) ListWorkspaceMembers(ctx context.Context) ([]Member, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/members/", c.WorkspaceSlug)
	return listAllGeneric[Member](ctx, c, path, nil)
}

// GetWorkItemByIdentifier retrieves a single work item using its project-prefixed sequence code (e.g. "PROJ-123")
func (c *Client) GetWorkItemByIdentifier(ctx context.Context, projectIdentifier string, sequenceID int) (*WorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/work-items/%s-%d/", c.WorkspaceSlug, projectIdentifier, sequenceID)
	var item WorkItem
	err := c.request(ctx, "GET", path, nil, nil, &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// GetMe returns the user record for the current API key's owner.
// Path: GET /api/v1/users/me/
func (c *Client) GetMe(ctx context.Context) (*Member, error) {
	path := "/api/v1/users/me/"
	var member Member
	err := c.request(ctx, "GET", path, nil, nil, &member)
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// ListWorkItems lists work items in a project with optional filter params.
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/work-items/
// The caller-provided params are forwarded as query params (e.g. assignees, state_group).
func (c *Client) ListWorkItems(ctx context.Context, projectID string, params map[string]string) ([]WorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/", c.WorkspaceSlug, projectID)
	return listAllGeneric[WorkItem](ctx, c, path, params)
}

// ListIntakeWorkItems retrieves all visible intake records for a project with
// the underlying work item expanded. Plane currently ignores the status query
// parameter on this endpoint, so status filtering belongs to the caller.
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/intake-issues/
func (c *Client) ListIntakeWorkItems(ctx context.Context, projectID string) ([]IntakeWorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/intake-issues/", c.WorkspaceSlug, projectID)
	return listAllGeneric[IntakeWorkItem](ctx, c, path, map[string]string{"expand": "issue"})
}

// GetIntakeWorkItem retrieves an intake record by the underlying work item
// UUID, not by the IntakeIssue record UUID.
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/intake-issues/{issueID}/
func (c *Client) GetIntakeWorkItem(ctx context.Context, projectID, issueID string) (*IntakeWorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/intake-issues/%s/", c.WorkspaceSlug, projectID, issueID)
	var item IntakeWorkItem
	if err := c.request(ctx, "GET", path, map[string]string{"expand": "issue"}, nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// TransitionIntakeWorkItem PATCHes an intake record by its underlying issue
// UUID. This is the only supported mutation route for Intake records; the
// /status/ subroute is invalid on the PAT API. Body fields follow the intake
// serializer (status, snoozed_till, duplicate_to). Callers must verify the
// resulting semantic state because insufficient project roles yield HTTP 200
// without applying the transition.
// Path: PATCH /api/v1/workspaces/{slug}/projects/{projectID}/intake-issues/{issueID}/
func (c *Client) TransitionIntakeWorkItem(ctx context.Context, projectID, issueID string, body map[string]any) (*IntakeWorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/intake-issues/%s/", c.WorkspaceSlug, projectID, issueID)
	var item IntakeWorkItem
	if err := c.request(ctx, "PATCH", path, map[string]string{"expand": "issue"}, body, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// CreateIntakeWorkItem submits a new idea directly into a project's Intake
// queue. Plane creates the underlying issue in the project's Triage state with
// an intake record (status -2 pending, source IN_APP) in one call; no separate
// state transition is needed. The body carries issue fields (name, priority,
// description_html) nested under "issue" per the intake serializer contract.
// Priority is validated server-side against none|low|medium|high|urgent.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/intake-issues/
func (c *Client) CreateIntakeWorkItem(ctx context.Context, projectID string, body map[string]any) (*IntakeWorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/intake-issues/", c.WorkspaceSlug, projectID)
	payload := map[string]any{"issue": body}
	var item IntakeWorkItem
	if err := c.request(ctx, "POST", path, map[string]string{"expand": "issue"}, payload, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// SearchWorkItems searches work items across the workspace.
// Path: GET /api/v1/workspaces/{slug}/work-items/search/
// The caller-provided params are forwarded as query params (e.g. search, project_id).
func (c *Client) SearchWorkItems(ctx context.Context, params map[string]string) ([]SearchWorkItemResult, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/work-items/search/", c.WorkspaceSlug)
	var resp struct {
		Issues []SearchWorkItemResult `json:"issues"`
	}
	if err := c.request(ctx, "GET", path, params, nil, &resp); err != nil {
		return nil, err
	}
	return resp.Issues, nil
}

// CreateWorkItem creates a new work item in a project.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/work-items/
func (c *Client) CreateWorkItem(ctx context.Context, projectID string, body map[string]any) (*WorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/", c.WorkspaceSlug, projectID)
	var item WorkItem
	err := c.request(ctx, "POST", path, nil, body, &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// UpdateWorkItem partially updates a work item via PATCH.
// Path: PATCH /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/
func (c *Client) UpdateWorkItem(ctx context.Context, projectID, workItemID string, body map[string]any) (*WorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/", c.WorkspaceSlug, projectID, workItemID)
	var item WorkItem
	err := c.request(ctx, "PATCH", path, nil, body, &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// IsHTMLTag checks whether trimmed text starts with a valid HTML tag, comment,
// doctype, or processing instruction. It is used to decide whether a comment
// body already contains HTML and should not be re-wrapped or Markdown-converted.
//
// It deliberately rejects non-tag uses of '<' such as "I <3 this" or "arrow <- here"
// so that those are still safely entity-escaped.
func IsHTMLTag(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "<") || len(trimmed) < 2 {
		return false
	}
	next := trimmed[1]
	return (next >= 'a' && next <= 'z') || (next >= 'A' && next <= 'Z') || next == '/' || next == '!' || next == '?'
}

// isHTMLTag is the unexported alias kept for internal use within this package.
func isHTMLTag(text string) bool { return IsHTMLTag(text) }

// CreateWorkItemComment posts a comment on a work item.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/comments/
// If text already looks like HTML (starts with '<' after trimming whitespace), it is used directly;
// otherwise it is wrapped in <p>...</p> for the comment_html field.
func (c *Client) CreateWorkItemComment(ctx context.Context, projectID, workItemID, text string) error {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/comments/", c.WorkspaceSlug, projectID, workItemID)
	commentHTML := text
	if !isHTMLTag(text) {
		commentHTML = "<p>" + text + "</p>"
	}
	body := map[string]any{
		"comment_html": commentHTML,
	}
	return c.request(ctx, "POST", path, nil, body, nil)
}

// CreateWorkItemLink attaches a URL to a work item.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/links/
func (c *Client) CreateWorkItemLink(ctx context.Context, projectID, workItemID, rawURL, title string) error {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/links/", c.WorkspaceSlug, projectID, workItemID)
	body := map[string]any{
		"url":   rawURL,
		"title": title,
	}
	return c.request(ctx, "POST", path, nil, body, nil)
}

// AddWorkItemsToModule associates one or more work items with a module.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/modules/{moduleID}/module-issues/
func (c *Client) AddWorkItemsToModule(ctx context.Context, projectID, moduleID string, workItemIDs []string) error {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/modules/%s/module-issues/", c.WorkspaceSlug, projectID, moduleID)
	body := map[string]any{
		"issues": workItemIDs,
	}
	return c.request(ctx, "POST", path, nil, body, nil)
}

// ListComments retrieves all comments for a work item.
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/comments/
func (c *Client) ListComments(ctx context.Context, projectID, workItemID string) ([]Comment, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/comments/", c.WorkspaceSlug, projectID, workItemID)
	return listAllGeneric[Comment](ctx, c, path, nil)
}

// GetWorkItem retrieves a single work item by project ID and work item ID (UUID).
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/
func (c *Client) GetWorkItem(ctx context.Context, projectID, workItemID string) (*WorkItem, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/", c.WorkspaceSlug, projectID, workItemID)
	var item WorkItem
	err := c.request(ctx, "GET", path, nil, nil, &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

// ListWorkItemRelations retrieves all relations for a work item.
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/relations/
func (c *Client) ListWorkItemRelations(ctx context.Context, projectID, workItemID string) (*WorkItemRelations, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/relations/", c.WorkspaceSlug, projectID, workItemID)
	var relations WorkItemRelations
	err := c.request(ctx, "GET", path, nil, nil, &relations)
	if err != nil {
		return nil, err
	}
	return &relations, nil
}

// CreateWorkItemRelation creates a relation between a work item and one or more other work items.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/relations/
func (c *Client) CreateWorkItemRelation(ctx context.Context, projectID, workItemID, relationType string, issues []string) error {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/relations/", c.WorkspaceSlug, projectID, workItemID)
	body := map[string]any{
		"relation_type": relationType,
		"issues":        issues,
	}
	return c.request(ctx, "POST", path, nil, body, nil)
}

// GetLastComment retrieves the single most recently created comment on a work item.
// Path: GET /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/comments/
// Returns nil if no comments exist.
//
// The order_by=-created_at query parameter is a silent no-op due to a Plane API
// server dispatch limitation: BaseAPIView.dispatch only copies URL-resolved kwargs
// (slug, project_id, issue_id) into self.kwargs; query-string parameters live in
// request.GET and are never merged. The server's get_queryset therefore always
// falls back to the default "-created_at" ordering. The parameter is left in place
// as belt-and-suspenders — correctness depends on that server default.
func (c *Client) GetLastComment(ctx context.Context, projectID, workItemID string) (*Comment, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/comments/", c.WorkspaceSlug, projectID, workItemID)
	params := map[string]string{
		"per_page": "1",
		"order_by": "-created_at",
	}

	var raw json.RawMessage
	err := c.request(ctx, "GET", path, params, nil, &raw)
	if err != nil {
		return nil, err
	}

	results, _, _, err := parseListResponse[Comment](raw)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	return &results[0], nil
}

// DeleteWorkItem deletes a work item.
// Path: DELETE /api/v1/workspaces/{slug}/projects/{projectID}/work-items/{workItemID}/
func (c *Client) DeleteWorkItem(ctx context.Context, projectID, workItemID string) error {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/work-items/%s/", c.WorkspaceSlug, projectID, workItemID)
	return c.request(ctx, "DELETE", path, nil, nil, nil)
}

// CreateLabel creates a new label definition in a project.
// Path: POST /api/v1/workspaces/{slug}/projects/{projectID}/labels/
func (c *Client) CreateLabel(ctx context.Context, projectID, name, color string) (*Label, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/labels/", c.WorkspaceSlug, projectID)
	body := map[string]any{"name": name}
	if color != "" {
		body["color"] = color
	}
	var label Label
	err := c.request(ctx, "POST", path, nil, body, &label)
	if err != nil {
		return nil, err
	}
	return &label, nil
}

// UpdateLabel updates a label definition (name and/or color) in a project.
// Path: PATCH /api/v1/workspaces/{slug}/projects/{projectID}/labels/{labelID}/
func (c *Client) UpdateLabel(ctx context.Context, projectID, labelID, name, color string) (*Label, error) {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/labels/%s/", c.WorkspaceSlug, projectID, labelID)
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if color != "" {
		body["color"] = color
	}
	var label Label
	err := c.request(ctx, "PATCH", path, nil, body, &label)
	if err != nil {
		return nil, err
	}
	return &label, nil
}

// DeleteLabel deletes a label definition from a project.
// Path: DELETE /api/v1/workspaces/{slug}/projects/{projectID}/labels/{labelID}/
func (c *Client) DeleteLabel(ctx context.Context, projectID, labelID string) error {
	path := fmt.Sprintf("/api/v1/workspaces/%s/projects/%s/labels/%s/", c.WorkspaceSlug, projectID, labelID)
	return c.request(ctx, "DELETE", path, nil, nil, nil)
}
