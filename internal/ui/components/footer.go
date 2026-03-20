package components

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderFooter(err string, filterMode bool, filterInput string, width int) string {
	pipe := theme.StylePipeSep.Render(" │ ")

	help := theme.StyleKey.Render("[q]") + " quit" + pipe +
		theme.StyleKey.Render("[tab]") + " scope" + pipe +
		theme.StyleKey.Render("[d]") + " dashboard" + pipe +
		theme.StyleKey.Render("[r]") + " raw" + pipe +
		theme.StyleKey.Render("[f]") + " pac-only" + pipe +
		theme.StyleKey.Render("[s]") + " sort" + pipe +
		theme.StyleKey.Render("[/]") + " filter" + pipe +
		theme.StyleKey.Render("[↑↓/jk]") + " scroll"

	var parts []string
	if filterMode {
		parts = append(parts, theme.StyleDetail.Render("filter: type to narrow raw metrics, enter to keep, esc to clear"))
		parts = append(parts, theme.StyleDetail.Render("filter> "+filterInput))
	}
	if err != "" {
		parts = append(parts, theme.StyleError.Render("error: "+err))
	}
	parts = append(parts, help)

	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}
