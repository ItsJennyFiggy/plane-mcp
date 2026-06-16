package plane

import (
	"context"
	"fmt"
	stdhtml "html"
	"strings"
	"sync"

	"github.com/JohannesKaufmann/dom"
	"github.com/JohannesKaufmann/html-to-markdown/v2/converter"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/base"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/commonmark"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/strikethrough"
	"github.com/JohannesKaufmann/html-to-markdown/v2/plugin/table"

	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

// converterCache provides a lazily-initialized, concurrency-safe singleton for the
// HTML-to-Markdown converter instance. Building the converter (importing plugins) is
// moderately expensive, so we only do it once.
var (
	convOnce sync.Once
	conv     *converter.Converter
)

func getConverter() *converter.Converter {
	convOnce.Do(func() {
		c := converter.NewConverter(
			converter.WithPlugins(
				base.NewBasePlugin(),
				commonmark.NewCommonmarkPlugin(),
				strikethrough.NewStrikethroughPlugin(),
				table.NewTablePlugin(),
			),
		)
		// Register a custom renderer for Tiptap task lists.
		// Plane's rich text editor emits:
		//   <ul data-type="taskList"><li data-type="taskItem" data-checked="true|false">
		//     <label><input type="checkbox"><span></span></label>
		//     <div><p>…</p></div>
		//   </li></ul>
		// We use PriorityEarly so this runs before commonmark's list renderer.
		// We extract text inline via dom.CollectText (which skips the <label>/<input>
		// and flattens block boundaries) to produce valid GFM on a single line:
		//   - [x] Done task  (not - [x]\n\nDone task)
		c.Register.Renderer(func(ctx converter.Context, w converter.Writer, n *html.Node) converter.RenderStatus {
			if dom.NodeName(n) == "ul" {
				if attr, ok := dom.GetAttribute(n, "data-type"); ok && attr == "taskList" {
					for child := n.FirstChild; child != nil; child = child.NextSibling {
						if child.Type == html.ElementNode && dom.NodeName(child) == "li" {
							checked := false
							if val, ok := dom.GetAttribute(child, "data-checked"); ok && val == "true" {
								checked = true
							}
							w.WriteString("- [")
							if checked {
								w.WriteString("x")
							} else {
								w.WriteString(" ")
							}
							w.WriteString("] ")
							w.WriteString(strings.TrimSpace(dom.CollectText(child)))
							w.WriteString("\n")
						}
					}
					return converter.RenderSuccess
				}
			}
			return converter.RenderTryNext
		}, converter.PriorityEarly)
		conv = c
	})
	return conv
}

// looksDoublyEncoded reports whether s appears to be an entity-encoded HTML
// string that needs decoding. It returns true when s contains entity-encoded
// tag brackets (&lt; or &gt;) but no raw angle brackets (< or >).
func looksDoublyEncoded(s string) bool {
	hasEntityBracket := strings.Contains(s, "&lt;") || strings.Contains(s, "&gt;")
	hasRawBracket := strings.Contains(s, "<") || strings.Contains(s, ">")
	return hasEntityBracket && !hasRawBracket
}

// ConvertHTMLToMarkdown converts HTML to markdown using the configured plugins
// (table, strikethrough, tasklist), falling back to stripping HTML tags on error.
//
// When the input appears to be doubly-encoded — it contains entity-encoded tag
// brackets (&lt;, &gt;) but no raw angle brackets (<, >) — we decode entities
// first. This handles descriptions authored via Plane's rich-text UI or other
// MCP clients that store entity-encoded HTML.
//
// When the input already contains raw HTML tags we leave it as-is: those
// descriptions come from our own Markdown→HTML path (create_task), where
// entities represent literal characters (&lt; for < in inline code, etc.),
// not structural tags.
func ConvertHTMLToMarkdown(htmlStr string) string {
	if htmlStr == "" {
		return ""
	}
	// Only decode entities when the input looks doubly-encoded: it has
	// entity-encoded tag brackets (&lt;, &gt;) but no raw angle brackets.
	// This avoids corrupting legitimately-encoded content from
	// Markdown-authored descriptions (e.g. inline code with tags).
	if looksDoublyEncoded(htmlStr) {
		htmlStr = stdhtml.UnescapeString(htmlStr)
	}
	markdown, err := getConverter().ConvertString(htmlStr)
	if err != nil {
		return stripHTML(htmlStr)
	}
	return strings.TrimSpace(markdown)
}

// stripHTML is a safe fallback to strip HTML tags if the markdown converter fails.
func stripHTML(htmlStr string) string {
	var buf strings.Builder
	inTag := false
	for _, r := range htmlStr {
		if r == '<' {
			inTag = true
		} else if r == '>' {
			inTag = false
		} else if !inTag {
			buf.WriteRune(r)
		}
	}
	return strings.TrimSpace(buf.String())
}

// buildWorkItemMap extracts the fields of a work item into a serializable map based on detail level.
func buildWorkItemMap(item *WorkItem, resolved *ResolvedWorkItem, detail string) map[string]interface{} {
	m := make(map[string]interface{})

	identifier := resolved.ID
	if resolved.ProjectIdentifier != "" && resolved.SequenceID > 0 {
		identifier = fmt.Sprintf("%s-%d", resolved.ProjectIdentifier, resolved.SequenceID)
	}
	m["identifier"] = identifier
	m["name"] = resolved.Name

	if resolved.StateName != "" {
		m["state"] = resolved.StateName
	}
	if len(resolved.AssigneeNames) > 0 {
		m["assignees"] = resolved.AssigneeNames
	}
	if resolved.Priority != "" && resolved.Priority != "none" {
		m["priority"] = resolved.Priority
	}

	if strings.ToLower(detail) == "full" {
		var desc string
		if resolved.DescriptionHTML != "" {
			desc = ConvertHTMLToMarkdown(resolved.DescriptionHTML)
		} else if resolved.DescriptionStripped != "" {
			desc = resolved.DescriptionStripped
		}
		if desc != "" {
			m["description"] = desc
		}

		if len(resolved.LabelNames) > 0 {
			m["labels"] = resolved.LabelNames
		}
		if resolved.StartDate != "" {
			m["start_date"] = resolved.StartDate
		}
		if resolved.TargetDate != "" {
			m["target_date"] = resolved.TargetDate
		}
		if resolved.CompletedAt != "" {
			m["completed_at"] = resolved.CompletedAt
		}
		if resolved.ArchivedAt != "" {
			m["archived_at"] = resolved.ArchivedAt
		}
		if item.Parent != nil && *item.Parent != "" {
			m["parent"] = *item.Parent
		}
		if item.EstimatePoint != nil {
			m["estimate_point"] = *item.EstimatePoint
		}
		if item.Type != nil && *item.Type != "" {
			m["type"] = *item.Type
		}
		if resolved.IsDraft {
			m["is_draft"] = resolved.IsDraft
		}
	}

	if strings.ToLower(detail) == "full" || strings.ToLower(detail) == "summary_with_labels" {
		if len(resolved.LabelNames) > 0 {
			m["labels"] = resolved.LabelNames
		}
	}
	return m
}

// FormatWorkItemYAML serializes a work item to a compact YAML string based on the detail level.
// Unrecognized detail values silently default to summary mode.
func FormatWorkItemYAML(ctx context.Context, item *WorkItem, resolver *Resolver, detail string) (string, error) {
	resolved, err := resolver.ResolveWorkItem(ctx, item)
	if err != nil {
		return "", fmt.Errorf("failed to resolve work item: %w", err)
	}

	m := buildWorkItemMap(item, resolved, detail)

	d, err := yaml.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to yaml: %w", err)
	}

	return string(d), nil
}

// FormatWorkItemsYAML serializes a slice of work items to a compact YAML string based on the detail level.
// Unrecognized detail values silently default to summary mode.
func FormatWorkItemsYAML(ctx context.Context, items []WorkItem, resolver *Resolver, detail string) (string, error) {
	var list []map[string]interface{}
	for i := range items {
		resolved, err := resolver.ResolveWorkItem(ctx, &items[i])
		if err != nil {
			return "", fmt.Errorf("failed to resolve work item at index %d: %w", i, err)
		}

		m := buildWorkItemMap(&items[i], resolved, detail)
		list = append(list, m)
	}

	d, err := yaml.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("failed to marshal list to yaml: %w", err)
	}

	return string(d), nil
}
