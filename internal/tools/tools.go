package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/ItsJennyFiggy/plane-mcp/internal/config"
	"github.com/ItsJennyFiggy/plane-mcp/internal/plane"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	CreateWorkItem(ctx context.Context, projectID string, body map[string]any) (*plane.WorkItem, error)
	CreateWorkItemComment(ctx context.Context, projectID, itemID, comment string) error
	UpdateWorkItem(ctx context.Context, projectID, itemID string, body map[string]any) (*plane.WorkItem, error)
	CreateWorkItemLink(ctx context.Context, projectID, itemID, linkURL, title string) error
	AddWorkItemsToModule(ctx context.Context, projectID, moduleID string, workItemIDs []string) error
	ListLabels(ctx context.Context, projectID string) ([]plane.Label, error)
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
	Project    string `json:"project"`
	StateGroup string `json:"state_group"`
}

// GetWorkItemArgs are the arguments for the get_work_item tool.
type GetWorkItemArgs struct {
	Identifier string `json:"identifier"`
	Detail     string `json:"detail"`
}

// ReportProgressArgs are the arguments for the report_progress tool.
type ReportProgressArgs struct {
	Identifier string `json:"identifier"`
	Comment    string `json:"comment"`
	State      string `json:"state"`
}

// SubmitForReviewArgs are the arguments for the submit_for_review tool.
type SubmitForReviewArgs struct {
	Identifier string `json:"identifier"`
	PRURL      string `json:"pr_url"`
	Comment    string `json:"comment"`
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

// ListLabelsArgs are the arguments for the list_labels tool.
type ListLabelsArgs struct {
	Project string `json:"project"`
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
	if args.StateGroup != "" {
		params["state_group"] = args.StateGroup
	}

	var items []plane.WorkItem

	if args.Project != "" {
		proj, err := resolver.ResolveProject(ctx, args.Project)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve project %q: %v", args.Project, err)), nil
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
		detail = "summary"
	}

	yaml, err := formatter.FormatWorkItemYAML(ctx, item, detail)
	if err != nil {
		return toolError(fmt.Sprintf("failed to format work item: %v", err)), nil
	}

	return toolText(yaml), nil
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

	projectID := item.Project.ID

	if args.Comment != "" {
		if err := client.CreateWorkItemComment(ctx, projectID, item.ID, args.Comment); err != nil {
			return toolError(fmt.Sprintf("failed to post comment: %v", err)), nil
		}
	}

	if args.State != "" {
		state, err := resolver.ResolveState(ctx, projectID, args.State)
		if err != nil {
			return toolError(fmt.Sprintf("failed to resolve state %q: %v", args.State, err)), nil
		}
		if _, err := client.UpdateWorkItem(ctx, projectID, item.ID, map[string]any{"state": state.ID}); err != nil {
			return toolError(fmt.Sprintf("failed to update work item state: %v", err)), nil
		}
		return toolText(fmt.Sprintf("Progress reported on %s; state updated to %s.", args.Identifier, state.Name)), nil
	}

	return toolText(fmt.Sprintf("Progress reported on %s.", args.Identifier)), nil
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

	projectID := item.Project.ID

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
		fmt.Fprintf(&b, "- name: %s\n  color: %s\n", lbl.Name, lbl.Color)
	}

	return toolText(b.String()), nil
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

// ---------------------------------------------------------------------------
// Register — wires up all five tools to the MCP server
// ---------------------------------------------------------------------------

// registerWithDeps is the testable core of Register that accepts interface types.
// Production code calls this via Register() in register.go.
func registerWithDeps(server *mcp.Server, client planeClient, resolver planeResolver, formatter planeFormatter, cfg *config.Config) {
	workerPlannerFull := []string{"worker", "planner", "full"}
	plannerFull := []string{"planner", "full"}

	if shouldRegister("find_my_work", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "find_my_work",
			Description: "List all work items assigned to the current user, optionally filtered by project and state group.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args FindMyWorkArgs) (*mcp.CallToolResult, any, error) {
			result, err := findMyWork(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("list_labels", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "list_labels",
			Description: "List all labels in a project, returning each label's name and color.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ListLabelsArgs) (*mcp.CallToolResult, any, error) {
			result, err := listLabels(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("get_work_item", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "get_work_item",
			Description: "Retrieve a single work item by its project-prefixed identifier (e.g. PROJ-123).",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args GetWorkItemArgs) (*mcp.CallToolResult, any, error) {
			result, err := getWorkItem(ctx, args, client, formatter)
			return result, nil, err
		})
	}

	if shouldRegister("report_progress", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "report_progress",
			Description: "Post a progress comment on a work item and optionally transition it to a new state.",
		}, func(ctx context.Context, req *mcp.CallToolRequest, args ReportProgressArgs) (*mcp.CallToolResult, any, error) {
			result, err := reportProgress(ctx, args, client, resolver)
			return result, nil, err
		})
	}

	if shouldRegister("submit_for_review", workerPlannerFull, cfg) {
		mcp.AddTool(server, &mcp.Tool{
			Name:        "submit_for_review",
			Description: "Attach a PR link to a work item, post a comment, and move it to the 'In Review' state.",
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
		}, func(ctx context.Context, req *mcp.CallToolRequest, args CreateTaskArgs) (*mcp.CallToolResult, any, error) {
			result, err := createTask(ctx, args, client, resolver, formatter)
			return result, nil, err
		})
	}
}
