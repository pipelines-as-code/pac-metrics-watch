package components

import (
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderHeader(title string, scopes []string, activeScope int, viewMode string, sortMode string, filterLabel string, lastUpdate time.Time, lastDuration time.Duration, scraping bool) string {
	lastUpdateStr := "never"
	if !lastUpdate.IsZero() {
		lastUpdateStr = lastUpdate.Format("15:04:05")
	}

	statusDot := theme.StyleDotGreen.Render("●")
	status := "idle"
	if scraping {
		statusDot = theme.StyleScope.Render("●")
		status = "scraping"
	}
	if lastUpdate.IsZero() && !scraping {
		statusDot = theme.StyleDotDim.Render("○")
		status = "waiting"
	}

	scopeStrs := make([]string, len(scopes))
	for i, scope := range scopes {
		if i == activeScope {
			scopeStrs[i] = theme.StyleScope.Render("[" + scope + "]")
		} else {
			scopeStrs[i] = theme.StyleDim.Render(scope)
		}
	}

	tabs := []struct {
		label string
		mode  string
	}{
		{"Dashboard (d)", "dashboard"},
		{"Raw (r)", "raw"},
		{"Resources (m)", "resources"},
		{"Health (h)", "health"},
		{"Repos (p)", "repos"},
		{"Events (e)", "events"},
	}

	pipe := theme.StylePipeSep.Render(" │ ")

	tabStrs := make([]string, len(tabs))
	for i, tab := range tabs {
		if viewMode == tab.mode {
			tabStrs[i] = theme.StyleTabActive.Render(tab.label)
		} else {
			tabStrs[i] = theme.StyleTabInactive.Render(tab.label)
		}
	}

	topLine := theme.StyleHeader.Render(title) + "  " + strings.Join(scopeStrs, pipe)
	tabsLine := lipgloss.JoinHorizontal(lipgloss.Bottom, tabStrs[0], pipe, tabStrs[1], pipe, tabStrs[2], pipe, tabStrs[3], pipe, tabStrs[4], pipe, tabStrs[5])
	infoLine := statusDot + " " + theme.StyleDim.Render(status) +
		pipe + theme.StyleDim.Render("sort:"+sortMode) +
		pipe + theme.StyleDim.Render("filter:"+filterLabel) +
		pipe + theme.StyleDim.Render("last:"+lastUpdateStr) +
		pipe + theme.StyleDim.Render("took:"+lastDuration.Round(time.Millisecond).String())

	return lipgloss.JoinVertical(lipgloss.Left, topLine, tabsLine, infoLine)
}
