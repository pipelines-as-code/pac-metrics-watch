package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/components"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
)

const (
	historySize          = 40
	sparkChars           = "▁▂▃▄▅▆▇█"
	defaultNamespace     = "pipelines-as-code"
	defaultInterval      = 5 * time.Second
	defaultSnapshotMode  = "table"
	filterInputHelp      = "filter: type to narrow raw metrics, enter to keep, esc to clear"
	minimumViewportWidth = 96
)

type sortMode string

const (
	sortByDelta sortMode = "delta"
	sortByAlpha sortMode = "alpha"
)

type viewMode string

const (
	viewDashboard viewMode = "dashboard"
	viewRaw       viewMode = "raw"
)



type scrapeFunc func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error)

type scrapeCycleResultMsg struct {
	id       int
	scope    int
	metrics  map[string]float64
	err      error
	duration time.Duration
}

type tickMsg time.Time

type endpointDef struct {
	name    string
	svcPath string
}

type scopeDef struct {
	name            string
	endpointIndexes []int
}

type metricRow struct {
	Name    string
	Value   float64
	Delta   float64
	History []float64
}

type signalDefinition struct {
	ID          string
	Title       string
	Kind        string
	Exact       []string
	Prefixes    []string
	Description string
	Why         string
}

type dashboardGroup struct {
	Title       string
	Description string
	Signals     []signalDefinition
}

type dashboardRow struct {
	Group       string
	Description string
	Signal      signalDefinition
	Value       float64
	Delta       float64
	History     []float64
	Sources     []string
	Available   bool
}

type snapshotConfig struct {
	kubeconfig string
	namespace  string
	scope      string
	pacOnly    bool
	filter     string
	sortMode   sortMode
	output     string
}

type model struct {
	kubeconfig string
	namespace  string
	interval   time.Duration
	pacOnly    bool
	endpoints  []endpointDef
	scopes     []scopeDef
	scraper    scrapeFunc

	activeScope  int
	history      map[string][]float64
	delta        map[string]float64
	keys         []string
	cursor       int
	visibleStart int
	lastUpdate   time.Time
	lastDuration time.Duration
	err          string
	width        int
	height       int
	scraping     bool
	scrapeSeq    int
	cancelScrape context.CancelFunc
	sortMode     sortMode
	filter       string
	filterInput  string
	filterMode   bool
	viewMode     viewMode
}

