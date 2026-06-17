package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"
)

// ---------------------------------------------------------------------------
// Interfaces — thin seams that allow tests to inject mock implementations.
// The production code uses *plane.Client and *plane.Resolver which must satisfy
// these interfaces (the methods are added by a parallel agent).
// ---------------------------------------------------------------------------

// planeClient abstracts all Plane API calls made by the tool handlers.
type planeClient interface {
	ListProjects(ctx context.Context) ([]plane.Project, error)
	GetWorkItemByIdentifier(ctx context.Context, projectIdentifier string, sequenceID int) (*plane.WorkItem, error)
	ListWorkItems(ctx context.Context, projectID string, params map[string]string) ([]plane.WorkItem, error)
	SearchWorkItems(ctx context.Context, params map[string]string) ([]plane.SearchWorkItemResult, error)
	CreateWorkItem(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error)
	CreateWorkItemComment(ctx context.Context, projectID, itemID, comment string) error
	UpdateWorkItem(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error)
	CreateWorkItemLink(ctx context.Context, projectID, itemID, linkURL, title string) error
	AddWorkItemsToModule(ctx context.Context, projectID, moduleID string, workItemIDs []string) error
	ListLabels(ctx context.Context, projectID string) ([]plane.Label, error)
	ListStates(ctx context.Context, projectID string) ([]plane.State, error)
	ListComments(ctx context.Context, projectID, workItemID string) ([]plane.Comment, error)
	GetLastComment(ctx context.Context, projectID, workItemID string) (*plane.Comment, error)
	GetWorkItem(ctx context.Context, projectID, workItemID string) (*plane.WorkItem, error)
	ListWorkItemRelations(ctx context.Context, projectID, workItemID string) (*plane.WorkItemRelations, error)
	CreateWorkItemRelation(ctx context.Context, projectID, workItemID, relationType string, issues []string) error
	RemoveWorkItemRelation(ctx context.Context, projectID, workItemID, relatedIssue string) error
	DeleteWorkItem(ctx context.Context, projectID, workItemID string) error
}

// planeResolver abstracts all name-resolution calls made by the tool handlers.
type planeResolver interface {
	GetCallerID(ctx context.Context) (string, error)
	ResolveProject(ctx context.Context, input string) (*plane.Project, error)
	ResolveState(ctx context.Context, projectID string, input string) (*plane.State, error)
	ResolveLabel(ctx context.Context, projectID string, input string) (*plane.Label, error)
	ResolveModule(ctx context.Context, projectID string, input string) (*plane.Module, error)
	ResolveMember(ctx context.Context, input string) (*plane.Member, error)
}

// planeFormatter abstracts work item formatting.
// This seam exists so tests can verify output without running a full resolver chain.
type planeFormatter interface {
	FormatWorkItemYAML(ctx context.Context, item *plane.WorkItem, detail string) (string, error)
	FormatWorkItemsYAML(ctx context.Context, items []plane.WorkItem, detail string) (string, error)
}

// resolverFormatter wraps a *plane.Resolver and delegates to the plane package formatters.
type resolverFormatter struct {
	resolver *plane.Resolver
}

func (f *resolverFormatter) FormatWorkItemYAML(ctx context.Context, item *plane.WorkItem, detail string) (string, error) {
	return plane.FormatWorkItemYAML(ctx, item, f.resolver, detail)
}

func (f *resolverFormatter) FormatWorkItemsYAML(ctx context.Context, items []plane.WorkItem, detail string) (string, error) {
	return plane.FormatWorkItemsYAML(ctx, items, f.resolver, detail)
}

// ---------------------------------------------------------------------------
// Helper utilities
// ---------------------------------------------------------------------------

// parseIdentifier splits "PROJ-123" into ("PROJ", 123).
// It splits on the LAST hyphen to support project identifiers like "MY-PROJ-123".
// Returns an error if the format is wrong or the sequence number is not a positive integer.
func parseIdentifier(id string) (string, int, error) {
	idx := strings.LastIndex(id, "-")
	if idx < 0 || idx == 0 || idx == len(id)-1 {
		return "", 0, fmt.Errorf("invalid identifier %q: expected format PROJECT-N", id)
	}

	projPart := id[:idx]
	seqPart := id[idx+1:]

	seqID, err := strconv.Atoi(seqPart)
	if err != nil {
		return "", 0, fmt.Errorf("invalid identifier %q: sequence number %q is not an integer", id, seqPart)
	}
	if seqID <= 0 {
		return "", 0, fmt.Errorf("invalid identifier %q: sequence number must be a positive integer, got %d", id, seqID)
	}

	return projPart, seqID, nil
}

// shouldRegister returns whether a tool with the given name should be registered,
// given the configured profile and explicit tool allowlist.
//
// If cfg.PlaneMCPTools is non-empty, return true only if name is in that list.
// Otherwise return true if cfg.PlaneMCPProfile matches any of allowedProfiles.
func shouldRegister(name string, allowedProfiles []string, cfg *config.Config) bool {
	if len(cfg.PlaneMCPTools) > 0 {
		for _, t := range cfg.PlaneMCPTools {
			if t == name {
				return true
			}
		}
		return false
	}

	for _, p := range allowedProfiles {
		if cfg.PlaneMCPProfile == p {
			return true
		}
	}
	return false
}

// toolError returns a CallToolResult representing a tool-level error.
// The error is returned as MCP tool content with IsError=true so the LLM can self-correct.
func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{
			&mcp.TextContent{Text: msg},
		},
	}
}

// toolText returns a successful CallToolResult with the given text.
func toolText(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

// getProjectID safely extracts the project ID from an Expandable[Project],
// preferring Val.ID if Val is present, falling back to the ID field.
func getProjectID(p plane.Expandable[plane.Project]) string {
	if p.Val != nil {
		return p.Val.ID
	}
	return p.ID
}

// Inline-span and list-item patterns used by the Markdown converter.
var (
	inlineCodeRe  = regexp.MustCompile("`([^`]+)`")
	boldRe        = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	italicRe      = regexp.MustCompile(`\*([^*]+)\*`)
	taskItemRe    = regexp.MustCompile(`^[-*]\s+\[([ xX])\]\s*(.*)$`)
	orderedItemRe = regexp.MustCompile(`^\d+\.\s+(.*)$`)
)

// escapeHTML escapes the minimal set of HTML-significant characters so user text
// cannot inject markup. Quotes/apostrophes are intentionally left intact to keep
// the converted output readable.
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

// convertInline escapes a span of text and applies inline Markdown emphasis:
// inline code, then bold, then italic (bold before italic so `**x**` is not
// mis-parsed as nested italics).
func convertInline(s string) string {
	s = escapeHTML(s)
	s = inlineCodeRe.ReplaceAllString(s, "<code>$1</code>")
	s = boldRe.ReplaceAllString(s, "<strong>$1</strong>")
	s = italicRe.ReplaceAllString(s, "<em>$1</em>")
	return s
}

// headingLevel returns the heading level (1–6) if the line is an ATX heading
// (`# ` … `###### `), or 0 otherwise. A space must follow the hashes.
func headingLevel(t string) int {
	n := 0
	for n < len(t) && t[n] == '#' {
		n++
	}
	if n >= 1 && n <= 6 && n < len(t) && t[n] == ' ' {
		return n
	}
	return 0
}

func isUnorderedItem(t string) bool {
	return strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ")
}

func isOrderedItem(t string) bool { return orderedItemRe.MatchString(t) }

func isTaskItem(t string) bool { return taskItemRe.MatchString(t) }

func isHorizontalRule(t string) bool {
	return t == "---" || t == "***" || t == "___"
}

// isBlockStart reports whether a trimmed line begins a non-paragraph block, used
// to terminate paragraph accumulation.
func isBlockStart(t string) bool {
	return headingLevel(t) > 0 ||
		strings.HasPrefix(t, "```") ||
		isHorizontalRule(t) ||
		strings.HasPrefix(t, ">") ||
		isOrderedItem(t) ||
		isTaskItem(t) ||
		isUnorderedItem(t)
}

// convertDescriptionToHTML converts a Markdown description into Plane-native
// editor HTML. It is a line-based block parser supporting headings, ordered /
// unordered / task lists, fenced code blocks, blockquotes, horizontal rules, and
// paragraphs, with inline bold/italic/code spans. Block types outside this set
// (tables, callouts, links, etc.) degrade to plain paragraphs.
func convertDescriptionToHTML(desc string) string {
	if desc == "" {
		return ""
	}

	lines := strings.Split(desc, "\n")
	var b strings.Builder
	i := 0

	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])

		switch {
		case trimmed == "":
			i++

		case strings.HasPrefix(trimmed, "```"):
			// Fenced code block — collect verbatim until the closing fence.
			// Trailing \r is stripped from each line so CRLF input does not
			// leak a carriage-return into the rendered <pre><code> block.
			i++
			var code []string
			for i < len(lines) && strings.TrimSpace(lines[i]) != "```" {
				code = append(code, strings.TrimRight(lines[i], "\r"))
				i++
			}
			if i < len(lines) {
				i++ // consume closing fence
			}
			b.WriteString("<pre><code>")
			b.WriteString(escapeHTML(strings.Join(code, "\n")))
			b.WriteString("</code></pre>")

		case isHorizontalRule(trimmed):
			b.WriteString("<hr>")
			i++

		case headingLevel(trimmed) > 0:
			level := headingLevel(trimmed)
			content := strings.TrimSpace(trimmed[level:])
			fmt.Fprintf(&b, `<h%d class="editor-heading-block">%s</h%d>`, level, convertInline(content), level)
			i++

		case strings.HasPrefix(trimmed, ">"):
			var quote []string
			for i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">") {
				q := strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")
				quote = append(quote, strings.TrimSpace(q))
				i++
			}
			b.WriteString("<blockquote><p>")
			b.WriteString(convertInline(strings.Join(quote, " ")))
			b.WriteString("</p></blockquote>")

		case isTaskItem(trimmed):
			b.WriteString(`<ul class="task-list">`)
			for i < len(lines) && isTaskItem(strings.TrimSpace(lines[i])) {
				m := taskItemRe.FindStringSubmatch(strings.TrimSpace(lines[i]))
				checked := "false"
				if m[1] == "x" || m[1] == "X" {
					checked = "true"
				}
				fmt.Fprintf(&b, `<li data-checked="%s">%s</li>`, checked, convertInline(strings.TrimSpace(m[2])))
				i++
			}
			b.WriteString("</ul>")

		case isUnorderedItem(trimmed):
			b.WriteString("<ul>")
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if !isUnorderedItem(t) || isTaskItem(t) {
					break
				}
				fmt.Fprintf(&b, "<li>%s</li>", convertInline(strings.TrimSpace(t[2:])))
				i++
			}
			b.WriteString("</ul>")

		case isOrderedItem(trimmed):
			b.WriteString("<ol>")
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if !isOrderedItem(t) {
					break
				}
				m := orderedItemRe.FindStringSubmatch(t)
				fmt.Fprintf(&b, "<li>%s</li>", convertInline(strings.TrimSpace(m[1])))
				i++
			}
			b.WriteString("</ol>")

		default:
			// Paragraph — accumulate consecutive plain lines (joined with a space)
			// until a blank line or the start of another block.
			var para []string
			for i < len(lines) {
				t := strings.TrimSpace(lines[i])
				if t == "" || isBlockStart(t) {
					break
				}
				para = append(para, t)
				i++
			}
			b.WriteString("<p>")
			b.WriteString(convertInline(strings.Join(para, " ")))
			b.WriteString("</p>")
		}
	}

	return b.String()
}

