package views

import (
	"fmt"
	"strings"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

type HealthCheck struct {
	Name   string
	Status string // "pass", "fail", "warn"
	Detail string
}

func RenderHealthView(checks []HealthCheck, loading bool, width int) string {
	if loading {
		return theme.StyleDim.Render("  Running health checks...")
	}

	if len(checks) == 0 {
		return theme.StyleDim.Render("  No health checks available. Press 'h' to run checks.")
	}

	var lines []string
	lines = append(lines, theme.StyleChartTitle.Render("  PAC Installation Health"))
	lines = append(lines, "")

	for _, check := range checks {
		var indicator string
		var style = theme.StyleNormal
		switch check.Status {
		case "pass":
			indicator = theme.StyleDotGreen.Render("  ✓")
			style = theme.StyleIncr
		case "fail":
			indicator = theme.StyleDotRed.Render("  ✗")
			style = theme.StyleError
		case "warn":
			indicator = theme.StyleScope.Render("  !")
			style = theme.StyleScope
		default:
			indicator = theme.StyleDotDim.Render("  ?")
			style = theme.StyleDim
		}

		line := fmt.Sprintf("%s  %s", indicator, style.Render(check.Name))
		if check.Detail != "" {
			line += "  " + theme.StyleDim.Render(check.Detail)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func RenderHealthSnapshot(checks []HealthCheck) string {
	var builder strings.Builder
	builder.WriteString("PAC Installation Health\n")
	builder.WriteString(strings.Repeat("=", 40) + "\n\n")

	for _, check := range checks {
		var indicator string
		switch check.Status {
		case "pass":
			indicator = "[PASS]"
		case "fail":
			indicator = "[FAIL]"
		case "warn":
			indicator = "[WARN]"
		default:
			indicator = "[????]"
		}
		fmt.Fprintf(&builder, "%-6s  %s", indicator, check.Name)
		if check.Detail != "" {
			builder.WriteString("  -- " + check.Detail)
		}
		builder.WriteString("\n")
	}

	return builder.String()
}