var dashboardGroups = []dashboardGroup{
	{
		Title:       "PAC Flow",
		Description: "Core business signals for webhook traffic and PipelineRun activity.",
		Signals: []signalDefinition{
			{
				ID:          "git-api-requests",
				Title:       "Git Provider API Requests",
				Kind:        "counter",
				Exact:       []string{"pipelines_as_code_git_provider_api_request_count"},
				Description: "Total API calls made by PAC to Git providers.",
				Why:         "Watch the delta for spikes caused by webhook bursts, retries, or inefficient polling.",
			},
			{
				ID:          "pipelineruns-created",
				Title:       "PipelineRuns Created",
				Kind:        "counter",
				Exact:       []string{"pipelines_as_code_pipelinerun_count"},
				Description: "PipelineRuns created by PAC.",
				Why:         "This is the clearest signal that PAC is accepting events and launching work.",
			},
			{
				ID:          "running-pipelineruns",
				Title:       "Running PipelineRuns",
				Kind:        "gauge",
				Exact:       []string{"pipelines_as_code_running_pipelineruns_count"},
				Description: "Current number of in-flight PipelineRuns.",
				Why:         "A sustained high value points to backlog, long-running builds, or stuck executions.",
			},
			{
				ID:          "pipelinerun-duration",
				Title:       "PipelineRun Duration Seconds",
				Kind:        "counter",
				Exact:       []string{"pipelines_as_code_pipelinerun_duration_seconds_sum"},
				Description: "Cumulative seconds spent by PAC-created PipelineRuns.",
				Why:         "The delta approximates how much run-time accumulated during the last refresh window.",
			},
		},
	},
	{
		Title:       "Queue Health",
		Description: "Controller queue behavior that usually explains why PAC feels slow or bursty.",
		Signals: []signalDefinition{
			{
				ID:          "workqueue-depth",
				Title:       "Workqueue Depth",
				Kind:        "gauge",
				Exact:       []string{"workqueue_depth"},
				Description: "Items waiting in controller workqueues.",
				Why:         "Depth above zero means the controller is lagging behind incoming work.",
			},
			{
				ID:          "workqueue-adds",
				Title:       "Workqueue Adds",
				Kind:        "counter",
				Exact:       []string{"workqueue_adds_total"},
				Description: "New items added to controller workqueues.",
				Why:         "A fast-growing delta means PAC is being fed work quickly, often from webhook traffic.",
			},
			{
				ID:          "workqueue-retries",
				Title:       "Workqueue Retries",
				Kind:        "counter",
				Exact:       []string{"workqueue_retries_total"},
				Description: "Retries issued by controller workqueues.",
				Why:         "Any sustained retry growth deserves investigation because reconciles are failing or being requeued.",
			},
			{
				ID:          "workqueue-queue-seconds",
				Title:       "Queue Wait Seconds",
				Kind:        "counter",
				Exact:       []string{"workqueue_queue_duration_seconds_sum"},
				Description: "Cumulative time spent waiting in the queue.",
				Why:         "If this delta rises faster than work throughput, PAC is falling behind.",
			},
		},
	},
	{
		Title:       "Reconcile Health",
		Description: "Signals from controller-runtime that help explain controller behavior.",
		Signals: []signalDefinition{
			{
				ID:          "reconcile-total",
				Title:       "Reconciles",
				Kind:        "counter",
				Exact:       []string{"controller_runtime_reconcile_total"},
				Description: "Total reconcile executions.",
				Why:         "Use the delta to see how active the reconciler is during webhook or status-reporting bursts.",
			},
			{
				ID:          "reconcile-errors",
				Title:       "Reconcile Errors",
				Kind:        "counter",
				Exact:       []string{"controller_runtime_reconcile_errors_total"},
				Description: "Reconcile executions that ended in error.",
				Why:         "This should stay near zero. Growth here often lines up with user-visible PAC failures.",
			},
			{
				ID:          "active-workers",
				Title:       "Active Workers",
				Kind:        "gauge",
				Exact:       []string{"controller_runtime_active_workers"},
				Description: "Current active reconcile workers.",
				Why:         "A low worker count with growing queue depth means the controller is under-provisioned or blocked.",
			},
			{
				ID:          "workqueue-work-seconds",
				Title:       "Work Seconds",
				Kind:        "counter",
				Exact:       []string{"workqueue_work_duration_seconds_sum"},
				Description: "Cumulative active work duration in queues.",
				Why:         "This helps distinguish time spent processing from time spent merely waiting in the queue.",
			},
		},
	},
}

func buildEndpoints(namespace string) []endpointDef {
	base := fmt.Sprintf("/api/v1/namespaces/%s/services", namespace)
	return []endpointDef{
		{name: "controller", svcPath: fmt.Sprintf("%s/pipelines-as-code-controller:9090/proxy/metrics", base)},
		{name: "watcher", svcPath: fmt.Sprintf("%s/pipelines-as-code-watcher:9090/proxy/metrics", base)},
	}
}

func buildScopes(endpoints []endpointDef) []scopeDef {
	return []scopeDef{
		{name: "all", endpointIndexes: []int{0, 1}},
		{name: "controller", endpointIndexes: []int{0}},
		{name: "watcher", endpointIndexes: []int{1}},
	}
}

func normalizeSortMode(raw string) (sortMode, error) {
	switch sortMode(strings.ToLower(strings.TrimSpace(raw))) {
	case "", sortByDelta:
		return sortByDelta, nil
	case sortByAlpha:
		return sortByAlpha, nil
	default:
		return "", fmt.Errorf("unsupported sort mode %q", raw)
	}
}

func normalizeOutputMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", defaultSnapshotMode:
		return defaultSnapshotMode, nil
	case "tsv":
		return "tsv", nil
	default:
		return "", fmt.Errorf("unsupported output mode %q", raw)
	}
}