// ---------------------------------------------------------------------------
// Arg structs (exported for MCP SDK JSON unmarshaling)
// ---------------------------------------------------------------------------

// FindMyWorkArgs are the arguments for the find_my_work tool.
type FindMyWorkArgs struct {
	Project    *string `json:"project,omitempty"`
	StateGroup *string `json:"state_group,omitempty"`
}

// GetWorkItemArgs are the arguments for the get_work_item tool.
type GetWorkItemArgs struct {
	Identifier string         `json:"identifier"`
	Detail     FlexibleDetail `json:"detail"`
}

// ReportProgressArgs are the arguments for the report_progress tool.
type ReportProgressArgs struct {
	Identifier string  `json:"identifier"`
	Comment    string  `json:"comment"`
	State      *string `json:"state,omitempty"`
}

// SubmitForReviewArgs are the arguments for the submit_for_review tool.
type SubmitForReviewArgs struct {
	Identifier string `json:"identifier"`
	PRURL      string `json:"pr_url"`
	Comment    string `json:"comment"`
}

// AddCommentArgs are the arguments for the add_comment tool.
type AddCommentArgs struct {
	Identifier string `json:"identifier"`
	Body       string `json:"body"`
}

// SetRelationArgs are the arguments for the set_relation tool.
type SetRelationArgs struct {
	Identifier        string `json:"identifier"`
	RelationType      string `json:"relation_type"`
	RelatedIdentifier string `json:"related_identifier"`
}

// RemoveRelationArgs are the arguments for the remove_relation tool.
type RemoveRelationArgs struct {
	Identifier        string `json:"identifier"`
	RelatedIdentifier string `json:"related_identifier"`
}

// ListRelationsArgs are the arguments for the list_relations tool.
type ListRelationsArgs struct {
	Identifier string `json:"identifier"`
}

// FlexibleStringSlice is a []string that unmarshals from either a JSON array
// or a JSON string containing a JSON-encoded array (or a comma-separated
// list). This makes MCP tools robust against clients that serialise array
// arguments as strings.
type FlexibleStringSlice []string

// UnmarshalJSON implements json.Unmarshaler so FlexibleStringSlice accepts:
//   - a JSON array: ["a", "b"]
//   - a JSON string containing an array: "[\"a\", \"b\"]"
//   - a JSON string containing a comma-separated list: "a, b"
//   - null / empty string → empty slice
func (s *FlexibleStringSlice) UnmarshalJSON(data []byte) error {
	// 1. Trim whitespace and try as a JSON array first.
	d := bytes.TrimSpace(data)
	if len(d) > 0 && d[0] == '[' {
		var arr []string
		if err := json.Unmarshal(d, &arr); err != nil {
			return err
		}
		*s = FlexibleStringSlice(arr)
		return nil
	}

	// 2. Try as a JSON string.
	var raw string
	if err := json.Unmarshal(d, &raw); err != nil {
		// If it's not a valid JSON value at all, treat as empty.
		*s = nil
		return nil
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		*s = nil
		return nil
	}

	// 3. If the string starts with '[', treat it as a JSON-encoded array.
	if len(raw) > 0 && raw[0] == '[' {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err != nil {
			return err
		}
		*s = FlexibleStringSlice(arr)
		return nil
	}

	// 4. Otherwise, comma-split.
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	*s = FlexibleStringSlice(out)
	return nil
}

// FlexibleDetail is a string that unmarshals from either a JSON string or a
// JSON boolean, providing parsing fallbacks for the get_work_item detail
// parameter. Boolean true maps to "full", false maps to "summary".
// Unrecognized string values silently default to "summary".
type FlexibleDetail string

// Valid detail levels.
const (
	DetailSummary           FlexibleDetail = "summary"
	DetailFull              FlexibleDetail = "full"
	DetailSummaryWithLabels FlexibleDetail = "summary_with_labels"
)

// UnmarshalJSON implements json.Unmarshaler so FlexibleDetail accepts:
//   - a JSON boolean: true → "full", false → "summary"
//   - a JSON string: normalised (case-folded, trimmed) and mapped to a valid
//     detail level; "true" → "full", "false" → "summary", unrecognised → "summary"
//   - null / empty → "summary"
func (d *FlexibleDetail) UnmarshalJSON(data []byte) error {
	raw := bytes.TrimSpace(data)

	// JSON null or empty
	if len(raw) == 0 || string(raw) == "null" {
		*d = DetailSummary
		return nil
	}

	// Try boolean first
	if string(raw) == "true" {
		*d = DetailFull
		return nil
	}
	if string(raw) == "false" {
		*d = DetailSummary
		return nil
	}

	// Try string
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		// Not a valid JSON value — default to summary.
		*d = DetailSummary
		return nil
	}

	*d = normalizeDetail(s)
	return nil
}

// normalizeDetail folds a string detail value to a recognised constant.
func normalizeDetail(s string) FlexibleDetail {
	s = strings.TrimSpace(s)
	switch strings.ToLower(s) {
	case "full", "true":
		return DetailFull
	case "summary_with_labels", "summary-with-labels":
		return DetailSummaryWithLabels
	default:
		return DetailSummary
	}
}

// ListProjectsArgs are the arguments for the list_projects tool.
type ListProjectsArgs struct {
}

// ListLabelsArgs are the arguments for the list_labels tool.
type ListLabelsArgs struct {
	Project string `json:"project"`
}

// ListStatesArgs are the arguments for the list_states tool.
type ListStatesArgs struct {
	Project string `json:"project"`
}

// AddLabelArgs are the arguments for the add_label tool.
type AddLabelArgs struct {
	Identifier string `json:"identifier"`
	Label      string `json:"label"`
}

// RemoveLabelArgs are the arguments for the remove_label tool.
type RemoveLabelArgs struct {
	Identifier string `json:"identifier"`
	Label      string `json:"label"`
}

// ListCommentsArgs are the arguments for the list_comments tool.
type ListCommentsArgs struct {
	Identifier string `json:"identifier"`
}

// GetLastCommentArgs are the arguments for the get_last_comment tool.
type GetLastCommentArgs struct {
	Identifier string `json:"identifier"`
}

// AssignWorkItemArgs are the arguments for the assign_work_item tool.
type AssignWorkItemArgs struct {
	Identifier string              `json:"identifier"`
	Assignees  FlexibleStringSlice `json:"assignees"`
	Mode       string              `json:"mode,omitempty"`
}

