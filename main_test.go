package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/metrics"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/views"
)

func mustModel(t *testing.T, m tea.Model) *model {
	t.Helper()
	mm, ok := m.(*model)
	if !ok {
		t.Fatalf("expected *model, got %T", m)
	}
	return mm
}

func TestParseMetricsAggregatesMetricFamilies(t *testing.T) {
	m, err := metrics.ParseMetrics(`
# HELP pac_events_total Number of PAC events
pac_events_total{provider="github"} 2
pac_events_total{provider="gitlab"} 3
workqueue_depth{name="foo"} 4
`)
	if err != nil {
		t.Fatalf("ParseMetrics returned error: %v", err)
	}

	if got, want := m["pac_events_total"], 5.0; got != want {
		t.Fatalf("pac_events_total = %v, want %v", got, want)
	}
	if got, want := m["workqueue_depth"], 4.0; got != want {
		t.Fatalf("workqueue_depth = %v, want %v", got, want)
	}
}

func TestParseWithLabelsPreservesLabels(t *testing.T) {
	families, err := metrics.ParseWithLabels(`
pac_events_total{provider="github"} 2
pac_events_total{provider="gitlab"} 3
workqueue_depth{name="foo"} 4
`)
	if err != nil {
		t.Fatalf("ParseWithLabels returned error: %v", err)
	}

	family, ok := families["pac_events_total"]
	if !ok {
		t.Fatal("pac_events_total family not found")
	}
	if got, want := len(family.Samples), 2; got != want {
		t.Fatalf("len(samples) = %d, want %d", got, want)
	}
	if got, want := family.Total, 5.0; got != want {
		t.Fatalf("total = %v, want %v", got, want)
	}

	// Check that labels are preserved
	foundGithub := false
	for _, s := range family.Samples {
		if s.Labels["provider"] == "github" && s.Value == 2 {
			foundGithub = true
		}
	}
	if !foundGithub {
		t.Fatal("github provider sample not found")
	}
}

func TestBuildEndpointsUsesMetricsPort(t *testing.T) {
	endpoints := metrics.BuildEndpoints(defaultNamespace)
	if len(endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2", len(endpoints))
	}

	if got := endpoints[0].SvcPath; got != "/api/v1/namespaces/pipelines-as-code/services/pipelines-as-code-controller:9090/proxy/metrics" {
		t.Fatalf("controller svcPath = %q", got)
	}
	if got := endpoints[1].SvcPath; got != "/api/v1/namespaces/pipelines-as-code/services/pipelines-as-code-watcher:9090/proxy/metrics" {
		t.Fatalf("watcher svcPath = %q", got)
	}
}

func TestBuildScopes(t *testing.T) {
	scopes := metrics.BuildScopes(metrics.BuildEndpoints(defaultNamespace))
	if got := scopes[0].Name; got != "all" {
		t.Fatalf("scopes[0].Name = %q, want all", got)
	}
	if len(scopes[0].EndpointIndexes) != 2 {
		t.Fatalf("len(scopes[0].EndpointIndexes) = %d, want 2", len(scopes[0].EndpointIndexes))
	}
}

func TestCanonicalMetricName(t *testing.T) {
	if got := metrics.CanonicalMetricName("pac_controller_pipelines_as_code_git_provider_api_request_count"); got != "pipelines_as_code_git_provider_api_request_count" {
		t.Fatalf("CanonicalMetricName(controller) = %q", got)
	}
	if got := metrics.CanonicalMetricName("pac_watcher_workqueue_depth"); got != "workqueue_depth" {
		t.Fatalf("CanonicalMetricName(watcher) = %q", got)
	}
}

func TestBuildDashboardRowsCombinesControllerAndWatcherSignals(t *testing.T) {
	history := map[string][]float64{
		"pac_controller_pipelines_as_code_git_provider_api_request_count": {1, 5},
		"pac_watcher_pipelines_as_code_git_provider_api_request_count":    {2, 4},
		"pac_watcher_pipelines_as_code_running_pipelineruns_count":        {1, 3},
	}
	delta := map[string]float64{
		"pac_controller_pipelines_as_code_git_provider_api_request_count": 4,
		"pac_watcher_pipelines_as_code_git_provider_api_request_count":    2,
		"pac_watcher_pipelines_as_code_running_pipelineruns_count":        2,
	}

	rows := metrics.BuildDashboardRows(history, delta)
	var apiRow metrics.DashboardRow
	found := false
	for _, row := range rows {
		if row.Signal.ID == "git-api-requests" {
			apiRow = row
			found = true
			break
		}
	}
	if !found {
		t.Fatal("git-api-requests row not found")
	}
	if !apiRow.Available {
		t.Fatal("git-api-requests row marked unavailable")
	}
	if got, want := apiRow.Value, 9.0; got != want {
		t.Fatalf("apiRow.Value = %v, want %v", got, want)
	}
	if got, want := apiRow.Delta, 6.0; got != want {
		t.Fatalf("apiRow.Delta = %v, want %v", got, want)
	}
}

