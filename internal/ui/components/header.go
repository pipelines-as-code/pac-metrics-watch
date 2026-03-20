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

	status := "idle"
	if scraping {
		status = "scraping"
	}

	scopeStrs := make([]string, len(scopes))
	for i, scope := range scopes {
		if i == activeScope {
			scopeStrs[i] = theme.StyleScope.Render("[" + scope + "]")
		} else {
			scopeStrs[i] = theme.StyleDim.Render(scope)
		}
	}

	tabDashboard := theme.StyleTabInactive.Render("Dashboard (d)")
	tabRaw := theme.StyleTabInactive.Render("Raw (r)")
	if viewMode == "dashboard" {
		tabDashboard = theme.StyleTabActive.Render("Dashboard (d)")
	} else {
		tabRaw = theme.StyleTabActive.Render("Raw (r)")
	}

	topLine := theme.StyleHeader.Render(title) + "  " + strings.Join(scopeStrs, " | ")
	tabsLine := lipgloss.JoinHorizontal(lipgloss.Bottom, tabDashboard, " │ ", tabRaw)
	infoLine := theme.StyleDim.Render("sort:"+sortMode) + "  " +
		theme.StyleDim.Render("filter:"+filterLabel) + "  " +
		theme.StyleDim.Render("last:"+lastUpdateStr) + "  " +
		theme.StyleDim.Render("took:"+lastDuration.Round(time.Millisecond).String()) + "  " +
		theme.StyleDim.Render("state:"+status)

	return lipgloss.JoinVertical(lipgloss.Left, topLine, tabsLine, infoLine)
}
