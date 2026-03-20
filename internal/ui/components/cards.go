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
	SignalID  string
	DeltaNum  float64
}

func RenderSummaryCards(cards []CardData, width int) string {
	if len(cards) == 0 {
		return theme.StyleUnavailable.Render("No curated PAC metrics available yet.")
	}

	numCards := len(cards)
	gaps := numCards - 1
	// Each card border takes 4 chars (2 border + 2 padding)
	cardWidth := (width - gaps*1 - numCards*4) / numCards
	if cardWidth < 16 {
		cardWidth = 16
	}

	renderedCards := make([]string, 0, numCards)
	for _, card := range cards {
		titleStr := theme.StyleCardTitle.Render(truncate(card.Title, cardWidth))
		var content string
		if card.Available {
			deltaStyle := theme.StyleDim
			if card.DeltaNum > 0 {
				deltaStyle = theme.StyleIncr
			} else if card.DeltaNum < 0 {
				deltaStyle = theme.StyleDecr
			}
			content = titleStr + "\n" + card.Value + "  " + deltaStyle.Render(card.Delta)
			if card.Trend != "" {
				trendRunes := []rune(card.Trend)
				if len(trendRunes) > cardWidth {
					trendRunes = trendRunes[len(trendRunes)-cardWidth:]
				}
				content += "\n" + theme.StyleSparkDim.Render(string(trendRunes))
			}
		} else {
			content = titleStr + "\n" + theme.StyleUnavailable.Render("n/a")
		}

		cardStyle := theme.StyleCard.Width(cardWidth)
		// Conditional border color
		if card.Available {
			if card.SignalID == "workqueue-depth" && card.DeltaNum > 0 {
				cardStyle = theme.StyleCardRed.Width(cardWidth)
			} else if card.DeltaNum > 0 {
				cardStyle = theme.StyleCardGreen.Width(cardWidth)
			}
		}
		renderedCards = append(renderedCards, cardStyle.Render(content))
	}

	// Responsive wrapping
	if width < 80 && len(renderedCards) > 1 {
		// Stack all cards vertically
		return lipgloss.JoinVertical(lipgloss.Left, renderedCards...)
	}
	if width < 120 && len(renderedCards) > 2 {
		top := lipgloss.JoinHorizontal(lipgloss.Top, renderedCards[:2]...)
		bottom := lipgloss.JoinHorizontal(lipgloss.Top, renderedCards[2:]...)
		return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, renderedCards...)
}

func truncate(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}
	return string(runes[:maxLen-1]) + "…"
}