// CreateTaskArgs are the arguments for the create_task tool.
type CreateTaskArgs struct {
	Project     string              `json:"project"`
	Name        string              `json:"name"`
	Description string              `json:"description,omitempty"`
	Priority    string              `json:"priority,omitempty"`
	Assignees   FlexibleStringSlice `json:"assignees,omitempty"`
	Labels      FlexibleStringSlice `json:"labels,omitempty"`
	Module      string              `json:"module,omitempty"`
	Parent      string              `json:"parent,omitempty"`
}

// ListWorkItemsArgs are the arguments for the list_work_items tool.
type ListWorkItemsArgs struct {
	Project    string              `json:"project"`
	StateGroup *string             `json:"state_group,omitempty"`
	Priority   *string             `json:"priority,omitempty"`
	Type       *string             `json:"type,omitempty"`
	Assignees  FlexibleStringSlice `json:"assignees,omitempty"`
	Labels     FlexibleStringSlice `json:"labels,omitempty"`
	State      *string             `json:"state,omitempty"`
	Module     *string             `json:"module,omitempty"`
	Limit      *int                `json:"limit,omitempty"`
}

// SearchWorkItemsArgs are the arguments for the search_work_items tool.
type SearchWorkItemsArgs struct {
	Query   string  `json:"query"`
	Project *string `json:"project,omitempty"`
	Limit   *int    `json:"limit,omitempty"`
}

