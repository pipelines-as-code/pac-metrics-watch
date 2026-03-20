package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/kubectl"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/components"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

func RenderReposView(repos []kubectl.RepositoryStatus, cursor, visibleStart, maxRows, width int, loading bool, errMsg string) ([]string, string) {
	if loading {
		return []string{theme.StyleDim.Render("  Loading repository statuses...")}, ""
	}

	if errMsg != "" {
		return []string{theme.StyleError.Render("  " + errMsg)}, ""
	}

	if len(repos) == 0 {
		return []string{theme.StyleDim.Render("  No Repository CRs found.")}, ""
	}

	nsW := max(12, width/6)
	repoW := max(16, width/5)
	prW := max(20, width/5)
	statusW := max(10, width/10)
	eventW := max(10, width/10)
	shaW := 9
	completedW := max(14, width-nsW-repoW-prW-statusW-eventW-shaW-14)

	columns := []components.Column{
		{Title: "NAMESPACE", Width: nsW},
		{Title: "REPOSITORY", Width: repoW},
		{Title: "LAST PIPELINERUN", Width: prW},
		{Title: "STATUS", Width: statusW},
		{Title: "EVENT", Width: eventW},
		{Title: "SHA", Width: shaW},
		{Title: "COMPLETED", Width: completedW},
	}

	var tableRows []components.TableRow
	dataIdx := 0
	for _, repo := range repos {
		if len(repo.PipelineRuns) == 0 {
			if dataIdx >= visibleStart && dataIdx < visibleStart+maxRows {
				tableRows = append(tableRows, components.TableRow{
					Columns: []string{repo.Namespace, repo.Name, "n/a", "n/a", "n/a", "n/a", "n/a"},
					Style:   theme.StyleDim,
				})
			}
			dataIdx++
			continue
		}

		pr := repo.PipelineRuns[0]
		rowStyle := PipelineRunStatusStyle(pr.Status)

		completedStr := "n/a"
		if !pr.Completed.IsZero() {
			completedStr = RelativeTime(pr.Completed)
		}

		if dataIdx >= visibleStart && dataIdx < visibleStart+maxRows {
			tableRows = append(tableRows, components.TableRow{
				Columns: []string{repo.Namespace, repo.Name, pr.Name, pr.Status, pr.EventType, pr.SHA, completedStr},
				Style:   rowStyle,
			})
		}
		dataIdx++
	}

	rendered := components.RenderTable(columns, tableRows, cursor-visibleStart)

	detail := ""
	repoIdx := cursor
	if repoIdx >= 0 && repoIdx < len(repos) {
		repo := repos[repoIdx]
		detail = renderRepoDetail(repo, width)
	}

	return rendered, detail
}

func renderRepoDetail(repo kubectl.RepositoryStatus, width int) string {
	var lines []string
	lines = append(lines, theme.StyleChartTitle.Render(repo.Namespace+"/"+repo.Name))
	lines = append(lines, "")

	if len(repo.PipelineRuns) == 0 {
		lines = append(lines, theme.StyleDim.Render("  No PipelineRun history"))
		return strings.Join(lines, "\n")
	}

	lines = append(lines, theme.StyleTableHeader.Render(fmt.Sprintf("  %-30s %-12s %-12s %-9s %s", "PIPELINERUN", "STATUS", "EVENT", "SHA", "COMPLETED")))
	for _, pr := range repo.PipelineRuns {
		statusStyle := PipelineRunStatusStyle(pr.Status)
		completedStr := "n/a"
		if !pr.Completed.IsZero() {
			completedStr = RelativeTime(pr.Completed)
		}
		lines = append(lines, fmt.Sprintf("  %-30s %s %-12s %-9s %s",
			TruncateStr(pr.Name, 30),
			statusStyle.Render(fmt.Sprintf("%-12s", pr.Status)),
			pr.EventType,
			pr.SHA,
			completedStr,
		))
	}

	return theme.StyleChartPane.Width(width - 4).Render(strings.Join(lines, "\n"))
}

func RenderReposSnapshot(repos []kubectl.RepositoryStatus) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "%-20s %-20s %-30s %-12s %-12s %-9s %s\n",
		"NAMESPACE", "REPOSITORY", "LAST PIPELINERUN", "STATUS", "EVENT", "SHA", "COMPLETED")
	builder.WriteString(strings.Repeat("-", 110) + "\n")

	for _, repo := range repos {
		if len(repo.PipelineRuns) == 0 {
			fmt.Fprintf(&builder, "%-20s %-20s %-30s %-12s %-12s %-9s %s\n",
				repo.Namespace, repo.Name, "n/a", "n/a", "n/a", "n/a", "n/a")
			continue
		}
		pr := repo.PipelineRuns[0]
		completedStr := "n/a"
		if !pr.Completed.IsZero() {
			completedStr = pr.Completed.Format(time.RFC3339)
		}
		fmt.Fprintf(&builder, "%-20s %-20s %-30s %-12s %-12s %-9s %s\n",
			repo.Namespace, repo.Name, pr.Name, pr.Status, pr.EventType, pr.SHA, completedStr)
	}

	return builder.String()
}
