package theme

import "github.com/charmbracelet/lipgloss"

var (
	StyleHeader      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("33"))
	StyleSection     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69")).PaddingTop(1).PaddingBottom(1)
	StyleSep         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	StyleIncr        = lipgloss.NewStyle().Foreground(lipgloss.Color("82"))
	StyleDecr        = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	StyleDim         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	StyleNormal      = lipgloss.NewStyle()
	StyleKey         = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	StyleScope       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	StyleError       = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	StyleDetail      = lipgloss.NewStyle().Foreground(lipgloss.Color("111"))
	StyleCard        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	StyleCardTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("45"))
	StyleUnavailable = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))

	// Additional styles for beautification
	StyleTabActive   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).Background(lipgloss.Color("236")).Padding(0, 2)
	StyleTabInactive = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Padding(0, 2)
	StyleChartPane   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("69")).Padding(0, 1)
	StyleChartTitle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214")).PaddingBottom(1)
	StyleTable       = lipgloss.NewStyle().MarginTop(1)
	StyleTableRow    = lipgloss.NewStyle()
	StyleTableCursor = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("236"))
	StyleTableHeader = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Border(lipgloss.NormalBorder(), false, false, true, false).BorderForeground(lipgloss.Color("240")).PaddingBottom(1)
)