// UpdateWorkItemArgs are the arguments for the update_work_item tool.
// All fields except Identifier are pointers so that nil means "omit from PATCH".
type UpdateWorkItemArgs struct {
	Identifier  string  `json:"identifier"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
	Priority    *string `json:"priority,omitempty"`
	State       *string `json:"state,omitempty"`
}

// SetParentArgs are the arguments for the set_parent tool.
type SetParentArgs struct {
	Identifier       string `json:"identifier"`
	ParentIdentifier string `json:"parent_identifier"`
}

// ClearParentArgs are the arguments for the clear_parent tool.
type ClearParentArgs struct {
	Identifier string `json:"identifier"`
}

// ListChildrenArgs are the arguments for the list_children tool.
type ListChildrenArgs struct {
	Identifier string `json:"identifier"`
}

// MoveWorkItemArgs are the arguments for the move_work_item tool.
type MoveWorkItemArgs struct {
	Identifier     string `json:"identifier"`
	TargetProject  string `json:"target_project"`
	DeleteOriginal bool   `json:"delete_original"`
}

// commentOut is the YAML-serializable representation of a single comment,
// shared by list_comments and get_last_comment. Field order is fixed to
// author, created_at, body — matching the expected output shape.
type commentOut struct {
	Author    string `yaml:"author"`
	CreatedAt string `yaml:"created_at"`
	Body      string `yaml:"body"`
}

// formatCommentAuthor returns the best available human-readable author label
// for a comment, applying the priority: DisplayName → FirstName+LastName → "Unknown".
func formatCommentAuthor(c plane.Comment) string {
	author := c.ActorDetail.DisplayName
	if author == "" {
		author = strings.TrimSpace(c.ActorDetail.FirstName + " " + c.ActorDetail.LastName)
	}
	if author == "" {
		author = "Unknown"
	}
	return author
}

// makeCommentOut converts a plane.Comment to the YAML-ready commentOut struct.
func makeCommentOut(c plane.Comment) commentOut {
	return commentOut{
		Author:    formatCommentAuthor(c),
		CreatedAt: c.CreatedAt,
		Body:      plane.ConvertHTMLToMarkdown(c.CommentHTML),
	}
}

// ---------------------------------------------------------------------------
// Internal tool handler implementations (accept interfaces for testability)
// ---------------------------------------------------------------------------

// findMyWork implements the find_my_work tool logic.
func findMyWork(ctx context.Context, args FindMyWorkArgs, client planeClient, resolver planeResolver, formatter planeFormatter) (*mcp.CallToolResult, error) {
	callerID, err := resolver.GetCallerID(ctx)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get caller identity: %v", err)), nil
	}

	params := map[string]string{
		"assignees": callerID,
	}
	if args.StateGroup != nil && *args.StateGroup != "" {
		params["state_group"] = *args.StateGroup
	}

	var items []plane.WorkItem

	if args.Project != nil && *args.Project != "" {
		proj, err := resolver.ResolveProject(ctx, *args.Project)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve project %q: %v", *args.Project, err)), nil
		}
		items, err = client.ListWorkItems(ctx, proj.ID, params)
		if err != nil {
			return toolError(fmt.Sprintf("failed to list work items: %v", err)), nil
		}
	} else {
		projects, err := client.ListProjects(ctx)
		if err != nil {
			return toolError(fmt.Sprintf("failed to list projects: %v", err)), nil
		}
		for _, proj := range projects {
			projectItems, err := client.ListWorkItems(ctx, proj.ID, params)
			if err != nil {
				log.Printf("warning: failed to list work items for project %s: %v", proj.ID, err)
				continue
			}
			items = append(items, projectItems...)
		}
	}

	if len(items) == 0 {
		return toolText("No work items found matching the criteria."), nil
	}

	yaml, err := formatter.FormatWorkItemsYAML(ctx, items, "summary")
	if err != nil {
		return toolError(fmt.Sprintf("failed to format work items: %v", err)), nil
	}

	return toolText(yaml), nil
}

// getWorkItem implements the get_work_item tool logic.
func getWorkItem(ctx context.Context, args GetWorkItemArgs, client planeClient, formatter planeFormatter) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	detail := args.Detail
	if detail == "" {
		detail = DetailSummary
	}

	yaml, err := formatter.FormatWorkItemYAML(ctx, item, string(detail))
	if err != nil {
		return toolError(fmt.Sprintf("failed to format work item: %v", err)), nil
	}

	return toolText(yaml), nil
}

// listComments implements the list_comments tool logic.
func listComments(ctx context.Context, args ListCommentsArgs, client planeClient) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)
	workItemID := item.ID

	comments, err := client.ListComments(ctx, projectID, workItemID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list comments: %v", err)), nil
	}

	if len(comments) == 0 {
		yamlBytes, _ := yaml.Marshal([]commentOut{})
		return toolText(string(yamlBytes)), nil
	}

	// Sort by CreatedAt ascending (parsed as time for robustness).
	sort.Slice(comments, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339, comments[i].CreatedAt)
		tj, errJ := time.Parse(time.RFC3339, comments[j].CreatedAt)
		if errI != nil || errJ != nil {
			// Fall back to lexicographic comparison on parse failure.
			return comments[i].CreatedAt < comments[j].CreatedAt
		}
		return ti.Before(tj)
	})

	out := make([]commentOut, len(comments))
	for i, c := range comments {
		out[i] = makeCommentOut(c)
	}

	yamlBytes, err := yaml.Marshal(out)
	if err != nil {
		return toolError(fmt.Sprintf("failed to marshal comments: %v", err)), nil
	}

	return toolText(string(yamlBytes)), nil
}

// getLastComment implements the get_last_comment tool logic.
func getLastComment(ctx context.Context, args GetLastCommentArgs, client planeClient) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	comment, err := client.GetLastComment(ctx, getProjectID(item.Project), item.ID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to fetch last comment for %s: %v", args.Identifier, err)), nil
	}

	if comment == nil {
		return toolText("null"), nil
	}

	// Serialize a single commentOut so the YAML field order (author, created_at,
	// body) matches list_comments.
	d, err := yaml.Marshal(makeCommentOut(*comment))
	if err != nil {
		return toolError(fmt.Sprintf("failed to marshal comment to yaml: %v", err)), nil
	}

	return toolText(string(d)), nil
}

// reportProgress implements the report_progress tool logic.
func reportProgress(ctx context.Context, args ReportProgressArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	if args.Comment != "" {
		if err := client.CreateWorkItemComment(ctx, projectID, item.ID, args.Comment); err != nil {
			return toolError(fmt.Sprintf("failed to post comment: %v", err)), nil
		}
	}

	if args.State != nil && *args.State != "" {
		state, err := resolver.ResolveState(ctx, projectID, *args.State)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve state %q: %v", *args.State, err)), nil
		}
		if _, err := client.UpdateWorkItem(ctx, projectID, item.ID, map[string]any{"state": state.ID}); err != nil {
			return toolError(fmt.Sprintf("failed to update work item state: %v", err)), nil
		}
		return toolText(fmt.Sprintf("Progress reported on %s; state updated to %s.", args.Identifier, state.Name)), nil
	}

	return toolText(fmt.Sprintf("Progress reported on %s.", args.Identifier)), nil
}

// addComment implements the add_comment tool logic.
func addComment(ctx context.Context, args AddCommentArgs, client planeClient) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	commentHTML := convertDescriptionToHTML(args.Body)
	if err := client.CreateWorkItemComment(ctx, projectID, item.ID, commentHTML); err != nil {
		return toolError(fmt.Sprintf("failed to add comment: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Comment added to %s.", args.Identifier)), nil
}

// submitForReview implements the submit_for_review tool logic.
func submitForReview(ctx context.Context, args SubmitForReviewArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	inReviewState, err := resolver.ResolveState(ctx, projectID, "In Review")
	if err != nil {
		return toolError(fmt.Sprintf(
			"failed to find 'In Review' state for project %s: %v — state names are workspace-specific; verify the state exists",
			projectID, err,
		)), nil
	}

	if err := client.CreateWorkItemLink(ctx, projectID, item.ID, args.PRURL, "PR"); err != nil {
		return toolError(fmt.Sprintf("failed to attach PR link: %v", err)), nil
	}

	comment := args.Comment
	if comment == "" {
		comment = "PR submitted for review: " + args.PRURL
	}

	if err := client.CreateWorkItemComment(ctx, projectID, item.ID, comment); err != nil {
		return toolError(fmt.Sprintf("failed to post comment: %v", err)), nil
	}

	if _, err := client.UpdateWorkItem(ctx, projectID, item.ID, map[string]any{"state": inReviewState.ID}); err != nil {
		return toolError(fmt.Sprintf("failed to update work item state to 'In Review': %v", err)), nil
	}

	return toolText(fmt.Sprintf(
		"Work item %s has been moved to 'In Review' and PR link attached: %s",
		args.Identifier, args.PRURL,
	)), nil
}

// validRelationTypes defines the accepted relation type strings.
var validRelationTypes = map[string]bool{
	"blocking":      true,
	"blocked_by":    true,
	"duplicate":     true,
	"relates_to":    true,
	"start_after":   true,
	"start_before":  true,
	"finish_after":  true,
	"finish_before": true,
}

// setRelation implements the set_relation tool logic.
func setRelation(ctx context.Context, args SetRelationArgs, client planeClient) (*mcp.CallToolResult, error) {
	if !validRelationTypes[args.RelationType] {
		return toolError(fmt.Sprintf("invalid relation_type %q: must be one of blocking, blocked_by, duplicate, relates_to, start_after, start_before, finish_after, finish_before", args.RelationType)), nil
	}

	// Resolve source work item
	srcProj, srcSeq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	srcItem, err := client.GetWorkItemByIdentifier(ctx, srcProj, srcSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	// Resolve related work item
	relProj, relSeq, err := parseIdentifier(args.RelatedIdentifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	relItem, err := client.GetWorkItemByIdentifier(ctx, relProj, relSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.RelatedIdentifier, err)), nil
	}

	err = client.CreateWorkItemRelation(ctx, getProjectID(srcItem.Project), srcItem.ID, args.RelationType, []string{relItem.ID})
	if err != nil {
		return toolError(fmt.Sprintf("failed to create relation: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Relation %s set: %s -> %s", args.RelationType, args.Identifier, args.RelatedIdentifier)), nil
}

// removeRelation implements the remove_relation tool logic.
func removeRelation(ctx context.Context, args RemoveRelationArgs, client planeClient) (*mcp.CallToolResult, error) {
	// Resolve source work item
	srcProj, srcSeq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	srcItem, err := client.GetWorkItemByIdentifier(ctx, srcProj, srcSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	// Resolve related work item
	relProj, relSeq, err := parseIdentifier(args.RelatedIdentifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	relItem, err := client.GetWorkItemByIdentifier(ctx, relProj, relSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.RelatedIdentifier, err)), nil
	}

	err = client.RemoveWorkItemRelation(ctx, getProjectID(srcItem.Project), srcItem.ID, relItem.ID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to remove relation: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Relation removed between %s and %s", args.Identifier, args.RelatedIdentifier)), nil
}

// listRelations implements the list_relations tool logic.
func listRelations(ctx context.Context, args ListRelationsArgs, client planeClient) (*mcp.CallToolResult, error) {
	// Resolve the work item
	srcProj, srcSeq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	srcItem, err := client.GetWorkItemByIdentifier(ctx, srcProj, srcSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	relations, err := client.ListWorkItemRelations(ctx, getProjectID(srcItem.Project), srcItem.ID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list relations: %v", err)), nil
	}
	if relations == nil {
		return toolError("relations data is missing or empty"), nil
	}

	// Build a project ID -> identifier map from all projects
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list projects: %v", err)), nil
	}
	projectIDToIdentifier := make(map[string]string)
	for _, p := range projects {
		projectIDToIdentifier[p.ID] = p.Identifier
	}

	// Relation types in display order
	type relGroup struct {
		Label string
		Items []plane.RelationItem
	}
	groups := []relGroup{
		{"blocking", relations.Blocking},
		{"blocked_by", relations.BlockedBy},
		{"duplicate", relations.Duplicate},
		{"relates_to", relations.RelatesTo},
		{"start_after", relations.StartAfter},
		{"start_before", relations.StartBefore},
		{"finish_after", relations.FinishAfter},
		{"finish_before", relations.FinishBefore},
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("relations for %s:\n", args.Identifier))

	for _, g := range groups {
		if len(g.Items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n", g.Label)
		for _, ri := range g.Items {
			// Fetch the related work item to get its name and sequence_id
			projID := ri.ProjectID
			if projID == "" {
				projID = getProjectID(srcItem.Project)
			}
			relatedItem, err := client.GetWorkItem(ctx, projID, ri.IssueID)
			if err != nil {
				// Fallback: just show the UUID
				fmt.Fprintf(&b, "  - identifier: %s\n    name: (unknown)\n", ri.IssueID)
				continue
			}
			identifier := projectIDToIdentifier[projID]
			if identifier == "" {
				identifier = projID
			}
			fmt.Fprintf(&b, "  - identifier: %s-%d\n    name: %s\n", identifier, relatedItem.SequenceID, relatedItem.Name)
		}
	}

	return toolText(strings.TrimSpace(b.String())), nil
}

// setParent implements the set_parent tool logic.
// It sets the parent of a work item to another work item.
func setParent(ctx context.Context, args SetParentArgs, client planeClient) (*mcp.CallToolResult, error) {
	// Resolve child work item
	childProj, childSeq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	childItem, err := client.GetWorkItemByIdentifier(ctx, childProj, childSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	// Resolve parent work item
	parentProj, parentSeq, err := parseIdentifier(args.ParentIdentifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	parentItem, err := client.GetWorkItemByIdentifier(ctx, parentProj, parentSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get parent work item %s: %v", args.ParentIdentifier, err)), nil
	}

	_, err = client.UpdateWorkItem(ctx, getProjectID(childItem.Project), childItem.ID, map[string]any{"parent": parentItem.ID})
	if err != nil {
		return toolError(fmt.Sprintf("failed to set parent: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Parent %s set for %s.", args.ParentIdentifier, args.Identifier)), nil
}

// clearParent implements the clear_parent tool logic.
// It removes the parent reference from a work item.
func clearParent(ctx context.Context, args ClearParentArgs, client planeClient) (*mcp.CallToolResult, error) {
	proj, seq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	item, err := client.GetWorkItemByIdentifier(ctx, proj, seq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	_, err = client.UpdateWorkItem(ctx, getProjectID(item.Project), item.ID, map[string]any{"parent": nil})
	if err != nil {
		return toolError(fmt.Sprintf("failed to clear parent: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Parent cleared for %s.", args.Identifier)), nil
}

// listChildren implements the list_children tool logic.
// It returns the children (sub-issues) of a work item as a YAML list.
func listChildren(ctx context.Context, args ListChildrenArgs, client planeClient) (*mcp.CallToolResult, error) {
	proj, seq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}
	parentItem, err := client.GetWorkItemByIdentifier(ctx, proj, seq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	// Build a project ID -> identifier map from all projects
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list projects: %v", err)), nil
	}
	projectIDToIdentifier := make(map[string]string)
	for _, p := range projects {
		projectIDToIdentifier[p.ID] = p.Identifier
	}

	params := map[string]string{"parent": parentItem.ID}
	allItems, err := client.ListWorkItems(ctx, getProjectID(parentItem.Project), params)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list children: %v", err)), nil
	}

	// Filter client-side: keep only items whose Parent UUID matches the resolved
	// parent. The Plane API accepts the parent query parameter as a hint but some
	// deployments return the full project list regardless; we enforce the filter
	// here so callers always get exactly the direct children of the given item.
	var children []plane.WorkItem
	for _, item := range allItems {
		if item.Parent != nil && *item.Parent == parentItem.ID {
			children = append(children, item)
		}
	}

	if len(children) == 0 {
		return toolText("[]"), nil
	}

	// Build YAML list of {identifier, name} for each child
	type childOut struct {
		Identifier string `yaml:"identifier"`
		Name       string `yaml:"name"`
	}

	out := make([]childOut, len(children))
	for i, c := range children {
		identifier := projectIDToIdentifier[getProjectID(c.Project)]
		if identifier == "" {
			identifier = getProjectID(c.Project)
		}
		out[i] = childOut{
			Identifier: fmt.Sprintf("%s-%d", identifier, c.SequenceID),
			Name:       c.Name,
		}
	}

	yamlBytes, err := yaml.Marshal(out)
	if err != nil {
		return toolError(fmt.Sprintf("failed to marshal children: %v", err)), nil
	}

	return toolText(string(yamlBytes)), nil
}

// moveWorkItem implements the move_work_item tool logic.
func moveWorkItem(ctx context.Context, args MoveWorkItemArgs, client planeClient, resolver planeResolver, formatter planeFormatter) (*mcp.CallToolResult, error) {
	// a. Parse the source identifier.
	srcProj, srcSeq, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	// b. Resolve the target project ID.
	targetProj, err := resolver.ResolveProject(ctx, args.TargetProject)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve target project %q: %v", args.TargetProject, err)), nil
	}
	targetProjectID := targetProj.ID

	// c. Fetch the original work item.
	srcItem, err := client.GetWorkItemByIdentifier(ctx, srcProj, srcSeq)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get source work item %s: %v", args.Identifier, err)), nil
	}
	srcProjectID := getProjectID(srcItem.Project)

	// d. Resolve labels from the source item in the target project by name.
	var labelIDs []string
	var warnings []string
	for _, l := range srcItem.Labels {
		var labelName string
		if l.Val != nil {
			labelName = l.Val.Name
		} else if l.ID != "" {
			labelName = l.ID // fallback: try ID as name
		}
		if labelName == "" {
			continue
		}
		label, err := resolver.ResolveLabel(ctx, targetProjectID, labelName)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("label %q not found in target project, skipping", labelName))
			continue
		}
		labelIDs = append(labelIDs, label.ID)
	}

	// e. Retrieve target project states and match the source state.
	targetStates, err := client.ListStates(ctx, targetProjectID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list target project states: %v", err)), nil
	}
	srcStates, err := client.ListStates(ctx, srcProjectID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list source project states: %v", err)), nil
	}

	// Determine the source state's name and group.
	var srcStateName, srcStateGroup string
	if srcItem.State.Val != nil {
		srcStateName = srcItem.State.Val.Name
		srcStateGroup = srcItem.State.Val.Group
	} else {
		// Look up by ID in source states.
		for _, s := range srcStates {
			if s.ID == srcItem.State.ID {
				srcStateName = s.Name
				srcStateGroup = s.Group
				break
			}
		}
	}

	// Match target state: exact name → group → default.
	var targetStateID string
	var targetStateName string

	// 1. Exact match by name (case-insensitive).
	for _, s := range targetStates {
		if strings.EqualFold(s.Name, srcStateName) {
			targetStateID = s.ID
			targetStateName = s.Name
			break
		}
	}

	// 2. Match by state group (case-insensitive).
	if targetStateID == "" && srcStateGroup != "" {
		for _, s := range targetStates {
			if strings.EqualFold(s.Group, srcStateGroup) {
				targetStateID = s.ID
				targetStateName = s.Name
				break
			}
		}
	}

	// 3. Fallback to default (first backlog/unstarted, or first available).
	if targetStateID == "" {
		for _, group := range []string{"backlog", "unstarted"} {
			for _, s := range targetStates {
				if strings.EqualFold(s.Group, group) {
					targetStateID = s.ID
					targetStateName = s.Name
					break
				}
			}
			if targetStateID != "" {
				break
			}
		}
	}
	if targetStateID == "" && len(targetStates) > 0 {
		targetStateID = targetStates[0].ID
		targetStateName = targetStates[0].Name
	}

	if targetStateID == "" {
		return toolError("target project has no states available"), nil
	}

	if targetStateName != srcStateName {
		warnings = append(warnings, fmt.Sprintf("target state %q differs from original state %q", targetStateName, srcStateName))
	}

	// f. Convert description HTML → Markdown → HTML for the target.
	var descHTML string
	if srcItem.DescriptionHTML != "" {
		md := plane.ConvertHTMLToMarkdown(srcItem.DescriptionHTML)
		descHTML = convertDescriptionToHTML(md)
	}

	// g. Create the target work item.
	body := map[string]any{
		"name":  srcItem.Name,
		"state": targetStateID,
	}
	if descHTML != "" {
		body["description_html"] = descHTML
	}
	if srcItem.Priority != "" {
		body["priority"] = srcItem.Priority
	}
	if len(labelIDs) > 0 {
		body["labels"] = labelIDs
	}
	if len(srcItem.Assignees) > 0 {
		body["assignees"] = extractAssigneeIDs(srcItem.Assignees)
	}
	if srcItem.Type != nil && *srcItem.Type != "" {
		body["type"] = *srcItem.Type
	}

	created, err := client.CreateWorkItem(ctx, targetProjectID, body)
	if err != nil {
		return toolError(fmt.Sprintf("failed to create target work item: %v", err)), nil
	}

	newIdentifier := fmt.Sprintf("%s-%d", targetProj.Identifier, created.SequenceID)

	// h. Handle the original work item.
	if args.DeleteOriginal {
		if err := client.DeleteWorkItem(ctx, srcProjectID, srcItem.ID); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to delete original work item: %v", err))
		}
	} else {
		// Rename original item.
		newName := fmt.Sprintf("MOVED TO %s - %s", newIdentifier, srcItem.Name)
		updateBody := map[string]any{
			"name": newName,
		}

		// Find cancelled state in source project.
		var cancelledStateID string
		for _, s := range srcStates {
			if strings.EqualFold(s.Group, "cancelled") {
				cancelledStateID = s.ID
				break
			}
		}
		if cancelledStateID != "" {
			updateBody["state"] = cancelledStateID
		} else {
			log.Printf("warning: no cancelled state found in source project, leaving state unchanged")
		}

		if _, err := client.UpdateWorkItem(ctx, srcProjectID, srcItem.ID, updateBody); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to update original work item: %v", err))
		}

		// Create a 'duplicate' relation on the original pointing to the new item.
		if err := client.CreateWorkItemRelation(ctx, srcProjectID, srcItem.ID, "duplicate", []string{created.ID}); err != nil {
			warnings = append(warnings, fmt.Sprintf("failed to create duplicate relation on original: %v", err))
		}
	}

	// Format the created work item for output.
	yamlOut, err := formatter.FormatWorkItemYAML(ctx, created, "full")
	if err != nil {
		return toolError(fmt.Sprintf("work item %s created but failed to format: %v", newIdentifier, err)), nil
	}

	resultText := fmt.Sprintf("Work item %s moved to %s in project %s.\n\n%s", args.Identifier, newIdentifier, targetProj.Name, yamlOut)
	if len(warnings) > 0 {
		resultText += "\nWarnings:\n"
		for _, w := range warnings {
			resultText += fmt.Sprintf("- %s\n", w)
		}
		resultText += "Please manually verify the original work item state and delete/link it if necessary.\n"
	}

	return toolText(resultText), nil
}

// createTask implements the create_task tool logic.
func createTask(ctx context.Context, args CreateTaskArgs, client planeClient, resolver planeResolver, formatter planeFormatter) (*mcp.CallToolResult, error) {
	proj, err := resolver.ResolveProject(ctx, args.Project)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve project %q: %v", args.Project, err)), nil
	}
	projectID := proj.ID

	// Resolve the module up front (fail fast) — unlike assignees/labels, an unresolved
	// module is a hard error: it is the explicit intent of the field, and we must not
	// create an orphaned task that silently lands in no module.
	var module *plane.Module
	if args.Module != "" {
		module, err = resolver.ResolveModule(ctx, projectID, args.Module)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve module %q: %v", args.Module, err)), nil
		}
	}

	// Resolve the parent work item (if any). The parent is identified by a
	// project-prefixed identifier like "EXEC-6". We parse it and fetch the
	// work item to obtain its UUID, which Plane's API expects in body["parent"].
	var parentItem *plane.WorkItem
	if args.Parent != "" {
		parentProjID, parentSeqID, err := parseIdentifier(args.Parent)
		if err != nil {
			return toolError(fmt.Sprintf("invalid parent identifier %q: %v", args.Parent, err)), nil
		}
		parentItem, err = client.GetWorkItemByIdentifier(ctx, parentProjID, parentSeqID)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve parent work item %q: %v", args.Parent, err)), nil
		}
	}

	// Resolve assignees — skip failures with a warning.
	var assigneeIDs []string
	for _, a := range args.Assignees {
		member, err := resolver.ResolveMember(ctx, a)
		if err != nil {
			log.Printf("warning: failed to resolve assignee %q, skipping: %v", a, err)
			continue
		}
		assigneeIDs = append(assigneeIDs, member.ID)
	}

	// Resolve labels — skip failures with a warning.
	var labelIDs []string
	for _, l := range args.Labels {
		label, err := resolver.ResolveLabel(ctx, projectID, l)
		if err != nil {
			log.Printf("warning: failed to resolve label %q, skipping: %v", l, err)
			continue
		}
		labelIDs = append(labelIDs, label.ID)
	}

	body := map[string]any{
		"name": args.Name,
	}

	if args.Description != "" {
		body["description_html"] = convertDescriptionToHTML(args.Description)
	}
	if args.Priority != "" {
		body["priority"] = args.Priority
	}
	if len(assigneeIDs) > 0 {
		body["assignees"] = assigneeIDs
	}
	if len(labelIDs) > 0 {
		body["labels"] = labelIDs
	}
	if parentItem != nil {
		body["parent"] = parentItem.ID
	}

	created, err := client.CreateWorkItem(ctx, projectID, body)
	if err != nil {
		return toolError(fmt.Sprintf("failed to create work item: %v", err)), nil
	}

	// Associate with the resolved module (if any). The work item already exists at this
	// point, so on failure we surface a clear error noting the item was created — the
	// agent should fix the module association rather than retry the create.
	if module != nil {
		if err := client.AddWorkItemsToModule(ctx, projectID, module.ID, []string{created.ID}); err != nil {
			return toolError(fmt.Sprintf(
				"work item %q (id %s) was created but could not be added to module %q: %v",
				created.Name, created.ID, module.Name, err,
			)), nil
		}
	}

	yaml, err := formatter.FormatWorkItemYAML(ctx, created, "full")
	if err != nil {
		return toolError(fmt.Sprintf("failed to format created work item: %v", err)), nil
	}

	return toolText(yaml), nil
}

// listProjects implements the list_projects tool logic.
func listProjects(ctx context.Context, client planeClient) (*mcp.CallToolResult, error) {
	projects, err := client.ListProjects(ctx)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list projects: %v", err)), nil
	}

	if len(projects) == 0 {
		return toolText("No projects found."), nil
	}

	var b strings.Builder
	for _, p := range projects {
		fmt.Fprintf(&b, "- identifier: %q\n  name: %q\n  id: %q\n", p.Identifier, p.Name, p.ID)
	}

	return toolText(b.String()), nil
}

// listLabels implements the list_labels tool logic.
func listLabels(ctx context.Context, args ListLabelsArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	proj, err := resolver.ResolveProject(ctx, args.Project)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve project %q: %v", args.Project, err)), nil
	}

	labels, err := client.ListLabels(ctx, proj.ID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list labels: %v", err)), nil
	}

	if len(labels) == 0 {
		return toolText("No labels found in this project."), nil
	}

	var b strings.Builder
	for _, lbl := range labels {
		fmt.Fprintf(&b, "- id: %q\n  name: %q\n  color: %q\n", lbl.ID, lbl.Name, lbl.Color)
	}

	return toolText(b.String()), nil
}

// listStates implements the list_states tool logic.
func listStates(ctx context.Context, args ListStatesArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	proj, err := resolver.ResolveProject(ctx, args.Project)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve project %q: %v", args.Project, err)), nil
	}

	states, err := client.ListStates(ctx, proj.ID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list states: %v", err)), nil
	}

	if len(states) == 0 {
		return toolText("No states found in this project."), nil
	}

	var b strings.Builder
	for _, s := range states {
		fmt.Fprintf(&b, "- id: %q\n  name: %q\n  group: %q\n", s.ID, s.Name, s.Group)
	}

	return toolText(b.String()), nil
}

// extractLabelIDs returns the ID strings from a slice of Expandable[Label],
// handling both expanded (Val != nil) and non-expanded (ID only) entries.
func extractLabelIDs(labels []plane.Expandable[plane.Label]) []string {
	ids := make([]string, 0, len(labels))
	for _, l := range labels {
		if l.Val != nil {
			ids = append(ids, l.Val.ID)
		} else if l.ID != "" {
			ids = append(ids, l.ID)
		}
	}
	return ids
}

// extractAssigneeIDs returns the ID strings from a slice of Expandable[Member],
// handling both expanded (Val != nil) and non-expanded (ID only) entries.
func extractAssigneeIDs(assignees []plane.Expandable[plane.Member]) []string {
	ids := make([]string, 0, len(assignees))
	for _, a := range assignees {
		if a.Val != nil {
			ids = append(ids, a.Val.ID)
		} else if a.ID != "" {
			ids = append(ids, a.ID)
		}
	}
	return ids
}

// addLabel implements the add_label tool logic.
//
// Non-atomicity note: because Plane has no atomic add/remove-label endpoint,
// this handler uses GET → mutate → PATCH-full-array. If two concurrent callers
// modify labels on the same work item, the later PATCH replaces the entire
// labels array and may silently overwrite the earlier change.
func addLabel(ctx context.Context, args AddLabelArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	label, err := resolver.ResolveLabel(ctx, projectID, args.Label)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve label %q: %v", args.Label, err)), nil
	}

	currentIDs := extractLabelIDs(item.Labels)
	if slices.Contains(currentIDs, label.ID) {
		return toolText(fmt.Sprintf("Label %q is already attached to %s — no-op.", label.Name, args.Identifier)), nil
	}

	newIDs := append(currentIDs, label.ID)
	if _, err := client.UpdateWorkItem(ctx, projectID, item.ID, map[string]any{"labels": newIDs}); err != nil {
		return toolError(fmt.Sprintf("failed to attach label: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Label %q attached to %s.", label.Name, args.Identifier)), nil
}

// removeLabel implements the remove_label tool logic.
func removeLabel(ctx context.Context, args RemoveLabelArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	label, err := resolver.ResolveLabel(ctx, projectID, args.Label)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve label %q: %v", args.Label, err)), nil
	}

	currentIDs := extractLabelIDs(item.Labels)
	if !slices.Contains(currentIDs, label.ID) {
		return toolText(fmt.Sprintf("Label %q is not attached to %s — no-op.", label.Name, args.Identifier)), nil
	}

	// Build a new slice excluding the target label.
	newIDs := make([]string, 0, len(currentIDs))
	for _, id := range currentIDs {
		if id != label.ID {
			newIDs = append(newIDs, id)
		}
	}
	if _, err := client.UpdateWorkItem(ctx, projectID, item.ID, map[string]any{"labels": newIDs}); err != nil {
		return toolError(fmt.Sprintf("failed to detach label: %v", err)), nil
	}

	return toolText(fmt.Sprintf("Label %q removed from %s.", label.Name, args.Identifier)), nil
}

// assignWorkItem implements the assign_work_item tool logic.
//
// Modes:
//   - "set" (default): replaces the entire assignees list with the resolved IDs.
//   - "add": appends resolved IDs to the current assignees list (idempotent — no duplicates).
//   - "remove": removes resolved IDs from the current assignees list.
//
// An empty assignees list with mode "set" clears all assignees.
//
// Non-atomicity note: because Plane has no atomic add/remove-assignee endpoint,
// this handler uses GET → mutate → PATCH-full-array. Concurrent callers may race.
func assignWorkItem(ctx context.Context, args AssignWorkItemArgs, client planeClient, resolver planeResolver) (*mcp.CallToolResult, error) {
	mode := args.Mode
	if mode == "" {
		mode = "set"
	}
	if mode != "set" && mode != "add" && mode != "remove" {
		return toolError(fmt.Sprintf("invalid mode %q: must be 'set', 'add', or 'remove'", mode)), nil
	}

	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	// Resolve all new assignees up front.
	var resolved []plane.Member
	for _, a := range args.Assignees {
		member, err := resolver.ResolveMember(ctx, a)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve assignee %q: %v", a, err)), nil
		}
		resolved = append(resolved, *member)
	}

	currentIDs := extractAssigneeIDs(item.Assignees)

	newIDs := make([]string, 0, len(currentIDs)+len(resolved))
	switch mode {
	case "set":
		for _, m := range resolved {
			newIDs = append(newIDs, m.ID)
		}
	case "add":
		// Start from current, append only new IDs.
		newIDs = append(newIDs, currentIDs...)
		for _, m := range resolved {
			if !slices.Contains(newIDs, m.ID) {
				newIDs = append(newIDs, m.ID)
			}
		}
	case "remove":
		// Build a set of IDs to remove.
		removeSet := make(map[string]struct{}, len(resolved))
		for _, m := range resolved {
			removeSet[m.ID] = struct{}{}
		}
		for _, id := range currentIDs {
			if _, ok := removeSet[id]; !ok {
				newIDs = append(newIDs, id)
			}
		}
	}

	if _, err := client.UpdateWorkItem(ctx, projectID, item.ID, map[string]any{"assignees": newIDs}); err != nil {
		return toolError(fmt.Sprintf("failed to update assignees: %v", err)), nil
	}

	if len(newIDs) == 0 {
		return toolText(fmt.Sprintf("Assignees cleared on %s.", args.Identifier)), nil
	}

	var names []string
	for _, m := range resolved {
		names = append(names, m.DisplayName)
	}
	switch mode {
	case "add":
		return toolText(fmt.Sprintf("Assignees %v added to %s.", names, args.Identifier)), nil
	case "remove":
		return toolText(fmt.Sprintf("Assignees %v removed from %s.", names, args.Identifier)), nil
	default:
		return toolText(fmt.Sprintf("Assignees %v set on %s.", names, args.Identifier)), nil
	}
}

// listWorkItems implements the list_work_items tool logic.
func listWorkItems(ctx context.Context, args ListWorkItemsArgs, client planeClient, resolver planeResolver, formatter planeFormatter) (*mcp.CallToolResult, error) {
	proj, err := resolver.ResolveProject(ctx, args.Project)
	if err != nil {
		return toolError(fmt.Sprintf("failed to resolve project %q: %v", args.Project, err)), nil
	}

	// Determine whether we need client-side state_group filtering.
	filterByStateGroup := args.StateGroup != nil && *args.StateGroup != ""

	params := map[string]string{}
	if args.Priority != nil && *args.Priority != "" {
		params["priority"] = *args.Priority
	}
	if args.Type != nil && *args.Type != "" {
		params["type"] = *args.Type
	}

	// Resolve state name to UUID.
	if args.State != nil && *args.State != "" {
		state, err := resolver.ResolveState(ctx, proj.ID, *args.State)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve state %q: %v", *args.State, err)), nil
		}
		params["state"] = state.ID
	}

	// Resolve module name to UUID.
	if args.Module != nil && *args.Module != "" {
		module, err := resolver.ResolveModule(ctx, proj.ID, *args.Module)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve module %q: %v", *args.Module, err)), nil
		}
		params["module"] = module.ID
	}

	// Resolve assignee names to UUIDs.
	if len(args.Assignees) > 0 {
		var ids []string
		for _, a := range args.Assignees {
			member, err := resolver.ResolveMember(ctx, a)
			if err != nil {
				return toolError(fmt.Sprintf("failed to resolve assignee %q: %v", a, err)), nil
			}
			ids = append(ids, member.ID)
		}
		params["assignees"] = strings.Join(ids, ",")
	}

	// Resolve label names to UUIDs.
	if len(args.Labels) > 0 {
		var ids []string
		for _, l := range args.Labels {
			label, err := resolver.ResolveLabel(ctx, proj.ID, l)
			if err != nil {
				return toolError(fmt.Sprintf("failed to resolve label %q: %v", l, err)), nil
			}
			ids = append(ids, label.ID)
		}
		params["labels"] = strings.Join(ids, ",")
	}

	// When filtering by state_group, do NOT forward state_group to the
	// API (the Plane API ignores it). We strip the user's limit parameter
	// to get all results for client-side filtering, but set a safety cap
	// of 1000 to prevent runaway requests.
	maxItems := 1000
	if filterByStateGroup {
		params["limit"] = strconv.Itoa(maxItems)
		if args.Limit != nil && *args.Limit > 0 {
			// Limit is applied AFTER client-side filtering (see below).
		}
	} else {
		if args.Limit != nil && *args.Limit > 0 {
			params["limit"] = strconv.Itoa(*args.Limit)
		}
	}

	items, err := client.ListWorkItems(ctx, proj.ID, params)
	if err != nil {
		return toolError(fmt.Sprintf("failed to list work items: %v", err)), nil
	}

	// Client-side state_group filtering.
	if filterByStateGroup {
		states, err := client.ListStates(ctx, proj.ID)
		if err != nil {
			return toolError(fmt.Sprintf("failed to list states for state_group filtering: %v", err)), nil
		}

		stateGroupByID := make(map[string]string, len(states))
		for _, s := range states {
			stateGroupByID[s.ID] = s.Group
		}

		var filtered []plane.WorkItem
		for _, item := range items {
			stateID := item.State.ID
			if item.State.Val != nil {
				stateID = item.State.Val.ID
			}
			group, ok := stateGroupByID[stateID]
			if ok && strings.EqualFold(group, *args.StateGroup) {
				filtered = append(filtered, item)
			}
		}

		// Apply limit after filtering.
		if args.Limit != nil && *args.Limit > 0 && len(filtered) > *args.Limit {
			filtered = filtered[:*args.Limit]
		}

		items = filtered
	}

	if len(items) == 0 {
		return toolText("[]"), nil
	}

	yaml, err := formatter.FormatWorkItemsYAML(ctx, items, "summary_with_labels")
	if err != nil {
		return toolError(fmt.Sprintf("failed to format work items: %v", err)), nil
	}

	return toolText(yaml), nil
}

// searchWorkItems implements the search_work_items tool logic.
func searchWorkItems(ctx context.Context, args SearchWorkItemsArgs, client planeClient, resolver planeResolver, formatter planeFormatter) (*mcp.CallToolResult, error) {
	if args.Query == "" {
		return toolError("query is required"), nil
	}

	params := map[string]string{
		"search": args.Query,
	}

	if args.Project != nil && *args.Project != "" {
		proj, err := resolver.ResolveProject(ctx, *args.Project)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve project %q: %v", *args.Project, err)), nil
		}
		params["project_id"] = proj.ID
	}

	// Default limit to 10; hard cap at 20.
	limit := 10
	if args.Limit != nil {
		if *args.Limit <= 0 {
			limit = 10
		} else if *args.Limit > 20 {
			limit = 20
		} else {
			limit = *args.Limit
		}
	}
	params["limit"] = strconv.Itoa(limit)

	results, err := client.SearchWorkItems(ctx, params)
	if err != nil {
		return toolError(fmt.Sprintf("failed to search work items: %v", err)), nil
	}

	if len(results) == 0 {
		return toolText("[]"), nil
	}

	var items []plane.WorkItem
	for _, r := range results {
		item, err := client.GetWorkItemByIdentifier(ctx, r.ProjectIdentifier, r.SequenceID)
		if err != nil {
			log.Printf("search_work_items: skipping %s-%d: %v", r.ProjectIdentifier, r.SequenceID, err)
			continue
		}
		items = append(items, *item)
	}

	if len(items) == 0 {
		return toolText("[]"), nil
	}

	yaml, err := formatter.FormatWorkItemsYAML(ctx, items, "summary")
	if err != nil {
		return toolError(fmt.Sprintf("failed to format work items: %v", err)), nil
	}

	return toolText(yaml), nil
}

// updateWorkItem implements the update_work_item tool logic.
// All optional fields use pointers: nil means "omit from PATCH", non-nil means "set to this value".
func updateWorkItem(ctx context.Context, args UpdateWorkItemArgs, client planeClient, resolver planeResolver, formatter planeFormatter) (*mcp.CallToolResult, error) {
	projIdentifier, seqID, err := parseIdentifier(args.Identifier)
	if err != nil {
		return toolError(err.Error()), nil
	}

	item, err := client.GetWorkItemByIdentifier(ctx, projIdentifier, seqID)
	if err != nil {
		return toolError(fmt.Sprintf("failed to get work item %s: %v", args.Identifier, err)), nil
	}

	projectID := getProjectID(item.Project)

	body := map[string]any{}

	if args.Name != nil {
		body["name"] = *args.Name
	}
	if args.Description != nil {
		body["description_html"] = convertDescriptionToHTML(*args.Description)
	}
	if args.Priority != nil {
		body["priority"] = *args.Priority
	}
	if args.State != nil {
		state, err := resolver.ResolveState(ctx, projectID, *args.State)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve state %q: %v", *args.State, err)), nil
		}
		body["state"] = state.ID
	}

	if len(body) == 0 {
		return toolError("at least one of name, description, priority, or state is required"), nil
	}

	updated, err := client.UpdateWorkItem(ctx, projectID, item.ID, body)
	if err != nil {
		return toolError(fmt.Sprintf("failed to update work item: %v", err)), nil
	}

	yaml, err := formatter.FormatWorkItemYAML(ctx, updated, "full")
	if err != nil {
		return toolError(fmt.Sprintf("failed to format updated work item: %v", err)), nil
	}

	return toolText(yaml), nil
}

// createTaskInputSchema builds the JSON Schema for the create_task tool.
// It overrides the FlexibleStringSlice type to accept "string" in addition
// to "null" and "array", so that MCP clients which serialise array
// arguments as JSON strings (e.g. "[\"uuid\"]") pass schema validation.
func createTaskInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[CreateTaskArgs](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[FlexibleStringSlice](): {
				Types: []string{"null", "array", "string"},
				Items: &jsonschema.Schema{Type: "string"},
			},
		},
	})
	if err != nil {
		// Should never happen for a well-known struct.
		panic(fmt.Sprintf("create_task: failed to build input schema: %v", err))
	}
	return schema
}

// assignWorkItemInputSchema builds the JSON Schema for the assign_work_item tool.
// It overrides the FlexibleStringSlice type to accept "string" in addition
// to "null" and "array", so that MCP clients which serialise array
// arguments as JSON strings (e.g. "[\"uuid\"]") pass schema validation.
// It also constrains mode to the three valid values.
func assignWorkItemInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[AssignWorkItemArgs](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[FlexibleStringSlice](): {
				Types: []string{"null", "array", "string"},
				Items: &jsonschema.Schema{Type: "string"},
			},
		},
	})
	if err != nil {
		panic(fmt.Sprintf("assign_work_item: failed to build input schema: %v", err))
	}
	// Constrain mode to the three valid values.
	for name, prop := range schema.Properties {
		if name == "mode" {
			prop.Enum = []any{"set", "add", "remove"}
		}
	}
	return schema
}

// listWorkItemsInputSchema builds the JSON Schema for the list_work_items tool.
// It overrides the FlexibleStringSlice type to accept "string" in addition
// to "null" and "array", so that MCP clients which serialise array
// arguments as JSON strings (e.g. "[\"uuid\"]") pass schema validation.
func listWorkItemsInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[ListWorkItemsArgs](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{
			reflect.TypeFor[FlexibleStringSlice](): {
				Types: []string{"null", "array", "string"},
				Items: &jsonschema.Schema{Type: "string"},
			},
		},
	})
	if err != nil {
		panic(fmt.Sprintf("list_work_items: failed to build input schema: %v", err))
	}
	return schema
}

// getWorkItemInputSchema builds the JSON Schema for the get_work_item tool.
// It constrains detail to the three valid string enum values.
// Note: the schema advertises only string enum values, but FlexibleDetail
// also accepts JSON booleans at parse time (true → "full", false →
// "summary") as a defensive fallback for clients that serialise boolean
// parameters.
func getWorkItemInputSchema() *jsonschema.Schema {
	schema, err := jsonschema.For[GetWorkItemArgs](nil)
	if err != nil {
		panic(fmt.Sprintf("get_work_item: failed to build input schema: %v", err))
	}
	// Constrain detail to the valid enum values.
	for name, prop := range schema.Properties {
		if name == "detail" {
			prop.Enum = []any{"summary", "full", "summary_with_labels"}
		}
	}
	return schema
}

// ---------------------------------------------------------------------------
// Register — wires up all five tools to the MCP server
// ---------------------------------------------------------------------------

// registerWithDeps is the testable core of Register that accepts interface types.
// Production code calls this via Register() in register.go.
func registerWithDeps(server *mcp.Server, client planeClient, resolver planeResolver, formatter planeFormatter, cfg *config.Config) {
	workerPlannerFull := []string{"worker", "planner", "full"}
	plannerFull := []string{"planner", "full"}

	falsePtr := false

	if shouldRegister("find_my_work", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "find_my_work",
			Description: "List all work items assigned to the current user, optionally filtered by project and state group.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args FindMyWorkArgs) (*mcp.CallToolResult, any, error) {
			result, err := findMyWork(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("list_projects", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_projects",
			Description: "List all projects, returning each project's identifier, name, and id.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListProjectsArgs) (*mcp.CallToolResult, any, error) {
			result, err := listProjects(ctx, client)
			return result, nil, err
		})
	}

	if shouldRegister("list_labels", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_labels",
			Description: "List all labels in a project, returning each label's id, name, and color.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListLabelsArgs) (*mcp.CallToolResult, any, error) {
			result, err := listLabels(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("list_states", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_states",
			Description: "List all states in a project, returning each state's id, name, and group.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListStatesArgs) (*mcp.CallToolResult, any, error) {
			result, err := listStates(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("add_label", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "add_label",
			Description: "Attach a label (by name or id) to a work item. Idempotent — returns success if the label is already attached.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args AddLabelArgs) (*mcp.CallToolResult, any, error) {
			result, err := addLabel(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("remove_label", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "remove_label",
			Description: "Detach a label (by name or id) from a work item. Idempotent — returns success if the label is already absent.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args RemoveLabelArgs) (*mcp.CallToolResult, any, error) {
			result, err := removeLabel(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("assign_work_item", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "assign_work_item",
			Description: "Set, add, or remove assignees on a work item by user name, display name, email, or ID. Mode 'set' replaces all assignees, 'add' appends, 'remove' removes. An empty assignees list with mode 'set' clears all assignees.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
			InputSchema: assignWorkItemInputSchema(),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args AssignWorkItemArgs) (*mcp.CallToolResult, any, error) {
			result, err := assignWorkItem(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("update_work_item", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "update_work_item",
			Description: "Update a work item's editable fields (name, description, priority, state) by its project-prefixed identifier (e.g. PROJ-123). Only the fields you provide are changed; omit any field to leave it unchanged. State is resolved by name or ID. Description accepts Markdown and is converted to Plane-native rich text.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args UpdateWorkItemArgs) (*mcp.CallToolResult, any, error) {
			result, err := updateWorkItem(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("get_work_item", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_work_item",
			Description: "Retrieve a single work item by its project-prefixed identifier (e.g. PROJ-123).",
			InputSchema: getWorkItemInputSchema(),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args GetWorkItemArgs) (*mcp.CallToolResult, any, error) {
			result, err := getWorkItem(ctx, args, client, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("report_progress", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "report_progress",
			Description: "Post a progress comment on a work item and optionally transition it to a new state.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ReportProgressArgs) (*mcp.CallToolResult, any, error) {
			result, err := reportProgress(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("add_comment", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "add_comment",
			Description: "Add a comment to a work item by its project-prefixed identifier (e.g. PROJ-123). The body accepts Markdown and is converted to Plane-native rich text.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: false},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args AddCommentArgs) (*mcp.CallToolResult, any, error) {
			result, err := addComment(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("submit_for_review", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "submit_for_review",
			Description: "Attach a PR link to a work item, post a comment, and move it to the 'In Review' state.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &falsePtr},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args SubmitForReviewArgs) (*mcp.CallToolResult, any, error) {
			result, err := submitForReview(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("create_task", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "create_task",
			Description: "Create a new work item in the specified project with optional description, priority, assignees, labels, and module. The description accepts Markdown (headings, lists, task lists, code blocks, blockquotes, emphasis) and is converted to Plane-native rich text. The module may be a module name or ID; if it cannot be resolved the task is not created.",
			InputSchema: createTaskInputSchema(),
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &falsePtr},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateTaskArgs) (*mcp.CallToolResult, any, error) {
			result, err := createTask(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("list_work_items", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_work_items",
			Description: "List work items in a project with optional filters for state group, state, priority, type, module, assignees, labels, and limit. Assignees, labels, states, and modules may be specified by name or ID and are resolved automatically.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
			InputSchema: listWorkItemsInputSchema(),
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListWorkItemsArgs) (*mcp.CallToolResult, any, error) {
			result, err := listWorkItems(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("search_work_items", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "search_work_items",
			Description: "Search work items across the workspace by a text query, with optional project filter and limit.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args SearchWorkItemsArgs) (*mcp.CallToolResult, any, error) {
			result, err := searchWorkItems(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("list_comments", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_comments",
			Description: "List all comments on a work item by its project-prefixed identifier (e.g. PROJ-123), sorted by creation time ascending. Returns YAML with author, created_at, and body (HTML converted to Markdown) for each comment.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListCommentsArgs) (*mcp.CallToolResult, any, error) {
			result, err := listComments(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("get_last_comment", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_last_comment",
			Description: "Retrieve the single most recently created comment on a work item by its project-prefixed identifier (e.g. PROJ-123). Returns YAML with author, created_at, and body (HTML converted to Markdown). If no comments exist, returns 'null'.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args GetLastCommentArgs) (*mcp.CallToolResult, any, error) {
			result, err := getLastComment(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("set_relation", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "set_relation",
			Description: "Create a relation between two work items by their project-prefixed identifiers (e.g. PROJ-123). Valid relation types: blocking, blocked_by, duplicate, relates_to, start_after, start_before, finish_after, finish_before.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args SetRelationArgs) (*mcp.CallToolResult, any, error) {
			result, err := setRelation(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("remove_relation", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "remove_relation",
			Description: "Remove a relation between two work items by their project-prefixed identifiers (e.g. PROJ-123).",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args RemoveRelationArgs) (*mcp.CallToolResult, any, error) {
			result, err := removeRelation(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("list_relations", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_relations",
			Description: "List all relations for a work item by its project-prefixed identifier (e.g. PROJ-123). Returns YAML grouped by relation type with resolved identifiers and names.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListRelationsArgs) (*mcp.CallToolResult, any, error) {
			result, err := listRelations(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("set_parent", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "set_parent",
			Description: "Set the parent of a work item by their project-prefixed identifiers (e.g. PROJ-123). The first identifier is the child, the second is the parent.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args SetParentArgs) (*mcp.CallToolResult, any, error) {
			result, err := setParent(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("clear_parent", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "clear_parent",
			Description: "Remove the parent reference from a work item by its project-prefixed identifier (e.g. PROJ-123).",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, IdempotentHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ClearParentArgs) (*mcp.CallToolResult, any, error) {
			result, err := clearParent(ctx, args, client)
			return result, nil, err
		})
	}

	if shouldRegister("list_children", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_children",
			Description: "List all child work items (sub-issues) for a work item by its project-prefixed identifier (e.g. PROJ-123). Returns YAML list with identifier and name for each child.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListChildrenArgs) (*mcp.CallToolResult, any, error) {
			result, err := listChildren(ctx, args, client)
			return result, nil, err
		})
	}

	truePtr := true
	if shouldRegister("move_work_item", plannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "move_work_item",
			Description: "Move a work item to another project by its project-prefixed identifier (e.g. PROJ-123). Creates a copy in the target project with the same name, description, priority, assignees, labels, and state (matched by name or state group). Optionally deletes the original; otherwise renames it with a MOVED TO prefix, transitions it to a cancelled state, and adds a duplicate relation pointing to the new item.",
			Annotations: &mcp.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: &truePtr},
		}, func(ctx context.Context, req *mcp.CallToolRequest, args MoveWorkItemArgs) (*mcp.CallToolResult, any, error) {
			result, err := moveWorkItem(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}
}