func scopeIndex(scopes []scopeDef, name string) (int, error) {
	needle := strings.ToLower(strings.TrimSpace(name))
	for i, scope := range scopes {
		if scope.name == needle {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unsupported endpoint %q", name)
}

func scrapeMetrics(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
	args := []string{"get", "--raw", svcPath}
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}

	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output == "" {
			return nil, fmt.Errorf("kubectl: %w", err)
		}
		return nil, fmt.Errorf("kubectl: %s: %w", output, err)
	}

	return parseMetrics(string(out))
}

func parseMetrics(data string) (map[string]float64, error) {
	result := map[string]float64{}
	scanner := bufio.NewScanner(strings.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		val, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue
		}

		name := parts[0]
		if idx := strings.Index(name, "{"); idx >= 0 {
			name = name[:idx]
		}
		result[name] += val
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan metrics: %w", err)
	}

	return result, nil
}

func interestingMetric(name string) bool {
	for _, prefix := range []string{
		"pac_",
		"workqueue_",
		"reconciler_",
		"controller_",
		"controller_runtime_",
		"tekton_",
		"grpc_",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func canonicalMetricName(name string) string {
	for _, prefix := range []string{"pac_controller_", "pac_watcher_"} {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

func matchesSignal(name string, signal signalDefinition) bool {
	canonical := canonicalMetricName(name)
	for _, exact := range signal.Exact {
		if canonical == exact {
			return true
		}
	}
	for _, prefix := range signal.Prefixes {
		if strings.HasPrefix(canonical, prefix) {
			return true
		}
	}
	return false
}

func sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}

	minValue, maxValue := values[0], values[0]
	for _, value := range values {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}

	chars := []rune(sparkChars)
	maxIndex := float64(len(chars) - 1)
	var builder strings.Builder
	for _, value := range values {
		index := 0
		if maxValue > minValue {
			index = int(math.Round((value - minValue) / (maxValue - minValue) * maxIndex))
		}
		if index < 0 {
			index = 0
		}
		if index >= len(chars) {
			index = len(chars) - 1
		}
		builder.WriteRune(chars[index])
	}
	return builder.String()
}

func formatMetricNumber(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%g", value)
	}
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.3f", value)
}

func formatDelta(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%+g", value)
	}
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return fmt.Sprintf("%+.0f", value)
	}
	return fmt.Sprintf("%+.3f", value)
}

func metricAllowed(name string, pacOnly bool, filter string) bool {
	if pacOnly {
		if !strings.HasPrefix(canonicalMetricName(name), "pipelines_as_code_") {
			return false
		}
	} else if !interestingMetric(name) {
		return false
	}

	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(filter))
}

func sortRows(rows []metricRow, mode sortMode, byDelta bool) {
	switch mode {
	case sortByAlpha:
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Name < rows[j].Name
		})
	default:
		sort.Slice(rows, func(i, j int) bool {
			left := rows[i].Value
			right := rows[j].Value
			if byDelta {
				left = math.Abs(rows[i].Delta)
				right = math.Abs(rows[j].Delta)
			}
			if left == right {
				return rows[i].Name < rows[j].Name
			}
			return left > right
		})
	}
}

func buildRowsFromHistory(history map[string][]float64, delta map[string]float64, pacOnly bool, filter string, mode sortMode) []metricRow {
	rows := make([]metricRow, 0, len(history))
	for name, hist := range history {
		if !metricAllowed(name, pacOnly, filter) {
			continue
		}
		value := 0.0
		if len(hist) > 0 {
			value = hist[len(hist)-1]
		}
		rows = append(rows, metricRow{
			Name:    name,
			Value:   value,
			Delta:   delta[name],
			History: hist,
		})
	}
	sortRows(rows, mode, true)
	return rows
}

func aggregateHistories(history map[string][]float64, names []string) []float64 {
	maxLen := 0
	for _, name := range names {
		if len(history[name]) > maxLen {
			maxLen = len(history[name])
		}
	}
	if maxLen == 0 {
		return nil
	}

	aggregated := make([]float64, maxLen)
	for _, name := range names {
		hist := history[name]
		offset := maxLen - len(hist)
		for i, value := range hist {
			aggregated[offset+i] += value
		}
	}
	return aggregated
}

