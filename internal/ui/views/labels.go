package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/metrics"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderLabelBreakdown(family *metrics.MetricFamily, width int) string {
	if family == nil || len(family.Samples) == 0 {
		return theme.StyleDim.Render("No label data available")
	}

	// Find the most discriminating label (most unique values)
	labelValues := map[string]map[string]bool{}
	for _, sample := range family.Samples {
		for k, v := range sample.Labels {
			if labelValues[k] == nil {
				labelValues[k] = map[string]bool{}
			}
			labelValues[k][v] = true
		}
	}

	bestLabel := ""
	bestCount := 0
	for label, values := range labelValues {
		if len(values) > bestCount {
			bestCount = len(values)
			bestLabel = label
		}
	}

	var lines []string
	lines = append(lines, theme.StyleChartTitle.Render(family.Name+" — label breakdown"))
	lines = append(lines, "")

	if bestLabel == "" {
		lines = append(lines, fmt.Sprintf("  Total: %s", metrics.FormatMetricNumber(family.Total)))
		lines = append(lines, theme.StyleDim.Render("  No labels found on this metric"))
		return theme.StyleChartPane.Width(width - 4).Render(strings.Join(lines, "\n"))
	}

	lines = append(lines, theme.StyleDetail.Render(fmt.Sprintf("  Grouped by: %s (%d values)", bestLabel, bestCount)))
	lines = append(lines, "")

	// Aggregate by bestLabel
	type labelSum struct {
		value string
		total float64
	}
	sums := map[string]float64{}
	for _, sample := range family.Samples {
		key := sample.Labels[bestLabel]
		if key == "" {
			key = "<none>"
		}
		sums[key] += sample.Value
	}

	sorted := make([]labelSum, 0, len(sums))
	for v, total := range sums {
		sorted = append(sorted, labelSum{value: v, total: total})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].total > sorted[j].total
	})

	// Find max for bar scaling
	maxVal := 0.0
	for _, s := range sorted {
		if s.total > maxVal {
			maxVal = s.total
		}
	}

	barMax := width - 40
	if barMax < 10 {
		barMax = 10
	}

	lines = append(lines, theme.StyleTableHeader.Render(fmt.Sprintf("  %-20s %12s  %s", bestLabel, "VALUE", "BAR")))

	for _, s := range sorted {
		barLen := 0
		if maxVal > 0 {
			barLen = int(s.total / maxVal * float64(barMax))
		}
		if barLen < 0 {
			barLen = 0
		}
		bar := strings.Repeat("█", barLen)
		lines = append(lines, fmt.Sprintf("  %-20s %12s  %s",
			TruncateStr(s.value, 20),
			metrics.FormatMetricNumber(s.total),
			theme.StyleBarFill.Render(bar),
		))
	}

	lines = append(lines, "")
	lines = append(lines, theme.StyleDim.Render(fmt.Sprintf("  Total: %s  Samples: %d", metrics.FormatMetricNumber(family.Total), len(family.Samples))))

	// Show all labels available
	allLabels := make([]string, 0, len(labelValues))
	for l := range labelValues {
		allLabels = append(allLabels, l)
	}
	sort.Strings(allLabels)
	lines = append(lines, theme.StyleDim.Render("  Labels: "+strings.Join(allLabels, ", ")))

	return theme.StyleChartPane.Width(width - 4).Render(strings.Join(lines, "\n"))
}