func TestParseMetricsScannerError(t *testing.T) {
	line := strings.Repeat("a", 70*1024) + " 1\n"
	if _, err := metrics.ParseMetrics(line); err == nil {
		t.Fatal("ParseMetrics returned nil error for oversized token")
	}
}

func TestMergeEndpointMetricsPrefixesRuntimeCollectors(t *testing.T) {
	merged := map[string]float64{}
	mergeEndpointMetrics(merged, "watcher", map[string]float64{
		"go_goroutines":                 17,
		"process_resident_memory_bytes": 4096,
		"workqueue_depth":               3,
	})

	if got := merged["pac_watcher_go_goroutines"]; got != 17 {
		t.Fatalf("pac_watcher_go_goroutines = %v, want 17", got)
	}
	if got := merged["pac_watcher_process_resident_memory_bytes"]; got != 4096 {
		t.Fatalf("pac_watcher_process_resident_memory_bytes = %v, want 4096", got)
	}
	if got := merged["workqueue_depth"]; got != 3 {
		t.Fatalf("workqueue_depth = %v, want 3", got)
	}
	if _, ok := merged["go_goroutines"]; ok {
		t.Fatal("go_goroutines should be endpoint-scoped")
	}
	if _, ok := merged["process_resident_memory_bytes"]; ok {
		t.Fatal("process_resident_memory_bytes should be endpoint-scoped")
	}
}

func TestBuildRowsFromHistoryRespectsFilterAndSort(t *testing.T) {
	history := map[string][]float64{
		"pac_active_repositories":   {1, 5},
		"workqueue_queue_duration":  {3, 4},
		"controller_runtime_errors": {2, 2},
	}
	delta := map[string]float64{
		"pac_active_repositories":   4,
		"workqueue_queue_duration":  1,
		"controller_runtime_errors": 0,
	}

	rows := metrics.BuildRowsFromHistory(history, delta, false, "queue", metrics.SortByDelta)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Name != "workqueue_queue_duration" {
		t.Fatalf("rows[0].Name = %q, want workqueue_queue_duration", rows[0].Name)
	}

	rows = metrics.BuildRowsFromHistory(history, delta, false, "", metrics.SortByDelta)
	if rows[0].Name != "pac_active_repositories" {
		t.Fatalf("rows sorted by delta = %q, want pac_active_repositories", rows[0].Name)
	}
}

func TestModelInitStartsImmediateScrape(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
		return map[string]float64{"pac_controller_pipelines_as_code_pipelinerun_count": 7}, nil
	})

	cmd := m.Init()
	if !m.scraping {
		t.Fatal("model.scraping = false, want true after Init")
	}
	if cmd == nil {
		t.Fatal("Init returned nil command")
	}

	msg := cmd()
	result, ok := msg.(scrapeCycleResultMsg)
	if !ok {
		t.Fatalf("Init command returned %T, want scrapeCycleResultMsg", msg)
	}
	if result.scope != 0 {
		t.Fatalf("result.scope = %d, want 0", result.scope)
	}
}

func TestTickIgnoredWhileScraping(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.scraping = true
	m.scrapeSeq = 9

	nextModel, cmd := m.Update(tickMsg(time.Now()))
	if cmd != nil {
		t.Fatal("tick during active scrape returned a command")
	}
	updated := mustModel(t, nextModel)
	if updated.scrapeSeq != 9 {
		t.Fatalf("scrapeSeq = %d, want 9", updated.scrapeSeq)
	}
}

