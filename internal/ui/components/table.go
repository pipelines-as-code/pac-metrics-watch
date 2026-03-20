package components

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

type Column struct {
	Title      string
	Width      int
	AlignRight bool
}

type TableRow struct {
	Columns    []string
	Style      lipgloss.Style
	IsGroup    bool
	GroupTitle string
}

func RenderTable(columns []Column, rows []TableRow, cursor int) []string {
	var rendered []string

	headerParts := make([]string, len(columns))
	for i, col := range columns {
		if col.AlignRight {
			headerParts[i] = fmt.Sprintf("%*s", col.Width, trimForWidth(col.Title, col.Width))
		} else {
			headerParts[i] = fmt.Sprintf("%-*s", col.Width, trimForWidth(col.Title, col.Width))
		}
	}
	headerLine := theme.StyleTableHeader.Render(strings.Join(headerParts, " "))
	rendered = append(rendered, headerLine)

	for i, row := range rows {
		if row.IsGroup {
			rendered = append(rendered, theme.StyleSection.Render(row.GroupTitle))
			continue
		}

		rowParts := make([]string, len(columns))
		for j, colText := range row.Columns {
			width := columns[j].Width

			if j == 0 {
				prefix := "  "
				if i == cursor {
					prefix = "> "
				}
				text := trimForWidth(colText, width-2)
				rowParts[j] = fmt.Sprintf("%s%-*s", prefix, width-2, text)
			} else {
				text := trimForWidth(colText, width)
				if columns[j].AlignRight {
					rowParts[j] = fmt.Sprintf("%*s", width, text)
				} else {
					rowParts[j] = fmt.Sprintf("%-*s", width, text)
				}
			}
		}

		line := strings.Join(rowParts, " ")
		if i == cursor {
			rendered = append(rendered, theme.StyleTableCursor.Render(line))
		} else {
			rendered = append(rendered, row.Style.Render(line))
		}
	}

	return rendered
}

func trimForWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}
