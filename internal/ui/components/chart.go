package components

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

const chartHeight = 8

// renderLineChart draws a Unicode line chart with Y-axis labels and min/max/avg annotation.
func renderLineChart(data []float64, width int) string {
	if len(data) < 2 {
		return theme.StyleDim.Render("Not enough data for chart")
	}

	allZeros := true
	for _, v := range data {
		if v != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		return theme.StyleDim.Render("All values are 0")
	}

	minVal, maxVal := data[0], data[0]
	sum := 0.0
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
		sum += v
	}
	avgVal := sum / float64(len(data))

	// Y-axis label width
	maxLabel := formatChartNum(maxVal)
	minLabel := formatChartNum(minVal)
	labelWidth := len(maxLabel)
	if len(minLabel) > labelWidth {
		labelWidth = len(minLabel)
	}
	labelWidth += 1 // space after label

	plotWidth := width - labelWidth - 3 // 3 for "│" and margins
	if plotWidth < 10 {
		plotWidth = 10
	}

	// Resample data to fit plotWidth
	samples := resample(data, plotWidth)

	// Braille-style blocks: use vertical eighth blocks for smoother rendering
	blocks := []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

	valRange := maxVal - minVal
	if valRange == 0 {
		valRange = 1
	}

	var lines []string
	for row := chartHeight - 1; row >= 0; row-- {
		var label string
		switch row {
		case chartHeight - 1:
			label = fmt.Sprintf("%*s", labelWidth, formatChartNum(maxVal))
		case 0:
			label = fmt.Sprintf("%*s", labelWidth, formatChartNum(minVal))
		case chartHeight / 2:
			midVal := minVal + valRange/2
			label = fmt.Sprintf("%*s", labelWidth, formatChartNum(midVal))
		default:
			label = strings.Repeat(" ", labelWidth)
		}

		var rowChars strings.Builder
		for _, s := range samples {
			// How much of this row is filled?
			normalized := (s - minVal) / valRange * float64(chartHeight)
			level := normalized - float64(row)
			if level <= 0 {
				rowChars.WriteRune(' ')
			} else if level >= 1 {
				rowChars.WriteRune('█')
			} else {
				idx := int(math.Round(level * float64(len(blocks)-1)))
				if idx >= len(blocks) {
					idx = len(blocks) - 1
				}
				rowChars.WriteRune(blocks[idx])
			}
		}

		axisChar := "│"
		line := theme.StyleAxisLabel.Render(label) + theme.StyleDim.Render(axisChar) + theme.StyleBarFill.Render(rowChars.String())
		lines = append(lines, line)
	}

	// Bottom axis
	axisLine := strings.Repeat(" ", labelWidth) + "└" + strings.Repeat("─", plotWidth)
	lines = append(lines, theme.StyleDim.Render(axisLine))

	// Annotation line
	annotation := theme.StyleMinMax.Render(fmt.Sprintf("  min=%s  max=%s  avg=%s  samples=%d",
		formatChartNum(minVal), formatChartNum(maxVal), formatChartNum(avgVal), len(data)))
	lines = append(lines, annotation)

	return strings.Join(lines, "\n")
}

// RenderBarChart draws a horizontal bar chart for delta-based signals.
func RenderBarChart(data []float64, width int) string {
	if len(data) < 2 {
		return theme.StyleDim.Render("Not enough data for chart")
	}

	// Compute deltas
	deltas := make([]float64, len(data)-1)
	for i := 1; i < len(data); i++ {
		deltas[i-1] = data[i] - data[i-1]
	}

	allZeros := true
	for _, d := range deltas {
		if d != 0 {
			allZeros = false
			break
		}
	}
	if allZeros {
		return theme.StyleDim.Render("No delta activity")
	}

	maxDelta := 0.0
	for _, d := range deltas {
		if math.Abs(d) > maxDelta {
			maxDelta = math.Abs(d)
		}
	}

	labelWidth := len(formatChartNum(maxDelta)) + 1
	barMaxWidth := width - labelWidth - 5
	if barMaxWidth < 10 {
		barMaxWidth = 10
	}

	// Show last chartHeight deltas
	visible := deltas
	if len(visible) > chartHeight {
		visible = visible[len(visible)-chartHeight:]
	}

	var lines []string
	for _, d := range visible {
		barLen := 0
		if maxDelta > 0 {
			barLen = int(math.Round(math.Abs(d) / maxDelta * float64(barMaxWidth)))
		}
		if barLen < 0 {
			barLen = 0
		}

		label := fmt.Sprintf("%*s", labelWidth, formatChartNum(d))
		bar := strings.Repeat("█", barLen)

		barStyle := theme.StyleIncr
		if d < 0 {
			barStyle = theme.StyleDecr
		} else if d == 0 {
			barStyle = theme.StyleDim
			bar = "·"
		}

		lines = append(lines, theme.StyleAxisLabel.Render(label)+" "+barStyle.Render(bar))
	}

	return strings.Join(lines, "\n")
}

func RenderDetailPane(title, kind, value, delta, description, sources string, history []float64, width int) string {
	infoLine := fmt.Sprintf("[%s] value=%s delta=%s sources=%s\n%s",
		kind, value, delta, sources, description)

	infoStyled := theme.StyleDetail.Render(infoLine)

	var chart string
	if kind == "counter" {
		chart = RenderBarChart(history, width-6)
	} else {
		chart = renderLineChart(history, width-6)
	}

	content := lipgloss.JoinVertical(lipgloss.Left, theme.StyleChartTitle.Render(title), infoStyled, "", chart)
	return theme.StyleChartPane.Width(width - 4).Render(content)
}

func resample(data []float64, targetLen int) []float64 {
	if len(data) <= targetLen {
		return data
	}
	result := make([]float64, targetLen)
	ratio := float64(len(data)) / float64(targetLen)
	for i := range targetLen {
		srcIdx := int(float64(i) * ratio)
		if srcIdx >= len(data) {
			srcIdx = len(data) - 1
		}
		result[i] = data[srcIdx]
	}
	return result
}

func formatChartNum(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%.1fK", v/1_000)
	case abs == math.Trunc(abs):
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}