func TestScrapeResultSchedulesNextTickAndUpdatesMetrics(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.scraping = true
	m.scrapeSeq = 1

	nextModel, cmd := m.Update(scrapeCycleResultMsg{
		id:       1,
		scope:    0,
		metrics:  map[string]float64{"pac_controller_pipelines_as_code_pipelinerun_count": 5},
		duration: 25 * time.Millisecond,
	})

	if cmd == nil {
		t.Fatal("scrape result did not schedule next tick")
	}

	updated := mustModel(t, nextModel)
	if updated.scraping {
		t.Fatal("model.scraping = true, want false after scrape result")
	}
	if got := updated.history["pac_controller_pipelines_as_code_pipelinerun_count"]; len(got) != 1 || got[0] != 5 {
		t.Fatalf("history = %#v, want [5]", got)
	}
	if updated.lastDuration != 25*time.Millisecond {
		t.Fatalf("lastDuration = %v, want 25ms", updated.lastDuration)
	}
}

func TestSwitchScopeResetsStateAndCancelsScrape(t *testing.T) {
	canceled := false
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
		return map[string]float64{"pac_controller_pipelines_as_code_pipelinerun_count": 1}, nil
	})
	m.scraping = true
	m.cancelScrape = func() { canceled = true }
	m.cursor = 6
	m.visibleStart = 3
	m.history["pac_controller_pipelines_as_code_pipelinerun_count"] = []float64{1, 2}
	m.delta["pac_controller_pipelines_as_code_pipelinerun_count"] = 1
	m.keys = []string{"pac_controller_pipelines_as_code_pipelinerun_count"}
	m.err = "old error"
	m.lastUpdate = time.Now()

	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if cmd == nil {
		t.Fatal("switch scope did not trigger immediate scrape")
	}

	updated := mustModel(t, nextModel)
	if !canceled {
		t.Fatal("active scrape was not canceled")
	}
	if updated.activeScope != 1 {
		t.Fatalf("activeScope = %d, want 1", updated.activeScope)
	}
	if updated.cursor != 0 || updated.visibleStart != 0 {
		t.Fatalf("cursor/visibleStart = %d/%d, want 0/0", updated.cursor, updated.visibleStart)
	}
	if len(updated.history) != 0 || len(updated.keys) != 0 {
		t.Fatalf("history/keys were not reset: %#v %#v", updated.history, updated.keys)
	}
	if !updated.lastUpdate.IsZero() {
		t.Fatal("lastUpdate was not reset")
	}
}

func TestCanceledScrapeResultIsIgnored(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.scraping = true
	m.scrapeSeq = 2

	nextModel, cmd := m.Update(scrapeCycleResultMsg{
		id:    2,
		scope: 0,
		err:   context.Canceled,
	})

	if cmd != nil {
		t.Fatal("canceled scrape should not schedule next tick")
	}
	updated := mustModel(t, nextModel)
	if updated.err != "" {
		t.Fatalf("err = %q, want empty", updated.err)
	}
}

func TestRenderSnapshotTSV(t *testing.T) {
	output := renderSnapshot("all", time.Unix(0, 0).UTC(), map[string]float64{
		"pac_controller_pipelines_as_code_pipelinerun_count": 5,
	}, "tsv")

	if !strings.Contains(output, "# scope=all") {
		t.Fatalf("output missing scope metadata: %q", output)
	}
	if !strings.Contains(output, "metric\tvalue") {
		t.Fatalf("output missing header: %q", output)
	}
	if !strings.Contains(output, "pac_controller_pipelines_as_code_pipelinerun_count\t5") {
		t.Fatalf("output missing metric row: %q", output)
	}
}

func TestRunSnapshotIncludesScrapeErrorText(t *testing.T) {
	_, err := runSnapshot(metrics.SnapshotConfig{
		Namespace: defaultNamespace,
		Scope:     "controller",
		Output:    "table",
		SortMode:  metrics.SortByAlpha,
	}, func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
		return nil, errors.New("forbidden: services/proxy is not allowed")
	})

	if err == nil {
		t.Fatal("runSnapshot returned nil error")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("error = %q, want forbidden details", err)
	}
}

func TestHealthViewSwitch(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "h"})
	updated := mustModel(t, nextModel)
	if updated.viewMode != metrics.ViewHealth {
		t.Fatalf("viewMode = %q, want health", updated.viewMode)
	}
	if !updated.healthLoading {
		t.Fatal("healthLoading = false, want true")
	}
	if cmd == nil {
		t.Fatal("switching to health view should trigger health checks")
	}
}

