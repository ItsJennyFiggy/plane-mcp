package plane

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
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
}

// NewClient initializes a client from configuration
func NewClient(cfg *config.Config) *Client {
	return &Client{
		BaseURL:              strings.TrimSuffix(cfg.PlaneBaseURL, "/"),
		APIKey:               cfg.PlaneAPIKey,
		WorkspaceSlug:        cfg.PlaneWorkspaceSlug,
		HTTPClient:           &http.Client{Timeout: 30 * time.Second},
		CFAccessClientID:     cfg.CFAccessClientID,
		CFAccessClientSecret: cfg.CFAccessClientSecret,
	}
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

	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
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
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(respBody))
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

// listAllGeneric handles auto-pagination for list endpoints
func listAllGeneric[T any](ctx context.Context, c *Client, path string, queryParams map[string]string) ([]T, error) {
	var allResults []T
	cursor := ""

	// Parse limit from query params, then remove it so it's not forwarded.
	limit := 0
	if limitStr, ok := queryParams["limit"]; ok {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
		delete(queryParams, "limit")
	}

	for {
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

// RemoveWorkItemRelation removes a relation between a work item and another work item.
//
// The Plane v1 API (/api/v1/) registers only GET and POST on the /relations/
// sub-path; there is no /relations/remove/ endpoint at the v1 level.
// The working endpoint lives under the app-level API and uses the legacy "issues"
// path segment:
//
//	POST /api/workspaces/{slug}/projects/{projectID}/issues/{workItemID}/remove-relation/
//
// Body: {"related_issue": "<related-work-item-uuid>"}
// The server performs a bidirectional lookup so argument order does not matter.
func (c *Client) RemoveWorkItemRelation(ctx context.Context, projectID, workItemID, relatedIssue string) error {
	path := fmt.Sprintf("/api/workspaces/%s/projects/%s/issues/%s/remove-relation/", c.WorkspaceSlug, projectID, workItemID)
	body := map[string]any{
		"related_issue": relatedIssue,
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