func buildDashboardRows(history map[string][]float64, delta map[string]float64) []dashboardRow {
	rows := make([]dashboardRow, 0, 12)
	for _, group := range dashboardGroups {
		for _, signal := range group.Signals {
			sources := make([]string, 0, 2)
			totalDelta := 0.0
			for name := range history {
				if matchesSignal(name, signal) {
					sources = append(sources, name)
					totalDelta += delta[name]
				}
			}
			sort.Strings(sources)

			row := dashboardRow{
				Group:       group.Title,
				Description: group.Description,
				Signal:      signal,
				Delta:       totalDelta,
				Sources:     sources,
				Available:   len(sources) > 0,
			}
			if row.Available {
				row.History = aggregateHistories(history, sources)
				if len(row.History) > 0 {
					row.Value = row.History[len(row.History)-1]
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func signalRowsFromMetrics(metrics map[string]float64) []dashboardRow {
	history := make(map[string][]float64, len(metrics))
	for name, value := range metrics {
		history[name] = []float64{value}
	}
	return buildDashboardRows(history, map[string]float64{})
}

func renderSnapshot(scope string, collectedAt time.Time, metrics map[string]float64, output string) string {
	var builder strings.Builder
	if output == "tsv" {
		rows := make([]metricRow, 0, len(metrics))
		for name, value := range metrics {
			rows = append(rows, metricRow{Name: name, Value: value})
		}
		sortRows(rows, sortByAlpha, false)

		fmt.Fprintf(&builder, "# scope=%s\n", scope)
		fmt.Fprintf(&builder, "# timestamp=%s\n", collectedAt.Format(time.RFC3339))
		builder.WriteString("metric\tvalue\n")
		for _, row := range rows {
			fmt.Fprintf(&builder, "%s\t%s\n", row.Name, formatMetricNumber(row.Value))
		}
		return builder.String()
	}

	dashboardRows := signalRowsFromMetrics(metrics)
	tabWriter := tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tabWriter, "scope:\t%s\n", scope)
	_, _ = fmt.Fprintf(tabWriter, "timestamp:\t%s\n", collectedAt.Format(time.RFC3339))
	_ = tabWriter.Flush()
	builder.WriteString("\n")

	lastGroup := ""
	tabWriter = tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tabWriter, "GROUP\tSIGNAL\tVALUE\tWHY IT MATTERS")
	for _, row := range dashboardRows {
		if row.Group != lastGroup {
			lastGroup = row.Group
		}
		value := "n/a"
		if row.Available {
			value = formatMetricNumber(row.Value)
		}
		_, _ = fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\n", row.Group, row.Signal.Title, value, row.Signal.Why)
	}
	_ = tabWriter.Flush()
	return builder.String()
}

func runSnapshot(config snapshotConfig, scraper scrapeFunc) (string, error) {
	endpoints := buildEndpoints(config.namespace)
	scopes := buildScopes(endpoints)
	index, err := scopeIndex(scopes, config.scope)
	if err != nil {
		return "", err
	}

	scope := scopes[index]
	metrics := map[string]float64{}
	for _, endpointIndex := range scope.endpointIndexes {
		endpoint := endpoints[endpointIndex]
		scrapedMetrics, err := scraper(context.Background(), config.kubeconfig, endpoint.svcPath)
		if err != nil {
			return "", fmt.Errorf("%s: %w", endpoint.name, err)
		}
		for name, value := range scrapedMetrics {
			metrics[name] += value
		}
	}

	return renderSnapshot(scope.name, time.Now(), metrics, config.output), nil
}

func initialModel(kubeconfig, namespace string, interval time.Duration, pacOnly bool, initialScope int, mode sortMode, filter string, scraper scrapeFunc) *model {
	endpoints := buildEndpoints(namespace)
	if scraper == nil {
		scraper = scrapeMetrics
	}

	return &model{
		kubeconfig:  kubeconfig,
		namespace:   namespace,
		interval:    interval,
		pacOnly:     pacOnly,
		endpoints:   endpoints,
		scopes:      buildScopes(endpoints),
		scraper:     scraper,
		activeScope: initialScope,
		history:     map[string][]float64{},
		delta:       map[string]float64{},
		sortMode:    mode,
		filter:      filter,
		filterInput: filter,
		viewMode:    viewDashboard,
	}
}

func doTick(delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(now time.Time) tea.Msg {
		return tickMsg(now)
	})
}

func (m *model) resetMetrics() {
	m.history = map[string][]float64{}
	m.delta = map[string]float64{}
	m.keys = nil
	m.cursor = 0
	m.visibleStart = 0
	m.lastUpdate = time.Time{}
	m.lastDuration = 0
	m.err = ""
}

func (m *model) recomputeRows() {
	rows := buildRowsFromHistory(m.history, m.delta, m.pacOnly, m.filter, m.sortMode)
	m.keys = make([]string, 0, len(rows))
	for _, row := range rows {
		m.keys = append(m.keys, row.Name)
	}

	m.clampCursor()
}

func (m *model) currentScope() scopeDef {
	return m.scopes[m.activeScope]
}

func (m *model) beginScrape() tea.Cmd {
	if m.scraping {
		return nil
	}

	scopeIndex := m.activeScope
	scope := m.currentScope()
	ctx, cancel := context.WithCancel(context.Background())
	m.scrapeSeq++
	id := m.scrapeSeq
	m.cancelScrape = cancel
	m.scraping = true

	return func() tea.Msg {
		startedAt := time.Now()
		mergedMetrics := map[string]float64{}
		var scrapeErrors []string
		for _, endpointIndex := range scope.endpointIndexes {
			endpoint := m.endpoints[endpointIndex]
			metrics, err := m.scraper(ctx, m.kubeconfig, endpoint.svcPath)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					return scrapeCycleResultMsg{
						id:       id,
						scope:    scopeIndex,
						err:      context.Canceled,
						duration: time.Since(startedAt),
					}
				}
				scrapeErrors = append(scrapeErrors, fmt.Sprintf("%s: %v", endpoint.name, err))
				continue
			}
			for name, value := range metrics {
				mergedMetrics[name] += value
			}
		}

		var scrapeErr error
		if len(scrapeErrors) > 0 {
			scrapeErr = errors.New(strings.Join(scrapeErrors, " | "))
		}

		return scrapeCycleResultMsg{
			id:       id,
			scope:    scopeIndex,
			metrics:  mergedMetrics,
			err:      scrapeErr,
			duration: time.Since(startedAt),
		}
	}
}

