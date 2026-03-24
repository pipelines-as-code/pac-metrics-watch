package views

import (
	"fmt"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/components"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

type ResourceFocus string

const (
	ResourceFocusMemory  ResourceFocus = "memory"
	ResourceFocusRuntime ResourceFocus = "runtime"
	ResourceFocusQueue   ResourceFocus = "queue"
)

type ResourceStat struct {
	Label     string
	Value     string
	Delta     string
	Available bool
}

type ContainerStat struct {
	PodName       string
	ContainerName string
	CPU           string
	Memory        string
}

type ComponentResources struct {
	Name               string
	PrimaryTitle       string
	PrimaryKind        string
	PrimaryValue       string
	PrimaryDelta       string
	PrimaryDescription string
	PrimarySources     string
	PrimaryHistory     []float64
	Stats              []ResourceStat
	Containers         []ContainerStat
}

func RenderResourcesView(focus ResourceFocus, componentsData []ComponentResources, warning string, width int) string {
	var sections []string
	sections = append(sections, theme.StyleChartTitle.Render("PAC Resource Diagnostics  focus="+string(focus)))
	if warning != "" {
		sections = append(sections, theme.StyleScope.Render("partial data: "+warning))
	}
	sections = append(sections, "")

	if len(componentsData) == 0 {
		sections = append(sections, theme.StyleDim.Render("No component diagnostics available yet."))
		return strings.Join(sections, "\n")
	}

	for _, component := range componentsData {
		sections = append(sections, components.RenderDetailPane(
			component.PrimaryTitle,
			component.PrimaryKind,
			component.PrimaryValue,
			component.PrimaryDelta,
			component.PrimaryDescription,
			component.PrimarySources,
			component.PrimaryHistory,
			width,
		))
		sections = append(sections, renderResourceStats(component, width))
	}

	return strings.Join(sections, "\n")
}

func renderResourceStats(component ComponentResources, width int) string {
	var lines []string
	lines = append(lines, theme.StyleChartTitle.Render(component.Name+" stats"))
	lines = append(lines, "")

	tab := new(tabwriter.Writer)
	tab.Init(&builderWriter{lines: &lines}, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tab, "METRIC\tVALUE\tDELTA")
	for _, stat := range component.Stats {
		value := "n/a"
		delta := "n/a"
		if stat.Available {
			value = stat.Value
			delta = stat.Delta
		}
		_, _ = fmt.Fprintf(tab, "%s\t%s\t%s\n", stat.Label, value, delta)
	}
	_ = tab.Flush()

	if len(component.Containers) > 0 {
		lines = append(lines, "", theme.StyleTableHeader.Render("TOP CONTAINERS"))
		for i, container := range component.Containers {
			if i >= 4 {
				break
			}
			lines = append(lines, fmt.Sprintf("  %-26s %-18s %8s %8s",
				TruncateStr(container.PodName, 26),
				TruncateStr(container.ContainerName, 18),
				container.CPU,
				container.Memory,
			))
		}
	} else {
		lines = append(lines, "", theme.StyleDim.Render("  no container usage available"))
	}

	return theme.StyleChartPane.Width(width - 4).Render(strings.Join(lines, "\n"))
}

func RenderResourcesSnapshot(scope string, collectedAt time.Time, focus ResourceFocus, componentsData []ComponentResources, warning, output string) string {
	var builder strings.Builder
	if output == "tsv" {
		fmt.Fprintf(&builder, "# scope=%s view=resources focus=%s\n", scope, focus)
		fmt.Fprintf(&builder, "# timestamp=%s\n", collectedAt.Format(time.RFC3339))
		builder.WriteString("component\tmetric\tvalue\tdelta\n")
		for _, component := range componentsData {
			fmt.Fprintf(&builder, "%s\t%s\t%s\t%s\n", component.Name, "primary", component.PrimaryValue, component.PrimaryDelta)
			for _, stat := range component.Stats {
				value := "n/a"
				delta := "n/a"
				if stat.Available {
					value = stat.Value
					delta = stat.Delta
				}
				fmt.Fprintf(&builder, "%s\t%s\t%s\t%s\n", component.Name, stat.Label, value, delta)
			}
		}
		return builder.String()
	}

	fmt.Fprintf(&builder, "scope:\t%s\n", scope)
	fmt.Fprintf(&builder, "view:\tresources\n")
	fmt.Fprintf(&builder, "focus:\t%s\n", focus)
	fmt.Fprintf(&builder, "timestamp:\t%s\n", collectedAt.Format(time.RFC3339))
	if warning != "" {
		fmt.Fprintf(&builder, "warning:\t%s\n", warning)
	}
	builder.WriteString("\n")

	tab := tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tab, "COMPONENT\tPRIMARY\tVALUE\tDELTA")
	for _, component := range componentsData {
		_, _ = fmt.Fprintf(tab, "%s\t%s\t%s\t%s\n", component.Name, component.PrimaryTitle, component.PrimaryValue, component.PrimaryDelta)
	}
	_ = tab.Flush()
	builder.WriteString("\n")

	for _, component := range componentsData {
		builder.WriteString(component.Name + "\n")
		builder.WriteString(strings.Repeat("-", len(component.Name)) + "\n")
		tab = tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
		_, _ = fmt.Fprintln(tab, "METRIC\tVALUE\tDELTA")
		for _, stat := range component.Stats {
			value := "n/a"
			delta := "n/a"
			if stat.Available {
				value = stat.Value
				delta = stat.Delta
			}
			_, _ = fmt.Fprintf(tab, "%s\t%s\t%s\n", stat.Label, value, delta)
		}
		_ = tab.Flush()
		builder.WriteString("\n")
	}

	return builder.String()
}

type builderWriter struct {
	lines *[]string
	buf   strings.Builder
}

func (w *builderWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	for {
		text := w.buf.String()
		idx := strings.IndexByte(text, '\n')
		if idx < 0 {
			break
		}
		*w.lines = append(*w.lines, strings.TrimRight(text[:idx], " "))
		w.buf.Reset()
		w.buf.WriteString(text[idx+1:])
	}
	return len(p), nil
}
