package components

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderFooter(err string, filterMode bool, filterInput string, width int) string {
	help := theme.StyleKey.Render("[q]") + "quit  " +
		theme.StyleKey.Render("[tab]") + "scope  " +
		theme.StyleKey.Render("[d]") + "dashboard  " +
		theme.StyleKey.Render("[r]") + "raw  " +
		theme.StyleKey.Render("[f]") + "pac-only  " +
		theme.StyleKey.Render("[s]") + "sort  " +
		theme.StyleKey.Render("[/]") + "filter  " +
		theme.StyleKey.Render("[↑↓/jk]") + "scroll"

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
