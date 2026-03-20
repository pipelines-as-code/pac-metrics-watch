package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestParseMetricsAggregatesMetricFamilies(t *testing.T) {
	metrics, err := parseMetrics(`
# HELP pac_events_total Number of PAC events
pac_events_total{provider="github"} 2
pac_events_total{provider="gitlab"} 3
workqueue_depth{name="foo"} 4
`)
	if err != nil {
		t.Fatalf("parseMetrics returned error: %v", err)
	}

	if got, want := metrics["pac_events_total"], 5.0; got != want {
		t.Fatalf("pac_events_total = %v, want %v", got, want)
	}
	if got, want := metrics["workqueue_depth"], 4.0; got != want {
		t.Fatalf("workqueue_depth = %v, want %v", got, want)
	}
}

func TestBuildEndpointsUsesMetricsPort(t *testing.T) {
	endpoints := buildEndpoints(defaultNamespace)
	if len(endpoints) != 2 {
		t.Fatalf("len(endpoints) = %d, want 2", len(endpoints))
	}

	if got := endpoints[0].svcPath; got != "/api/v1/namespaces/pipelines-as-code/services/pipelines-as-code-controller:9090/proxy/metrics" {
		t.Fatalf("controller svcPath = %q", got)
	}
	if got := endpoints[1].svcPath; got != "/api/v1/namespaces/pipelines-as-code/services/pipelines-as-code-watcher:9090/proxy/metrics" {
		t.Fatalf("watcher svcPath = %q", got)
	}
}

func TestBuildScopes(t *testing.T) {
	scopes := buildScopes(buildEndpoints(defaultNamespace))
	if got := scopes[0].name; got != "all" {
		t.Fatalf("scopes[0].name = %q, want all", got)
	}
	if len(scopes[0].endpointIndexes) != 2 {
		t.Fatalf("len(scopes[0].endpointIndexes) = %d, want 2", len(scopes[0].endpointIndexes))
	}
}

func TestCanonicalMetricName(t *testing.T) {
	if got := canonicalMetricName("pac_controller_pipelines_as_code_git_provider_api_request_count"); got != "pipelines_as_code_git_provider_api_request_count" {
		t.Fatalf("canonicalMetricName(controller) = %q", got)
	}
	if got := canonicalMetricName("pac_watcher_workqueue_depth"); got != "workqueue_depth" {
		t.Fatalf("canonicalMetricName(watcher) = %q", got)
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

	rows := buildDashboardRows(history, delta)
	var apiRow dashboardRow
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
	if _, err := parseMetrics(line); err == nil {
		t.Fatal("parseMetrics returned nil error for oversized token")
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

	rows := buildRowsFromHistory(history, delta, false, "queue", sortByDelta)
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1", len(rows))
	}
	if rows[0].Name != "workqueue_queue_duration" {
		t.Fatalf("rows[0].Name = %q, want workqueue_queue_duration", rows[0].Name)
	}

	rows = buildRowsFromHistory(history, delta, false, "", sortByDelta)
	if rows[0].Name != "pac_active_repositories" {
		t.Fatalf("rows sorted by delta = %q, want pac_active_repositories", rows[0].Name)
	}
}

func TestModelInitStartsImmediateScrape(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, sortByDelta, "", func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
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
	m := initialModel("", defaultNamespace, time.Second, false, 0, sortByDelta, "", nil)
	m.scraping = true
	m.scrapeSeq = 9

	nextModel, cmd := m.Update(tickMsg(time.Now()))
	if cmd != nil {
		t.Fatal("tick during active scrape returned a command")
	}
	updated := nextModel.(*model)
	if updated.scrapeSeq != 9 {
		t.Fatalf("scrapeSeq = %d, want 9", updated.scrapeSeq)
	}
}

func TestScrapeResultSchedulesNextTickAndUpdatesMetrics(t *testing.T) {
	m := initialModel("", defaultNamespace, time.Second, false, 0, sortByDelta, "", nil)
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

	updated := nextModel.(*model)
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
	m := initialModel("", defaultNamespace, time.Second, false, 0, sortByDelta, "", func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
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

	updated := nextModel.(*model)
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
	m := initialModel("", defaultNamespace, time.Second, false, 0, sortByDelta, "", nil)
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
	updated := nextModel.(*model)
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
	_, err := runSnapshot(snapshotConfig{
		namespace: defaultNamespace,
		scope:     "controller",
		output:    "table",
		sortMode:  sortByAlpha,
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
