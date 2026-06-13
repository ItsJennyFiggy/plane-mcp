package plane

import (
	"context"
	"fmt"
	"strings"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"gopkg.in/yaml.v3"
)

// ConvertHTMLToMarkdown converts HTML to markdown, falling back to stripping HTML tags on error.
func ConvertHTMLToMarkdown(htmlStr string) string {
	if htmlStr == "" {
		return ""
	}
	markdown, err := htmltomarkdown.ConvertString(htmlStr)
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

// FormatWorkItemYAML serializes a work item to a compact YAML string based on the detail level.
func FormatWorkItemYAML(ctx context.Context, item *WorkItem, resolver *Resolver, detail string) (string, error) {
	resolved, err := resolver.ResolveWorkItem(ctx, item)
	if err != nil {
		return "", fmt.Errorf("failed to resolve work item: %w", err)
	}

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

	d, err := yaml.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to yaml: %w", err)
	}

	return string(d), nil
}

// FormatWorkItemsYAML serializes a slice of work items to a compact YAML string based on the detail level.
func FormatWorkItemsYAML(ctx context.Context, items []WorkItem, resolver *Resolver, detail string) (string, error) {
	var list []map[string]interface{}
	for i := range items {
		resolved, err := resolver.ResolveWorkItem(ctx, &items[i])
		if err != nil {
			return "", fmt.Errorf("failed to resolve work item at index %d: %w", i, err)
		}

		m := make(map[string]interface{})
		identifier := items[i].ID
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
			if items[i].Parent != nil && *items[i].Parent != "" {
				m["parent"] = *items[i].Parent
			}
			if items[i].EstimatePoint != nil {
				m["estimate_point"] = *items[i].EstimatePoint
			}
			if items[i].Type != nil && *items[i].Type != "" {
				m["type"] = *items[i].Type
			}
			if resolved.IsDraft {
				m["is_draft"] = resolved.IsDraft
			}
		}

		list = append(list, m)
	}

	d, err := yaml.Marshal(list)
	if err != nil {
		return "", fmt.Errorf("failed to marshal list to yaml: %w", err)
	}

	return string(d), nil
}
