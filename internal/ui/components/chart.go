package components

import (
	"fmt"
	"math"

	"github.com/NimbleMarkets/ntcharts/v2/canvas/runes"
	"github.com/NimbleMarkets/ntcharts/v2/linechart"
	"github.com/NimbleMarkets/ntcharts/v2/linechart/streamlinechart"
	ntlipgloss "charm.land/lipgloss/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

const (
	chartHeight   = 8
	chartMinWidth = 24
)

var (
	chartAxisStyle  = ntlipgloss.NewStyle().Foreground(ntlipgloss.Color("#3E4451"))
	chartLabelStyle = ntlipgloss.NewStyle().Foreground(ntlipgloss.Color("#5C6370"))
	chartLineStyle  = ntlipgloss.NewStyle().Foreground(ntlipgloss.Color("#61AFEF"))
	chartIncrStyle  = ntlipgloss.NewStyle().Foreground(ntlipgloss.Color("#98C379"))
	chartDecrStyle  = ntlipgloss.NewStyle().Foreground(ntlipgloss.Color("#E06C75"))
)

func renderMetricChart(kind string, history []float64, width int) string {
	series := history
	seriesLabel := "samples"
	lineStyle := chartLineStyle

	if kind == "counter" {
		series = counterSeries(history)
		seriesLabel = "deltas"
		if len(series) == 0 {
			return theme.StyleDim.Render("Not enough data for chart")
		}
		if allZero(series) {
			return theme.StyleDim.Render("No delta activity")
		}
		if series[len(series)-1] < 0 {
			lineStyle = chartDecrStyle
		} else {
			lineStyle = chartIncrStyle
		}
	} else if len(series) < 2 {
		return theme.StyleDim.Render("Not enough data for chart")
	}

	chart := newStreamlineChart(series, width, lineStyle)
	minVal, maxVal, avgVal := chartStats(series)
	annotation := theme.StyleMinMax.Render(fmt.Sprintf("  min=%s  max=%s  avg=%s  %s=%d",
		formatChartNum(minVal), formatChartNum(maxVal), formatChartNum(avgVal), seriesLabel, len(series)))

	return lipgloss.JoinVertical(lipgloss.Left, chart.View(), annotation)
}

func RenderDetailPane(title, kind, value, delta, description, sources string, history []float64, width int) string {
	infoLine := fmt.Sprintf("[%s] value=%s delta=%s sources=%s\n%s",
		kind, value, delta, sources, description)

	infoStyled := theme.StyleDetail.Render(infoLine)

	chart := renderMetricChart(kind, history, width-6)

	content := lipgloss.JoinVertical(lipgloss.Left, theme.StyleChartTitle.Render(title), infoStyled, "", chart)
	return theme.StyleChartPane.Width(width - 4).Render(content)
}

func newStreamlineChart(series []float64, width int, lineStyle ntlipgloss.Style) streamlinechart.Model {
	minVal, maxVal := chartRange(series)
	chartWidth := max(chartMinWidth, width)
	model := linechart.New(
		chartWidth,
		chartHeight,
		0,
		float64(max(len(series)-1, 1)),
		minVal,
		maxVal,
		linechart.WithXYSteps(0, 2),
		linechart.WithYLabelFormatter(func(_ int, v float64) string { return formatChartNum(v) }),
		linechart.WithStyles(chartAxisStyle, chartLabelStyle, lineStyle),
	)

	chart := streamlinechart.New(
		chartWidth,
		chartHeight,
		streamlinechart.WithLineChart(&model),
		streamlinechart.WithStyles(runes.ArcLineStyle, lineStyle),
	)
	for _, value := range series {
		chart.Push(value)
	}
	chart.Draw()
	return chart
}

func counterSeries(history []float64) []float64 {
	if len(history) < 2 {
		return nil
	}

	deltas := make([]float64, 0, len(history)-1)
	for i := 1; i < len(history); i++ {
		deltas = append(deltas, history[i]-history[i-1])
	}
	return deltas
}

func chartStats(data []float64) (float64, float64, float64) {
	if len(data) == 0 {
		return 0, 0, 0
	}

	minVal, maxVal := data[0], data[0]
	sum := 0.0
	for _, value := range data {
		if value < minVal {
			minVal = value
		}
		if value > maxVal {
			maxVal = value
		}
		sum += value
	}
	return minVal, maxVal, sum / float64(len(data))
}

func chartRange(data []float64) (float64, float64) {
	minVal, maxVal, _ := chartStats(data)
	if minVal == maxVal {
		padding := math.Max(1, math.Abs(minVal)*0.2)
		return minVal - padding, maxVal + padding
	}
	return minVal, maxVal
}

func allZero(data []float64) bool {
	for _, value := range data {
		if value != 0 {
			return false
		}
	}
	return true
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