func (m *model) cancelActiveScrape() {
	if m.cancelScrape != nil {
		m.cancelScrape()
		m.cancelScrape = nil
	}
	m.scraping = false
}

func (m *model) applyMetrics(metrics map[string]float64) {
	seen := make(map[string]struct{}, len(metrics))
	for name, value := range metrics {
		prev := 0.0
		if hist := m.history[name]; len(hist) > 0 {
			prev = hist[len(hist)-1]
		}

		hist := append(m.history[name], value)
		if len(hist) > historySize {
			hist = hist[len(hist)-historySize:]
		}

		m.history[name] = hist
		m.delta[name] = value - prev
		seen[name] = struct{}{}
	}

	for name := range m.history {
		if _, ok := seen[name]; ok {
			continue
		}
		delete(m.history, name)
		delete(m.delta, name)
	}

	m.recomputeRows()
}

func (m *model) maxVisibleRows() int {
	maxRows := m.height - 14
	if maxRows < 1 {
		maxRows = 20
	}
	return maxRows
}

func (m *model) activeLen() int {
	if m.viewMode == viewDashboard {
		return len(buildDashboardRows(m.history, m.delta))
	}
	return len(m.keys)
}

func (m *model) clampCursor() {
	activeLen := m.activeLen()
	if activeLen == 0 {
		m.cursor = 0
		m.visibleStart = 0
		return
	}
	if m.cursor >= activeLen {
		m.cursor = activeLen - 1
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.visibleStart > m.cursor {
		m.visibleStart = m.cursor
	}
}

func (m *model) ensureCursorVisible() {
	activeLen := m.activeLen()
	if activeLen == 0 {
		m.visibleStart = 0
		return
	}

	maxRows := m.maxVisibleRows()
	if m.cursor < m.visibleStart {
		m.visibleStart = m.cursor
	}
	if m.cursor >= m.visibleStart+maxRows {
		m.visibleStart = m.cursor - maxRows + 1
	}
	if m.visibleStart < 0 {
		m.visibleStart = 0
	}
}
func (m *model) handleFilterKey(msg tea.KeyPressMsg) {
	switch msg.String() {
	case "esc":
		m.filterMode = false
		m.filter = ""
		m.filterInput = ""
		m.recomputeRows()
	case "enter":
		m.filterMode = false
		m.filter = strings.TrimSpace(m.filterInput)
		m.recomputeRows()
	case "backspace":
		if len(m.filterInput) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.filterInput)
			m.filterInput = m.filterInput[:len(m.filterInput)-size]
			m.filter = strings.TrimSpace(m.filterInput)
			m.recomputeRows()
		}
	default:
		key := msg.Key()
		if key.Text != "" {
			m.filterInput += key.Text
			m.filter = strings.TrimSpace(m.filterInput)
			m.recomputeRows()
		}
	}
}

