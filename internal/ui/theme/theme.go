package theme

import "github.com/charmbracelet/lipgloss"

var (
	StyleHeader      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#61AFEF"))
	StyleSection     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#C678DD")).PaddingTop(1).PaddingBottom(1)
	StyleSep         = lipgloss.NewStyle().Foreground(lipgloss.Color("#3E4451"))
	StyleIncr        = lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379"))
	StyleDecr        = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))
	StyleDim         = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370"))
	StyleNormal      = lipgloss.NewStyle()
	StyleKey         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#56B6C2"))
	StyleScope       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E5C07B"))
	StyleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))
	StyleDetail      = lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF"))
	StyleCard        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#3E4451")).Padding(0, 1)
	StyleCardTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#56B6C2"))
	StyleUnavailable = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370"))

	StyleTabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E5C07B")).Background(lipgloss.Color("#2C313A")).Padding(0, 2)
	StyleTabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370")).Padding(0, 2)
	StyleChartPane   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#C678DD")).Padding(0, 1)
	StyleChartTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#E5C07B")).PaddingBottom(1)
	StyleTableCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ABB2BF")).Background(lipgloss.Color("#2C313A"))
	StyleTableHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ABB2BF")).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("#3E4451")).PaddingBottom(1)

	StyleAxisLabel = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370"))
	StyleMinMax    = lipgloss.NewStyle().Foreground(lipgloss.Color("#5C6370"))
	StyleBarFill   = lipgloss.NewStyle().Foreground(lipgloss.Color("#61AFEF"))
	StyleAltRow    = lipgloss.NewStyle().Background(lipgloss.Color("#1E2127"))
	StyleDotGreen  = lipgloss.NewStyle().Foreground(lipgloss.Color("#98C379"))
	StyleDotRed    = lipgloss.NewStyle().Foreground(lipgloss.Color("#E06C75"))
	StyleDotDim    = lipgloss.NewStyle().Foreground(lipgloss.Color("#3E4451"))
	StylePipeSep   = lipgloss.NewStyle().Foreground(lipgloss.Color("#3E4451"))
	StyleCardGreen = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#98C379")).Padding(0, 1)
	StyleCardRed   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#E06C75")).Padding(0, 1)
	StyleSparkDim  = lipgloss.NewStyle().Foreground(lipgloss.Color("#3E4451"))
)
