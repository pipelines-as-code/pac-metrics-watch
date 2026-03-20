package components

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/guptarohit/asciigraph"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderDetailPane(title, kind, value, delta, description, sources string, history []float64, width int) string {
	infoLine := fmt.Sprintf("[%s] value=%s delta=%s sources=%s\n%s",
		kind, value, delta, sources, description)

	infoStyled := theme.StyleDetail.Render(infoLine)

	var chart string
	if len(history) < 2 {
		chart = theme.StyleDim.Render("Not enough data for chart")
	} else {
		chartWidth := width - 15
		if chartWidth < 20 {
			chartWidth = 20
		}

		allZeros := true
		for _, v := range history {
			if v != 0 {
				allZeros = false
				break
			}
		}

		if allZeros {
			chart = theme.StyleDim.Render("All values are 0")
		} else {
			graph := asciigraph.Plot(history, asciigraph.Width(chartWidth), asciigraph.Height(5), asciigraph.Precision(2))
			chart = theme.StyleNormal.Render(graph)
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Left, theme.StyleChartTitle.Render(title), infoStyled, "", chart)
	return theme.StyleChartPane.Width(width - 4).Render(content)
}