func (m *model) selectedRawRow() (metricRow, bool) {
	if len(m.keys) == 0 || m.cursor < 0 || m.cursor >= len(m.keys) {
		return metricRow{}, false
	}
	name := m.keys[m.cursor]
	hist := m.history[name]
	value := 0.0
	if len(hist) > 0 {
		value = hist[len(hist)-1]
	}
	return metricRow{
		Name:    name,
		Value:   value,
		Delta:   m.delta[name],
		History: hist,
	}, true
}

func (m *model) selectedDashboardRow() (dashboardRow, bool) {
	rows := buildDashboardRows(m.history, m.delta)
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return dashboardRow{}, false
	}
	return rows[m.cursor], true
}

func (m *model) summarySignals() []components.CardData {
	rows := buildDashboardRows(m.history, m.delta)
	ids := []string{"git-api-requests", "pipelineruns-created", "running-pipelineruns", "workqueue-depth"}
	byID := make(map[string]dashboardRow, len(rows))
	for _, row := range rows {
		byID[row.Signal.ID] = row
	}
	result := make([]components.CardData, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
            trend := ""
            if row.Available {
                trend = sparkline(row.History)
            }
			result = append(result, components.CardData{
                Title: row.Signal.Title,
                Value: formatMetricNumber(row.Value),
                Delta: formatDelta(row.Delta),
                Trend: trend,
                Available: row.Available,
            })
		}
	}
	return result
}

func (m *model) Init() tea.Cmd {
	return m.beginScrape()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ensureCursorVisible()
		return m, nil

	case tickMsg:
		if m.scraping {
			return m, nil
		}
		return m, m.beginScrape()

	case scrapeCycleResultMsg:
		if msg.id != m.scrapeSeq || msg.scope != m.activeScope {
			return m, nil
		}

		m.scraping = false
		m.cancelScrape = nil
		m.lastDuration = msg.duration

		if msg.err != nil && errors.Is(msg.err, context.Canceled) {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
		}
		if len(msg.metrics) > 0 {
			m.lastUpdate = time.Now()
			m.applyMetrics(msg.metrics)
		}
		return m, doTick(m.interval)

	case tea.KeyPressMsg:
		if m.filterMode {
			m.handleFilterKey(msg)
			m.ensureCursorVisible()
			return m, nil
		}

		switch msg.String() {
		case "q", "ctrl+c":
			m.cancelActiveScrape()
			return m, tea.Quit
		case "f":
			m.pacOnly = !m.pacOnly
			m.recomputeRows()
		case "s":
			if m.sortMode == sortByDelta {
				m.sortMode = sortByAlpha
			} else {
				m.sortMode = sortByDelta
			}
			m.recomputeRows()
		case "d":
			m.viewMode = viewDashboard
			m.cursor = 0
			m.visibleStart = 0
		case "r":
			m.viewMode = viewRaw
			m.cursor = 0
			m.visibleStart = 0
			m.recomputeRows()
		case "/":
			if m.viewMode == viewRaw {
				m.filterMode = true
				m.filterInput = m.filter
			}
		case "tab":
			m.cancelActiveScrape()
			m.activeScope = (m.activeScope + 1) % len(m.scopes)
			m.resetMetrics()
			return m, m.beginScrape()
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < m.activeLen()-1 {
				m.cursor++
			}
		}

		m.clampCursor()
		m.ensureCursorVisible()
		return m, nil
	}

	return m, nil
}


