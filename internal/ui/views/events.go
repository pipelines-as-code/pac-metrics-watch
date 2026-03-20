package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/kubectl"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/components"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderEventsView(events []kubectl.K8sEvent, cursor, visibleStart, maxRows, width int, loading bool, errMsg string) ([]string, string) {
	if loading {
		return []string{theme.StyleDim.Render("  Loading events...")}, ""
	}

	if errMsg != "" {
		return []string{theme.StyleError.Render("  " + errMsg)}, ""
	}

	if len(events) == 0 {
		return []string{theme.StyleDim.Render("  No PAC-related events found.")}, ""
	}

	timeW := max(12, width/8)
	typeW := 9
	reasonW := max(16, width/6)
	objectW := max(20, width/5)
	msgW := max(30, width-timeW-typeW-reasonW-objectW-10)

	columns := []components.Column{
		{Title: "TIME", Width: timeW},
		{Title: "TYPE", Width: typeW},
		{Title: "REASON", Width: reasonW},
		{Title: "OBJECT", Width: objectW},
		{Title: "MESSAGE", Width: msgW},
	}

	var tableRows []components.TableRow
	for i, ev := range events {
		if i < visibleStart || i >= visibleStart+maxRows {
			continue
		}

		timeStr := RelativeTime(ev.Time)
		if ev.Time.IsZero() {
			timeStr = "unknown"
		}

		rowStyle := theme.StyleNormal
		if ev.Type == "Warning" {
			rowStyle = theme.StyleError
		}

		tableRows = append(tableRows, components.TableRow{
			Columns: []string{timeStr, ev.Type, ev.Reason, ev.Object, ev.Message},
			Style:   rowStyle,
		})
	}

	rendered := components.RenderTable(columns, tableRows, cursor-visibleStart)

	detail := ""
	if cursor >= 0 && cursor < len(events) {
		ev := events[cursor]
		detail = renderEventDetail(ev, width)
	}

	return rendered, detail
}

func renderEventDetail(ev kubectl.K8sEvent, width int) string {
	var lines []string
	lines = append(lines, theme.StyleChartTitle.Render(ev.Reason))
	lines = append(lines, "")

	timeStr := ev.Time.Format(time.RFC3339)
	if ev.Time.IsZero() {
		timeStr = "unknown"
	}

	typeStyle := theme.StyleNormal
	if ev.Type == "Warning" {
		typeStyle = theme.StyleError
	}

	lines = append(lines, fmt.Sprintf("  Type:    %s", typeStyle.Render(ev.Type)))
	lines = append(lines, fmt.Sprintf("  Object:  %s", ev.Object))
	lines = append(lines, fmt.Sprintf("  Time:    %s", timeStr))
	lines = append(lines, "")
	lines = append(lines, "  "+theme.StyleDetail.Render(ev.Message))

	return theme.StyleChartPane.Width(width - 4).Render(strings.Join(lines, "\n"))
}

func RenderEventsSnapshot(events []kubectl.K8sEvent) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%-20s %-9s %-20s %-30s %s\n", "TIME", "TYPE", "REASON", "OBJECT", "MESSAGE")
	builder.WriteString(strings.Repeat("-", 110) + "\n")

	for _, ev := range events {
		timeStr := ev.Time.Format(time.RFC3339)
		if ev.Time.IsZero() {
			timeStr = "unknown"
		}
		msg := ev.Message
		if len(msg) > 60 {
			msg = msg[:57] + "..."
		}
		fmt.Fprintf(&builder, "%-20s %-9s %-20s %-30s %s\n",
			timeStr, ev.Type, ev.Reason, ev.Object, msg)
	}

	return builder.String()
}