func TestReposViewSwitch(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "p"})
	updated := mustModel(t, nextModel)
	if updated.viewMode != metrics.ViewRepos {
		t.Fatalf("viewMode = %q, want repos", updated.viewMode)
	}
	if !updated.reposLoading {
		t.Fatal("reposLoading = false, want true")
	}
	if cmd == nil {
		t.Fatal("switching to repos view should trigger fetch")
	}
}

func TestEventsViewSwitch(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "e"})
	updated := mustModel(t, nextModel)
	if updated.viewMode != metrics.ViewEvents {
		t.Fatalf("viewMode = %q, want events", updated.viewMode)
	}
	if !updated.eventsLoading {
		t.Fatal("eventsLoading = false, want true")
	}
	if cmd == nil {
		t.Fatal("switching to events view should trigger fetch")
	}
}

func TestResourcesViewSwitch(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: -1, Text: "m"})
	updated := mustModel(t, nextModel)
	if updated.viewMode != metrics.ViewResources {
		t.Fatalf("viewMode = %q, want resources", updated.viewMode)
	}
	if cmd != nil {
		t.Fatal("switching to resources view should not trigger a side fetch")
	}
}

func TestResourcesFocusCyclesWithS(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.viewMode = metrics.ViewResources

	nextModel, _ := m.Update(tea.KeyPressMsg{Code: -1, Text: "s"})
	updated := mustModel(t, nextModel)
	if updated.resourceFocus != views.ResourceFocusRuntime {
		t.Fatalf("resourceFocus = %q, want runtime", updated.resourceFocus)
	}

	nextModel, _ = updated.Update(tea.KeyPressMsg{Code: -1, Text: "s"})
	updated = mustModel(t, nextModel)
	if updated.resourceFocus != views.ResourceFocusQueue {
		t.Fatalf("resourceFocus = %q, want queue", updated.resourceFocus)
	}
}

func TestRenderWithFooterPadsToBottom(t *testing.T) {
	got := renderWithFooter([]string{"header", "content"}, "footer", 6)
	lines := strings.Split(got, "\n")
	if len(lines) != 6 {
		t.Fatalf("len(lines) = %d, want 6", len(lines))
	}
	if lines[len(lines)-1] != "footer" {
		t.Fatalf("last line = %q, want footer", lines[len(lines)-1])
	}
}

func TestBuildResourceSectionUsesEndpointScopedRuntimeMetrics(t *testing.T) {
	section := buildResourceSection(
		"watcher",
		views.ResourceFocusMemory,
		map[string][]float64{
			"pac_watcher_process_resident_memory_bytes": {100, 160},
			"pac_watcher_go_memstats_heap_inuse_bytes":  {80, 120},
			"pac_watcher_go_memstats_alloc_bytes":       {60, 90},
			"pac_watcher_go_goroutines":                 {11, 14},
		},
		map[string]float64{
			"pac_watcher_process_resident_memory_bytes": 60,
			"pac_watcher_go_memstats_heap_inuse_bytes":  40,
			"pac_watcher_go_memstats_alloc_bytes":       30,
			"pac_watcher_go_goroutines":                 3,
		},
		map[string][]float64{},
		map[string]float64{},
		nil,
	)

	if section.PrimaryTitle != "Watcher Process RSS" {
		t.Fatalf("PrimaryTitle = %q, want Watcher Process RSS", section.PrimaryTitle)
	}

	stats := map[string]views.ResourceStat{}
	for _, stat := range section.Stats {
		stats[stat.Label] = stat
	}

	for _, label := range []string{"RSS", "Heap In Use", "Alloc", "Goroutines"} {
		stat, ok := stats[label]
		if !ok {
			t.Fatalf("missing stat %q", label)
		}
		if !stat.Available {
			t.Fatalf("stat %q should be available", label)
		}
		if stat.Value == "" || stat.Value == "n/a" {
			t.Fatalf("stat %q value = %q, want populated value", label, stat.Value)
		}
	}
}

func TestRenderRawSnapshotTable(t *testing.T) {
	output := renderRawSnapshot("all", time.Unix(0, 0).UTC(), map[string]float64{
		"pac_controller_pipelines_as_code_pipelinerun_count": 5,
		"process_start_time_seconds":                         1000,
	}, metrics.SnapshotConfig{
		PacOnly:  true,
		SortMode: metrics.SortByAlpha,
		Output:   "table",
	})

	if !strings.Contains(output, "view:") || !strings.Contains(output, "raw") {
		t.Fatalf("output missing view metadata: %q", output)
	}
	if !strings.Contains(output, "METRIC") || !strings.Contains(output, "VALUE") {
		t.Fatalf("output missing table header: %q", output)
	}
	if !strings.Contains(output, "pac_controller_pipelines_as_code_pipelinerun_count") {
		t.Fatalf("output missing PAC metric: %q", output)
	}
	// pac-only should filter out non-PAC metrics
	if strings.Contains(output, "process_start_time_seconds") {
		t.Fatalf("output should not contain non-PAC metric with pac-only: %q", output)
	}
}