func (m *model) renderDashboardRows(width int) ([]string, string) {
	rows := buildDashboardRows(m.history, m.delta)
	maxRows := m.maxVisibleRows()
	m.ensureCursorVisible()

	columns := []components.Column{
		{Title: "SIGNAL", Width: 22},
		{Title: "VALUE", Width: 8, AlignRight: true},
		{Title: "DELTA", Width: 6, AlignRight: true},
		{Title: "TREND", Width: 12},
		{Title: "WHAT TO WATCH", Width: max(20, width-52)},
	}

	var tableRows []components.TableRow
	lastGroup := ""

	// Keep track of logical row index for cursor (ignoring group headers)
	logicalIdx := 0

	for i, row := range rows {
		if i < m.visibleStart || i >= m.visibleStart+maxRows {
			continue
		}
		if row.Group != lastGroup {
			tableRows = append(tableRows, components.TableRow{
				IsGroup:    true,
				GroupTitle: row.Group + "  " + row.Description,
			})
			lastGroup = row.Group
		}

		value := "n/a"
		delta := "n/a"
		graph := ""
		rowStyle := theme.StyleUnavailable
		if row.Available {
			value = formatMetricNumber(row.Value)
			delta = formatDelta(row.Delta)
			graph = sparkline(row.History)
			graphRunes := []rune(graph)
			if len(graphRunes) > 12 {
				graph = string(graphRunes[len(graphRunes)-12:])
			}
			if row.Delta > 0 {
				rowStyle = theme.StyleIncr
			} else {
				rowStyle = theme.StyleNormal
			}
		}

		tableRows = append(tableRows, components.TableRow{
			Columns: []string{row.Signal.Title, value, delta, graph, row.Signal.Why},
			Style:   rowStyle,
		})
		logicalIdx++
	}

	// Calculate correct cursor for table (accounting for group rows)
	rendered := components.RenderTable(columns, tableRows, m.cursor - m.visibleStart)

	detail := theme.StyleDim.Render("selected: <none>")
	if row, ok := m.selectedDashboardRow(); ok {
		sources := "<none>"
		if len(row.Sources) > 0 {
			sources = strings.Join(row.Sources, ", ")
		}

		valStr := "n/a"
		deltaStr := "n/a"
		if row.Available {
			valStr = formatMetricNumber(row.Value)
			deltaStr = formatDelta(row.Delta)
		}

		detail = components.RenderDetailPane(
			row.Signal.Title,
			row.Signal.Kind,
			valStr,
			deltaStr,
			row.Signal.Description,
			sources,
			row.History,
			width,
		)
	}

	return rendered, detail
}

