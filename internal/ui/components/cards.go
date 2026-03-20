package components

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

type CardData struct {
	Title     string
	Value     string
	Delta     string
	Trend     string
	Available bool
}

func RenderSummaryCards(cards []CardData, width int) string {
	if len(cards) == 0 {
		return theme.StyleUnavailable.Render("No curated PAC metrics available yet.")
	}

	renderedCards := make([]string, 0, len(cards))
	for _, card := range cards {
		content := theme.StyleCardTitle.Render(card.Title) + "\n"
		if card.Available {
			content += card.Value + "  " + card.Delta
			if card.Trend != "" {
				content += "\n" + card.Trend
			}
		} else {
			content += "n/a  n/a"
		}

		renderedCards = append(renderedCards, theme.StyleCard.Render(content))
	}

	if width < 120 && len(renderedCards) > 2 {
		top := lipgloss.JoinHorizontal(lipgloss.Top, renderedCards[:2]...)
		bottom := lipgloss.JoinHorizontal(lipgloss.Top, renderedCards[2:]...)
		return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedCards...)
}