func TestRenderRawSnapshotTSV(t *testing.T) {
	output := renderRawSnapshot("controller", time.Unix(0, 0).UTC(), map[string]float64{
		"pac_controller_pipelines_as_code_pipelinerun_count": 3,
	}, metrics.SnapshotConfig{
		SortMode: metrics.SortByAlpha,
		Output:   "tsv",
	})

	if !strings.Contains(output, "# scope=controller view=raw") {
		t.Fatalf("output missing scope/view metadata: %q", output)
	}
	if !strings.Contains(output, "metric\tvalue") {
		t.Fatalf("output missing header: %q", output)
	}
	if !strings.Contains(output, "pac_controller_pipelines_as_code_pipelinerun_count\t3") {
		t.Fatalf("output missing metric row: %q", output)
	}
}

func TestRenderRawSnapshotFilter(t *testing.T) {
	output := renderRawSnapshot("all", time.Unix(0, 0).UTC(), map[string]float64{
		"pac_controller_pipelines_as_code_pipelinerun_count":    5,
		"pac_controller_pipelines_as_code_git_provider_api_req": 10,
	}, metrics.SnapshotConfig{
		Filter:   "pipelinerun",
		SortMode: metrics.SortByAlpha,
		Output:   "table",
	})

	if !strings.Contains(output, "pipelinerun_count") {
		t.Fatalf("output missing filtered metric: %q", output)
	}
	if strings.Contains(output, "git_provider_api_req") {
		t.Fatalf("output should not contain filtered-out metric: %q", output)
	}
}

func TestRepoFetchErrorPropagation(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	nextModel, _ := m.Update(repoStatusResultMsg{
		err: errors.New("forbidden: cannot list repositories"),
	})
	updated := mustModel(t, nextModel)
	if updated.reposErr == "" {
		t.Fatal("reposErr should be set when fetch fails")
	}
	if !strings.Contains(updated.reposErr, "forbidden") {
		t.Fatalf("reposErr = %q, want to contain 'forbidden'", updated.reposErr)
	}
	if updated.reposLoading {
		t.Fatal("reposLoading should be false after result")
	}
}

func TestRepoFetchSuccessClearsError(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.reposErr = "previous error"
	nextModel, _ := m.Update(repoStatusResultMsg{})
	updated := mustModel(t, nextModel)
	if updated.reposErr != "" {
		t.Fatalf("reposErr = %q, want empty after successful fetch", updated.reposErr)
	}
}

func TestEventFetchErrorPropagation(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	nextModel, _ := m.Update(eventsResultMsg{
		err: errors.New("namespace not found"),
	})
	updated := mustModel(t, nextModel)
	if updated.eventsErr == "" {
		t.Fatal("eventsErr should be set when fetch fails")
	}
	if !strings.Contains(updated.eventsErr, "namespace not found") {
		t.Fatalf("eventsErr = %q, want to contain 'namespace not found'", updated.eventsErr)
	}
	if updated.eventsLoading {
		t.Fatal("eventsLoading should be false after result")
	}
}

func TestEventFetchSuccessClearsError(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.eventsErr = "previous error"
	nextModel, _ := m.Update(eventsResultMsg{})
	updated := mustModel(t, nextModel)
	if updated.eventsErr != "" {
		t.Fatalf("eventsErr = %q, want empty after successful fetch", updated.eventsErr)
	}
}

func TestLabelToggle(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, metrics.SortByDelta, "", nil)
	m.viewMode = metrics.ViewDashboard

	// Press enter to toggle labels
	nextModel, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	updated := mustModel(t, nextModel)
	if !updated.showLabels {
		t.Fatal("showLabels = false, want true after enter")
	}

	// Press esc to dismiss
	nextModel, _ = updated.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	updated = mustModel(t, nextModel)
	if updated.showLabels {
		t.Fatal("showLabels = true, want false after esc")
	}
}