func (m *model) renderRawRows(width int) ([]string, string) {
	maxRows := m.maxVisibleRows()
	m.ensureCursorVisible()

	columns := []components.Column{
		{Title: "RAW METRIC", Width: 48},
		{Title: "VALUE", Width: 12, AlignRight: true},
		{Title: "DELTA", Width: 12, AlignRight: true},
		{Title: "GRAPH", Width: 20},
	}

	var tableRows []components.TableRow

	for i, name := range m.keys {
		if i < m.visibleStart || i >= m.visibleStart+maxRows {
			continue
		}
		hist := m.history[name]
		value := 0.0
		if len(hist) > 0 {
			value = hist[len(hist)-1]
		}

		graph := sparkline(hist)
		graphRunes := []rune(graph)
		if len(graphRunes) > 20 {
			graph = string(graphRunes[len(graphRunes)-20:])
		}

		rowStyle := theme.StyleNormal
		if m.delta[name] > 0 {
			rowStyle = theme.StyleIncr
		}

		tableRows = append(tableRows, components.TableRow{
			Columns: []string{name, formatMetricNumber(value), formatDelta(m.delta[name]), graph},
			Style:   rowStyle,
		})
	}

	var rendered []string
	if len(m.keys) == 0 {
		message := "no raw metrics matched the current scope and filter"
		if m.scraping && len(m.history) == 0 {
			message = "scraping metrics..."
		}
		rendered = append(rendered, theme.StyleDim.Render(message))
	} else {
		rendered = components.RenderTable(columns, tableRows, m.cursor - m.visibleStart)
	}

	detail := theme.StyleDim.Render("selected: <none>")
	if row, ok := m.selectedRawRow(); ok {
		detail = components.RenderDetailPane(
			row.Name,
			"raw",
			formatMetricNumber(row.Value),
			formatDelta(row.Delta),
			fmt.Sprintf("samples=%d canonical=%s", len(row.History), canonicalMetricName(row.Name)),
			"kubectl",
			row.History,
			width,
		)
	}
	return rendered, detail
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (m *model) View() tea.View {
	width := m.width
	if width < minimumViewportWidth {
		width = minimumViewportWidth
	}

	filterLabel := m.filter
	if filterLabel == "" {
		filterLabel = "<none>"
	}

	scopeStrs := make([]string, len(m.scopes))
	for i, s := range m.scopes {
		scopeStrs[i] = s.name
	}

	header := components.RenderHeader(
		"PAC Metrics Dashboard",
		scopeStrs,
		m.activeScope,
		string(m.viewMode),
		string(m.sortMode),
		filterLabel,
		m.lastUpdate,
		m.lastDuration,
		m.scraping,
	)

	separator := theme.StyleSep.Render(strings.Repeat("─", width))
	summary := components.RenderSummaryCards(m.summarySignals(), width)

	var rows []string
	var detail string
	if m.viewMode == viewDashboard {
		rows, detail = m.renderDashboardRows(width)
	} else {
		rows, detail = m.renderRawRows(width)
	}

	footer := components.RenderFooter(m.err, m.filterMode, m.filterInput, width)

	parts := []string{header, separator, summary, separator}
	parts = append(parts, rows...)
	parts = append(parts, separator, detail, footer)

	view := tea.NewView(strings.Join(parts, "\n"))
	view.AltScreen = true
	return view
}

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (default: $KUBECONFIG or in-cluster)")
	namespace := flag.String("namespace", defaultNamespace, "namespace where PAC is running")
	interval := flag.Duration("interval", defaultInterval, "polling interval")
	pacOnly := flag.Bool("pac-only", false, "show only pac_ prefixed metrics in raw mode")
	once := flag.Bool("once", false, "scrape once, print a report, and exit")
	scopeFlag := flag.String("endpoint", "all", "scope to use: all, controller, or watcher")
	sortFlag := flag.String("sort", string(sortByDelta), "sort order for raw mode: delta or alpha")
	filter := flag.String("filter", "", "substring filter for raw metric names")
	output := flag.String("output", defaultSnapshotMode, "snapshot output format: table or tsv")
	flag.Parse()

	if *kubeconfig == "" {
		*kubeconfig = os.Getenv("KUBECONFIG")
	}

	sortModeValue, err := normalizeSortMode(*sortFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	outputMode, err := normalizeOutputMode(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *once {
		report, err := runSnapshot(snapshotConfig{
			kubeconfig: *kubeconfig,
			namespace:  *namespace,
			scope:      *scopeFlag,
			pacOnly:    *pacOnly,
			filter:     *filter,
			sortMode:   sortModeValue,
			output:     outputMode,
		}, scrapeMetrics)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(report)
		return
	}

	scopeValue, err := scopeIndex(buildScopes(buildEndpoints(*namespace)), *scopeFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	program := tea.NewProgram(initialModel(
		*kubeconfig,
		*namespace,
		*interval,
		*pacOnly,
		scopeValue,
		sortModeValue,
		*filter,
		scrapeMetrics,
	))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
