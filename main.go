package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/kubectl"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/metrics"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/components"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/theme"
	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/ui/views"
)

const (
	historySize          = 120
	defaultNamespace     = "pipelines-as-code"
	defaultInterval      = 5 * time.Second
	defaultSnapshotMode  = "table"
	minimumViewportWidth = 96
	snapshotTimeout      = 15 * time.Second
)

// Message types for async operations.
type scrapeCycleResultMsg struct {
	id              int
	scope           int
	metrics         map[string]float64
	rawData         map[string]string // endpoint name -> raw metrics text
	resourceMetrics map[string]float64
	containerUsages []kubectl.ContainerUsage
	err             error
	resourceErr     string
	duration        time.Duration
}

type tickMsg time.Time

type healthCheckResultMsg struct {
	checks []views.HealthCheck
}

type repoStatusResultMsg struct {
	repos []kubectl.RepositoryStatus
	err   error
}

type eventsResultMsg struct {
	events []kubectl.K8sEvent
	err    error
}

type model struct {
	kubeconfig string
	namespace  string
	interval   time.Duration
	pacOnly    bool
	endpoints  []metrics.EndpointDef
	scopes     []metrics.ScopeDef
	scraper    metrics.ScrapeFunc

	activeScope   int
	history       map[string][]float64
	delta         map[string]float64
	keys          []string
	cursor        int
	visibleStart  int
	lastUpdate    time.Time
	lastDuration  time.Duration
	err           string
	width         int
	height        int
	scraping      bool
	scrapeSeq     int
	cancelScrape  context.CancelFunc
	sortMode      metrics.SortMode
	filter        string
	filterInput   string
	filterMode    bool
	viewMode      metrics.ViewMode
	resourceFocus views.ResourceFocus

	// Label breakdown state
	labeledSnapshot map[string]*metrics.MetricFamily
	showLabels      bool

	// Component resource state
	resourceHistory map[string][]float64
	resourceDelta   map[string]float64
	containerUsages []kubectl.ContainerUsage
	resourceWarning string

	// Health check state
	healthChecks  []views.HealthCheck
	healthLoading bool

	// Repository status state
	repoStatuses []kubectl.RepositoryStatus
	reposLoading bool
	reposErr     string

	// Events state
	events        []kubectl.K8sEvent
	eventsLoading bool
	eventsErr     string
}

func initialModel(kubeconfig, namespace string, interval time.Duration, pacOnly bool, initialScope int, mode metrics.SortMode, filter string, scraper metrics.ScrapeFunc) *model {
	endpoints := metrics.BuildEndpoints(namespace)

	return &model{
		kubeconfig:      kubeconfig,
		namespace:       namespace,
		interval:        interval,
		pacOnly:         pacOnly,
		endpoints:       endpoints,
		scopes:          metrics.BuildScopes(endpoints),
		scraper:         scraper,
		activeScope:     initialScope,
		history:         map[string][]float64{},
		delta:           map[string]float64{},
		resourceHistory: map[string][]float64{},
		resourceDelta:   map[string]float64{},
		sortMode:        mode,
		filter:          filter,
		filterInput:     filter,
		viewMode:        metrics.ViewDashboard,
		resourceFocus:   views.ResourceFocusMemory,
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
	m.labeledSnapshot = nil
	m.resourceHistory = map[string][]float64{}
	m.resourceDelta = map[string]float64{}
	m.containerUsages = nil
	m.resourceWarning = ""
}

func (m *model) recomputeRows() {
	rows := metrics.BuildRowsFromHistory(m.history, m.delta, m.pacOnly, m.filter, m.sortMode)
	m.keys = make([]string, 0, len(rows))
	for _, row := range rows {
		m.keys = append(m.keys, row.Name)
	}
	m.clampCursor()
}

func (m *model) currentScope() metrics.ScopeDef {
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

	kubeconfig := m.kubeconfig
	namespace := m.namespace
	scraper := m.scraper
	endpointsCopy := m.endpoints

	useRawScrape := scraper == nil
	return func() tea.Msg {
		startedAt := time.Now()
		mergedMetrics := map[string]float64{}
		rawData := map[string]string{}
		resourceMetrics := map[string]float64{}
		var scrapeErrors []string
		for _, endpointIndex := range scope.EndpointIndexes {
			endpoint := endpointsCopy[endpointIndex]

			if useRawScrape {
				// Scrape raw text once and parse both ways
				raw, rawErr := kubectl.ScrapeRawMetrics(ctx, kubeconfig, endpoint.SvcPath)
				if rawErr != nil {
					if errors.Is(rawErr, context.Canceled) {
						return scrapeCycleResultMsg{
							id:       id,
							scope:    scopeIndex,
							err:      context.Canceled,
							duration: time.Since(startedAt),
						}
					}
					scrapeErrors = append(scrapeErrors, fmt.Sprintf("%s: %v", endpoint.Name, rawErr))
					continue
				}

				ms, parseErr := metrics.ParseMetrics(raw)
				if parseErr != nil {
					scrapeErrors = append(scrapeErrors, fmt.Sprintf("%s: %v", endpoint.Name, parseErr))
					continue
				}
				mergeEndpointMetrics(mergedMetrics, endpoint.Name, ms)
				rawData[endpoint.Name] = raw
			} else {
				ms, err := scraper(ctx, kubeconfig, endpoint.SvcPath)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return scrapeCycleResultMsg{
							id:       id,
							scope:    scopeIndex,
							err:      context.Canceled,
							duration: time.Since(startedAt),
						}
					}
					scrapeErrors = append(scrapeErrors, fmt.Sprintf("%s: %v", endpoint.Name, err))
					continue
				}
				mergeEndpointMetrics(mergedMetrics, endpoint.Name, ms)
			}
		}

		var scrapeErr error
		if len(scrapeErrors) > 0 {
			scrapeErr = errors.New(strings.Join(scrapeErrors, " | "))
		}

		containerUsages, resourceErr := kubectl.GetPACContainerUsages(ctx, kubeconfig, namespace)
		if resourceErr == nil {
			for name, value := range aggregateContainerUsages(containerUsages) {
				resourceMetrics[name] = value
			}
		}

		return scrapeCycleResultMsg{
			id:              id,
			scope:           scopeIndex,
			metrics:         mergedMetrics,
			rawData:         rawData,
			resourceMetrics: resourceMetrics,
			containerUsages: containerUsages,
			err:             scrapeErr,
			resourceErr:     errorString(resourceErr),
			duration:        time.Since(startedAt),
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

func (m *model) applyMetrics(msg scrapeCycleResultMsg) {
	seen := make(map[string]struct{}, len(msg.metrics))
	for name, value := range msg.metrics {
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

	// Build labeled snapshot from raw data
	if len(msg.rawData) > 0 {
		labeled := map[string]*metrics.MetricFamily{}
		for _, raw := range msg.rawData {
			parsed, err := metrics.ParseWithLabels(raw)
			if err != nil {
				continue
			}
			for name, family := range parsed {
				if existing, ok := labeled[name]; ok {
					existing.Samples = append(existing.Samples, family.Samples...)
					existing.Total += family.Total
				} else {
					labeled[name] = family
				}
			}
		}
		m.labeledSnapshot = labeled
	}

	m.recomputeRows()
}

func (m *model) applyResourceMetrics(samples map[string]float64, usages []kubectl.ContainerUsage) {
	seen := make(map[string]struct{}, len(samples))
	for name, value := range samples {
		prev := 0.0
		if hist := m.resourceHistory[name]; len(hist) > 0 {
			prev = hist[len(hist)-1]
		}

		hist := append(m.resourceHistory[name], value)
		if len(hist) > historySize {
			hist = hist[len(hist)-historySize:]
		}

		m.resourceHistory[name] = hist
		m.resourceDelta[name] = value - prev
		seen[name] = struct{}{}
	}

	for name := range m.resourceHistory {
		if _, ok := seen[name]; ok {
			continue
		}
		delete(m.resourceHistory, name)
		delete(m.resourceDelta, name)
	}

	m.containerUsages = usages
}

func (m *model) maxVisibleRows() int {
	maxRows := m.height - 14
	if maxRows < 1 {
		maxRows = 20
	}
	return maxRows
}

func (m *model) activeLen() int {
	switch m.viewMode {
	case metrics.ViewDashboard:
		return len(metrics.BuildDashboardRows(m.history, m.delta))
	case metrics.ViewRepos:
		return len(m.repoStatuses)
	case metrics.ViewEvents:
		return len(m.events)
	case metrics.ViewResources:
		return 0
	default:
		return len(m.keys)
	}
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

func (m *model) switchView(mode metrics.ViewMode) {
	m.viewMode = mode
	m.cursor = 0
	m.visibleStart = 0
	m.showLabels = false
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

func (m *model) selectedRawRow() (metrics.MetricRow, bool) {
	if len(m.keys) == 0 || m.cursor < 0 || m.cursor >= len(m.keys) {
		return metrics.MetricRow{}, false
	}
	name := m.keys[m.cursor]
	hist := m.history[name]
	value := 0.0
	if len(hist) > 0 {
		value = hist[len(hist)-1]
	}
	return metrics.MetricRow{
		Name:    name,
		Value:   value,
		Delta:   m.delta[name],
		History: hist,
	}, true
}

func (m *model) selectedDashboardRow() (metrics.DashboardRow, bool) {
	rows := metrics.BuildDashboardRows(m.history, m.delta)
	if len(rows) == 0 || m.cursor < 0 || m.cursor >= len(rows) {
		return metrics.DashboardRow{}, false
	}
	return rows[m.cursor], true
}

func (m *model) summarySignals() []components.CardData {
	rows := metrics.BuildDashboardRows(m.history, m.delta)
	ids := []string{"git-api-requests", "pipelineruns-created", "running-pipelineruns", "workqueue-depth"}
	byID := make(map[string]metrics.DashboardRow, len(rows))
	for _, row := range rows {
		byID[row.Signal.ID] = row
	}
	result := make([]components.CardData, 0, len(ids))
	for _, id := range ids {
		if row, ok := byID[id]; ok {
			trend := ""
			if row.Available {
				trend = metrics.Sparkline(row.History)
			}
			result = append(result, components.CardData{
				Title:     row.Signal.Title,
				Value:     metrics.FormatMetricNumber(row.Value),
				Delta:     metrics.FormatDelta(row.Delta),
				Trend:     trend,
				Available: row.Available,
				SignalID:  row.Signal.ID,
				DeltaNum:  row.Delta,
			})
		}
	}
	return result
}

func fetchViewIfNeeded(m *model, view metrics.ViewMode) (tea.Model, tea.Cmd) {
	switch view {
	case metrics.ViewHealth:
		if !m.healthLoading {
			m.healthLoading = true
			return m, m.runHealthChecks()
		}
	case metrics.ViewRepos:
		if !m.reposLoading {
			m.reposLoading = true
			return m, m.fetchRepoStatuses()
		}
	case metrics.ViewEvents:
		if !m.eventsLoading {
			m.eventsLoading = true
			return m, m.fetchEvents()
		}
	}
	return m, nil
}

func collectHealthChecks(ctx context.Context, kubeconfig, namespace string, endpoints []metrics.EndpointDef, scope metrics.ScopeDef) []views.HealthCheck {
	var checks []views.HealthCheck

	pods, err := kubectl.GetPACPods(ctx, kubeconfig, namespace)
	if err != nil {
		checks = append(checks, views.HealthCheck{Name: "PAC Pods", Status: "fail", Detail: err.Error()})
	} else if len(pods) == 0 {
		checks = append(checks, views.HealthCheck{Name: "PAC Pods", Status: "fail", Detail: "No PAC pods found"})
	} else {
		for _, pod := range pods {
			status := "pass"
			detail := fmt.Sprintf("Phase=%s Restarts=%d Age=%s", pod.Phase, pod.Restarts, pod.Age)
			if pod.Phase != "Running" {
				status = "fail"
			} else if !pod.Ready {
				status = "warn"
				detail += " (not ready)"
			} else if pod.Restarts > 5 {
				status = "warn"
				detail += " (high restart count)"
			}
			checks = append(checks, views.HealthCheck{Name: "Pod: " + pod.Name, Status: status, Detail: detail})
		}
	}

	for _, endpointIndex := range scope.EndpointIndexes {
		endpoint := endpoints[endpointIndex]
		scrapeCtx, scrapeCancel := context.WithTimeout(ctx, 5*time.Second)
		_, scrapeErr := kubectl.ScrapeMetrics(scrapeCtx, kubeconfig, endpoint.SvcPath)
		scrapeCancel()
		if scrapeErr != nil {
			checks = append(checks, views.HealthCheck{Name: "Metrics: " + endpoint.Name, Status: "fail", Detail: scrapeErr.Error()})
		} else {
			checks = append(checks, views.HealthCheck{Name: "Metrics: " + endpoint.Name, Status: "pass", Detail: "reachable"})
		}
	}

	crdExists, crdErr := kubectl.CheckRepositoryCRD(ctx, kubeconfig)
	if crdErr != nil {
		checks = append(checks, views.HealthCheck{Name: "Repository CRD", Status: "fail", Detail: crdErr.Error()})
	} else if crdExists {
		checks = append(checks, views.HealthCheck{Name: "Repository CRD", Status: "pass", Detail: "registered"})
	} else {
		checks = append(checks, views.HealthCheck{Name: "Repository CRD", Status: "fail", Detail: "not found"})
	}

	cmExists, cmErr := kubectl.CheckConfigMap(ctx, kubeconfig, namespace)
	if cmErr != nil {
		checks = append(checks, views.HealthCheck{Name: "ConfigMap: pipelines-as-code", Status: "fail", Detail: cmErr.Error()})
	} else if cmExists {
		checks = append(checks, views.HealthCheck{Name: "ConfigMap: pipelines-as-code", Status: "pass", Detail: "exists"})
	} else {
		checks = append(checks, views.HealthCheck{Name: "ConfigMap: pipelines-as-code", Status: "warn", Detail: "not found in " + namespace})
	}

	return checks
}

func (m *model) runHealthChecks() tea.Cmd {
	kubeconfig := m.kubeconfig
	namespace := m.namespace
	endpoints := m.endpoints
	scope := m.currentScope()

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
		defer cancel()
		return healthCheckResultMsg{checks: collectHealthChecks(ctx, kubeconfig, namespace, endpoints, scope)}
	}
}

func (m *model) fetchRepoStatuses() tea.Cmd {
	kubeconfig := m.kubeconfig
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
		defer cancel()
		repos, err := kubectl.GetRepositoryStatuses(ctx, kubeconfig)
		if err != nil {
			return repoStatusResultMsg{err: err}
		}
		return repoStatusResultMsg{repos: repos}
	}
}

func (m *model) fetchEvents() tea.Cmd {
	kubeconfig := m.kubeconfig
	namespace := m.namespace
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
		defer cancel()
		events, err := kubectl.GetPACEvents(ctx, kubeconfig, namespace)
		if err != nil {
			return eventsResultMsg{err: err}
		}
		return eventsResultMsg{events: events}
	}
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
		m.resourceWarning = msg.resourceErr

		if msg.err != nil && errors.Is(msg.err, context.Canceled) {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err.Error()
		} else {
			m.err = ""
		}
		if len(msg.metrics) > 0 || len(msg.resourceMetrics) > 0 {
			m.lastUpdate = time.Now()
		}
		if len(msg.metrics) > 0 {
			m.applyMetrics(msg)
		}
		if len(msg.resourceMetrics) > 0 || len(msg.containerUsages) > 0 {
			m.applyResourceMetrics(msg.resourceMetrics, msg.containerUsages)
		}
		return m, doTick(m.interval)

	case healthCheckResultMsg:
		m.healthLoading = false
		m.healthChecks = msg.checks
		return m, nil

	case repoStatusResultMsg:
		m.reposLoading = false
		m.repoStatuses = msg.repos
		if msg.err != nil {
			m.reposErr = fmt.Sprintf("error fetching repositories: %v", msg.err)
		} else {
			m.reposErr = ""
		}
		return m, nil

	case eventsResultMsg:
		m.eventsLoading = false
		m.events = msg.events
		if msg.err != nil {
			m.eventsErr = fmt.Sprintf("error fetching events: %v", msg.err)
		} else {
			m.eventsErr = ""
		}
		return m, nil

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
			if m.viewMode == metrics.ViewResources {
				switch m.resourceFocus {
				case views.ResourceFocusMemory:
					m.resourceFocus = views.ResourceFocusRuntime
				case views.ResourceFocusRuntime:
					m.resourceFocus = views.ResourceFocusQueue
				default:
					m.resourceFocus = views.ResourceFocusMemory
				}
			} else {
				if m.sortMode == metrics.SortByDelta {
					m.sortMode = metrics.SortByAlpha
				} else {
					m.sortMode = metrics.SortByDelta
				}
				m.recomputeRows()
			}
		case "d":
			m.switchView(metrics.ViewDashboard)
		case "r":
			m.switchView(metrics.ViewRaw)
			m.recomputeRows()
		case "m":
			m.switchView(metrics.ViewResources)
		case "h":
			m.switchView(metrics.ViewHealth)
			return fetchViewIfNeeded(m, metrics.ViewHealth)
		case "p":
			m.switchView(metrics.ViewRepos)
			return fetchViewIfNeeded(m, metrics.ViewRepos)
		case "e":
			m.switchView(metrics.ViewEvents)
			return fetchViewIfNeeded(m, metrics.ViewEvents)
		case "enter":
			if m.viewMode == metrics.ViewDashboard {
				m.showLabels = !m.showLabels
			}
		case "esc":
			m.showLabels = false
		case "/":
			if m.viewMode == metrics.ViewRaw {
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

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func aggregateContainerUsages(usages []kubectl.ContainerUsage) map[string]float64 {
	result := map[string]float64{}
	for _, usage := range usages {
		result[componentResourceKey(usage.Component, "container_memory_bytes")] += float64(usage.MemoryBytes)
		result[componentResourceKey(usage.Component, "container_cpu_millicores")] += float64(usage.CPUmilli)
	}
	return result
}

func componentResourceKey(component, metric string) string {
	return "component_" + component + "_" + metric
}

func mergeEndpointMetrics(dst map[string]float64, endpoint string, samples map[string]float64) {
	for name, value := range samples {
		if metrics.RuntimeCollectorMetric(name) {
			dst[componentMetricKey(endpoint, name)] = value
			continue
		}
		dst[name] += value
	}
}

func metricSnapshot(history map[string][]float64, delta map[string]float64, key string) ([]float64, float64, float64, bool, bool) {
	hist, ok := history[key]
	if !ok || len(hist) == 0 {
		return nil, 0, 0, false, false
	}
	current := hist[len(hist)-1]
	valueDelta, deltaOK := delta[key]
	return hist, current, valueDelta, true, deltaOK
}

func componentMetricKey(component, canonical string) string {
	return "pac_" + component + "_" + canonical
}

func metricStat(label, value string, available bool, deltaValue float64, deltaOK bool, formatter func(float64) string) views.ResourceStat {
	stat := views.ResourceStat{Label: label, Available: available}
	if !available {
		return stat
	}
	stat.Value = value
	if deltaOK {
		stat.Delta = formatter(deltaValue)
	} else {
		stat.Delta = "n/a"
	}
	return stat
}

func buildResourceSections(scope metrics.ScopeDef, focus views.ResourceFocus, metricHistory map[string][]float64, metricDelta map[string]float64, resourceHistory map[string][]float64, resourceDelta map[string]float64, containerUsages []kubectl.ContainerUsage) []views.ComponentResources {
	var componentsList []string
	switch scope.Name {
	case "controller":
		componentsList = []string{"controller"}
	case "watcher":
		componentsList = []string{"watcher"}
	default:
		componentsList = []string{"controller", "watcher"}
	}

	var sections []views.ComponentResources
	for _, component := range componentsList {
		section := buildResourceSection(component, focus, metricHistory, metricDelta, resourceHistory, resourceDelta, containerUsages)
		if section.PrimaryTitle == "" && len(section.Stats) == 0 {
			continue
		}
		sections = append(sections, section)
	}
	return sections
}

func buildResourceSection(component string, focus views.ResourceFocus, metricHistory map[string][]float64, metricDelta map[string]float64, resourceHistory map[string][]float64, resourceDelta map[string]float64, containerUsages []kubectl.ContainerUsage) views.ComponentResources {
	titlePrefix := strings.ToUpper(component[:1]) + component[1:]

	containerMemKey := componentResourceKey(component, "container_memory_bytes")
	containerCPUKey := componentResourceKey(component, "container_cpu_millicores")
	rssKey := componentMetricKey(component, "process_resident_memory_bytes")
	virtualMemKey := componentMetricKey(component, "process_virtual_memory_bytes")
	heapKey := componentMetricKey(component, "go_memstats_heap_inuse_bytes")
	allocKey := componentMetricKey(component, "go_memstats_alloc_bytes")
	goroutinesKey := componentMetricKey(component, "go_goroutines")
	gcPauseKey := componentMetricKey(component, "go_gc_duration_seconds_sum")
	openFDsKey := componentMetricKey(component, "process_open_fds")
	maxFDsKey := componentMetricKey(component, "process_max_fds")
	queueDepthKey := componentMetricKey(component, "workqueue_depth")
	unfinishedKey := componentMetricKey(component, "workqueue_unfinished_work_seconds")
	longestKey := componentMetricKey(component, "workqueue_longest_running_processor_seconds")
	retriesKey := componentMetricKey(component, "workqueue_retries_total")
	activeWorkersKey := componentMetricKey(component, "controller_runtime_active_workers")
	reconcileErrorsKey := componentMetricKey(component, "controller_runtime_reconcile_errors_total")

	primaryTitle := ""
	primaryKind := "gauge"
	primaryValue := "n/a"
	primaryDelta := "n/a"
	primaryDescription := "No data available for this component."
	primarySources := "<none>"
	var primaryHistory []float64

	switch focus {
	case views.ResourceFocusMemory:
		if hist, current, deltaValue, ok, deltaOK := metricSnapshot(resourceHistory, resourceDelta, containerMemKey); ok {
			primaryTitle = titlePrefix + " Container Memory"
			primaryValue = metrics.FormatBytes(current)
			if deltaOK {
				primaryDelta = metrics.FormatBytesDelta(deltaValue)
			}
			primaryDescription = "Summed container memory usage from kubectl top for this PAC component."
			primarySources = "kubectl top"
			primaryHistory = hist
		} else if hist, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, rssKey); ok {
			primaryTitle = titlePrefix + " Process RSS"
			primaryValue = metrics.FormatBytes(current)
			if deltaOK {
				primaryDelta = metrics.FormatBytesDelta(deltaValue)
			}
			primaryDescription = "Resident memory reported by the Prometheus process collector on the PAC metrics endpoint."
			primarySources = rssKey
			primaryHistory = hist
		}
	case views.ResourceFocusRuntime:
		if hist, current, deltaValue, ok, deltaOK := metricSnapshot(resourceHistory, resourceDelta, containerCPUKey); ok {
			primaryTitle = titlePrefix + " Container CPU"
			primaryValue = metrics.FormatMillicores(current)
			if deltaOK {
				primaryDelta = metrics.FormatMillicoresDelta(deltaValue)
			}
			primaryDescription = "Summed sampled CPU usage from kubectl top for this PAC component."
			primarySources = "kubectl top"
			primaryHistory = hist
		} else if hist, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, goroutinesKey); ok {
			primaryTitle = titlePrefix + " Goroutines"
			primaryValue = metrics.FormatMetricNumber(current)
			if deltaOK {
				primaryDelta = metrics.FormatDelta(deltaValue)
			}
			primaryDescription = "Go runtime goroutine count exposed by the PAC metrics endpoint."
			primarySources = goroutinesKey
			primaryHistory = hist
		}
	case views.ResourceFocusQueue:
		if hist, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, queueDepthKey); ok {
			primaryTitle = titlePrefix + " Workqueue Depth"
			primaryValue = metrics.FormatMetricNumber(current)
			if deltaOK {
				primaryDelta = metrics.FormatDelta(deltaValue)
			}
			primaryDescription = "Current workqueue depth for this PAC component."
			primarySources = queueDepthKey
			primaryHistory = hist
		} else if hist, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, activeWorkersKey); ok {
			primaryTitle = titlePrefix + " Active Workers"
			primaryValue = metrics.FormatMetricNumber(current)
			if deltaOK {
				primaryDelta = metrics.FormatDelta(deltaValue)
			}
			primaryDescription = "Current active reconcile workers for this PAC component."
			primarySources = activeWorkersKey
			primaryHistory = hist
		}
	}

	stats := make([]views.ResourceStat, 0, 8)
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(resourceHistory, resourceDelta, containerMemKey); ok {
		stats = append(stats, metricStat("Container Memory", metrics.FormatBytes(current), true, deltaValue, deltaOK, metrics.FormatBytesDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Container Memory"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(resourceHistory, resourceDelta, containerCPUKey); ok {
		stats = append(stats, metricStat("Container CPU", metrics.FormatMillicores(current), true, deltaValue, deltaOK, metrics.FormatMillicoresDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Container CPU"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, rssKey); ok {
		stats = append(stats, metricStat("RSS", metrics.FormatBytes(current), true, deltaValue, deltaOK, metrics.FormatBytesDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "RSS"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, heapKey); ok {
		stats = append(stats, metricStat("Heap In Use", metrics.FormatBytes(current), true, deltaValue, deltaOK, metrics.FormatBytesDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Heap In Use"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, allocKey); ok {
		stats = append(stats, metricStat("Alloc", metrics.FormatBytes(current), true, deltaValue, deltaOK, metrics.FormatBytesDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Alloc"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, virtualMemKey); ok {
		stats = append(stats, metricStat("Virtual Mem", metrics.FormatBytes(current), true, deltaValue, deltaOK, metrics.FormatBytesDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Virtual Mem"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, goroutinesKey); ok {
		stats = append(stats, metricStat("Goroutines", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Goroutines"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, gcPauseKey); ok {
		stats = append(stats, metricStat("GC Pause Sum", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "GC Pause Sum"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, openFDsKey); ok {
		stats = append(stats, metricStat("Open FDs", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Open FDs"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, maxFDsKey); ok {
		stats = append(stats, metricStat("Max FDs", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	} else {
		stats = append(stats, views.ResourceStat{Label: "Max FDs"})
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, queueDepthKey); ok {
		stats = append(stats, metricStat("Queue Depth", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, unfinishedKey); ok {
		stats = append(stats, metricStat("Unfinished Work", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, longestKey); ok {
		stats = append(stats, metricStat("Longest Running", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, retriesKey); ok {
		stats = append(stats, metricStat("Retries", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, activeWorkersKey); ok {
		stats = append(stats, metricStat("Active Workers", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	}
	if _, current, deltaValue, ok, deltaOK := metricSnapshot(metricHistory, metricDelta, reconcileErrorsKey); ok {
		stats = append(stats, metricStat("Reconcile Errors", metrics.FormatMetricNumber(current), true, deltaValue, deltaOK, metrics.FormatDelta))
	}

	filteredUsages := make([]kubectl.ContainerUsage, 0)
	for _, usage := range containerUsages {
		if usage.Component != component {
			continue
		}
		filteredUsages = append(filteredUsages, usage)
	}
	sort.Slice(filteredUsages, func(i, j int) bool {
		return filteredUsages[i].MemoryBytes > filteredUsages[j].MemoryBytes
	})

	componentContainers := make([]views.ContainerStat, 0, len(filteredUsages))
	for _, usage := range filteredUsages {
		componentContainers = append(componentContainers, views.ContainerStat{
			PodName:       usage.PodName,
			ContainerName: usage.ContainerName,
			CPU:           metrics.FormatMillicores(float64(usage.CPUmilli)),
			Memory:        metrics.FormatBytes(float64(usage.MemoryBytes)),
		})
	}

	if primaryTitle == "" {
		return views.ComponentResources{}
	}

	return views.ComponentResources{
		Name:               titlePrefix,
		PrimaryTitle:       primaryTitle,
		PrimaryKind:        primaryKind,
		PrimaryValue:       primaryValue,
		PrimaryDelta:       primaryDelta,
		PrimaryDescription: primaryDescription,
		PrimarySources:     primarySources,
		PrimaryHistory:     primaryHistory,
		Stats:              stats,
		Containers:         componentContainers,
	}
}

func (m *model) renderDashboardRows(width int) ([]string, string) {
	rows := metrics.BuildDashboardRows(m.history, m.delta)
	maxRows := m.maxVisibleRows()
	m.ensureCursorVisible()

	signalW := max(22, width/5)
	valueW := max(8, width/12)
	deltaW := max(6, width/14)
	trendW := max(12, width/8)
	whyW := max(20, width-signalW-valueW-deltaW-trendW-8)
	columns := []components.Column{
		{Title: "SIGNAL", Width: signalW},
		{Title: "VALUE", Width: valueW, AlignRight: true},
		{Title: "DELTA", Width: deltaW, AlignRight: true},
		{Title: "TREND", Width: trendW},
		{Title: "WHAT TO WATCH", Width: whyW},
	}

	var tableRows []components.TableRow
	lastGroup := ""

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
			value = metrics.FormatMetricNumber(row.Value)
			delta = metrics.FormatDelta(row.Delta)
			graph = metrics.Sparkline(row.History)
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
			Columns:    []string{row.Signal.Title, value, delta, graph, row.Signal.Why},
			Style:      rowStyle,
			DeltaValue: row.Delta,
		})
	}

	rendered := components.RenderTable(columns, tableRows, m.cursor-m.visibleStart)

	detail := theme.StyleDim.Render("selected: <none>")
	if row, ok := m.selectedDashboardRow(); ok {
		if m.showLabels {
			// Show label breakdown instead of chart
			detail = m.renderLabelDetail(row, width)
		} else {
			sources := "<none>"
			if len(row.Sources) > 0 {
				sources = strings.Join(row.Sources, ", ")
			}

			valStr := "n/a"
			deltaStr := "n/a"
			if row.Available {
				valStr = metrics.FormatMetricNumber(row.Value)
				deltaStr = metrics.FormatDelta(row.Delta)
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
	}

	return rendered, detail
}

func (m *model) renderLabelDetail(row metrics.DashboardRow, width int) string {
	if m.labeledSnapshot == nil {
		return theme.StyleDim.Render("No label data available (waiting for next scrape)")
	}

	// Find matching metric families for this signal
	var combined *metrics.MetricFamily
	for name, family := range m.labeledSnapshot {
		if metrics.MatchesSignal(name, row.Signal) {
			if combined == nil {
				combined = &metrics.MetricFamily{
					Name:    row.Signal.Title,
					Samples: make([]metrics.LabeledSample, 0, len(family.Samples)),
				}
			}
			combined.Samples = append(combined.Samples, family.Samples...)
			combined.Total += family.Total
		}
	}

	return views.RenderLabelBreakdown(combined, width)
}

func (m *model) renderRawRows(width int) ([]string, string) {
	maxRows := m.maxVisibleRows()
	m.ensureCursorVisible()

	metricW := max(36, width/2)
	valW := max(10, width/8)
	dltW := max(10, width/8)
	graphW := max(16, width-metricW-valW-dltW-6)
	columns := []components.Column{
		{Title: "RAW METRIC", Width: metricW},
		{Title: "VALUE", Width: valW, AlignRight: true},
		{Title: "DELTA", Width: dltW, AlignRight: true},
		{Title: "GRAPH", Width: graphW},
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

		graph := metrics.Sparkline(hist)
		graphRunes := []rune(graph)
		if len(graphRunes) > 20 {
			graph = string(graphRunes[len(graphRunes)-20:])
		}

		rowStyle := theme.StyleNormal
		if m.delta[name] > 0 {
			rowStyle = theme.StyleIncr
		}

		tableRows = append(tableRows, components.TableRow{
			Columns:    []string{name, metrics.FormatMetricNumber(value), metrics.FormatDelta(m.delta[name]), graph},
			Style:      rowStyle,
			DeltaValue: m.delta[name],
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
		rendered = components.RenderTable(columns, tableRows, m.cursor-m.visibleStart)
	}

	detail := theme.StyleDim.Render("selected: <none>")
	if row, ok := m.selectedRawRow(); ok {
		detail = components.RenderDetailPane(
			row.Name,
			"raw",
			metrics.FormatMetricNumber(row.Value),
			metrics.FormatDelta(row.Delta),
			fmt.Sprintf("samples=%d canonical=%s", len(row.History), metrics.CanonicalMetricName(row.Name)),
			"kubectl",
			row.History,
			width,
		)
	}
	return rendered, detail
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
		scopeStrs[i] = s.Name
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
	thinSep := theme.StyleSep.Render(strings.Repeat("╌", width))

	var rows []string
	var detail string

	switch m.viewMode {
	case metrics.ViewDashboard:
		summary := components.RenderSummaryCards(m.summarySignals(), width)
		rows, detail = m.renderDashboardRows(width)
		footer := components.RenderFooter(m.err, m.filterMode, m.filterInput, width)
		parts := []string{header, separator, summary, thinSep}
		parts = append(parts, rows...)
		parts = append(parts, separator, detail)
		view := tea.NewView(renderWithFooter(parts, footer, m.height))
		view.AltScreen = true
		return view

	case metrics.ViewRaw:
		summary := components.RenderSummaryCards(m.summarySignals(), width)
		rows, detail = m.renderRawRows(width)
		footer := components.RenderFooter(m.err, m.filterMode, m.filterInput, width)
		parts := []string{header, separator, summary, thinSep}
		parts = append(parts, rows...)
		parts = append(parts, separator, detail)
		view := tea.NewView(renderWithFooter(parts, footer, m.height))
		view.AltScreen = true
		return view

	case metrics.ViewResources:
		content := views.RenderResourcesView(
			m.resourceFocus,
			buildResourceSections(m.currentScope(), m.resourceFocus, m.history, m.delta, m.resourceHistory, m.resourceDelta, m.containerUsages),
			m.resourceWarning,
			width,
		)
		footer := components.RenderFooter(m.err, false, "", width)
		parts := []string{header, separator, content}
		view := tea.NewView(renderWithFooter(parts, footer, m.height))
		view.AltScreen = true
		return view

	case metrics.ViewHealth:
		healthContent := views.RenderHealthView(m.healthChecks, m.healthLoading, width)
		footer := components.RenderFooter(m.err, false, "", width)
		parts := []string{header, separator, healthContent}
		view := tea.NewView(renderWithFooter(parts, footer, m.height))
		view.AltScreen = true
		return view

	case metrics.ViewRepos:
		rows, detail = views.RenderReposView(m.repoStatuses, m.cursor, m.visibleStart, m.maxVisibleRows(), width, m.reposLoading, m.reposErr)
		footer := components.RenderFooter(m.err, false, "", width)
		parts := []string{header, separator}
		parts = append(parts, rows...)
		if detail != "" {
			parts = append(parts, separator, detail)
		}
		view := tea.NewView(renderWithFooter(parts, footer, m.height))
		view.AltScreen = true
		return view

	case metrics.ViewEvents:
		rows, detail = views.RenderEventsView(m.events, m.cursor, m.visibleStart, m.maxVisibleRows(), width, m.eventsLoading, m.eventsErr)
		footer := components.RenderFooter(m.err, false, "", width)
		parts := []string{header, separator}
		parts = append(parts, rows...)
		if detail != "" {
			parts = append(parts, separator, detail)
		}
		view := tea.NewView(renderWithFooter(parts, footer, m.height))
		view.AltScreen = true
		return view
	}

	// fallback
	view := tea.NewView(header)
	view.AltScreen = true
	return view
}

func renderWithFooter(parts []string, footer string, height int) string {
	content := strings.Join(parts, "\n")
	if height <= 0 {
		return content + "\n\n" + footer
	}

	spacerLines := height - lipgloss.Height(content) - lipgloss.Height(footer)
	if spacerLines < 1 {
		spacerLines = 1
	}

	return content + "\n" + strings.Repeat("\n", spacerLines) + footer
}

func renderSnapshot(scope string, collectedAt time.Time, m map[string]float64, output string) string {
	var builder strings.Builder
	if output == "tsv" {
		rows := make([]metrics.MetricRow, 0, len(m))
		for name, value := range m {
			rows = append(rows, metrics.MetricRow{Name: name, Value: value})
		}
		metrics.SortRows(rows, metrics.SortByAlpha)

		fmt.Fprintf(&builder, "# scope=%s\n", scope)
		fmt.Fprintf(&builder, "# timestamp=%s\n", collectedAt.Format(time.RFC3339))
		builder.WriteString("metric\tvalue\n")
		for _, row := range rows {
			fmt.Fprintf(&builder, "%s\t%s\n", row.Name, metrics.FormatMetricNumber(row.Value))
		}
		return builder.String()
	}

	dashboardRows := metrics.SignalRowsFromMetrics(m)
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
			value = metrics.FormatMetricNumber(row.Value)
		}
		_, _ = fmt.Fprintf(tabWriter, "%s\t%s\t%s\t%s\n", row.Group, row.Signal.Title, value, row.Signal.Why)
	}
	_ = tabWriter.Flush()
	return builder.String()
}

func runSnapshot(config metrics.SnapshotConfig, scraper metrics.ScrapeFunc) (string, error) {
	endpoints := metrics.BuildEndpoints(config.Namespace)
	scopes := metrics.BuildScopes(endpoints)
	index, err := metrics.ScopeIndex(scopes, config.Scope)
	if err != nil {
		return "", err
	}

	scope := scopes[index]

	// Handle --view flag for non-metrics views
	switch config.View {
	case "health":
		return runHealthSnapshot(config, endpoints, scope)
	case "repos":
		return runReposSnapshot(config)
	case "events":
		return runEventsSnapshot(config)
	case "raw":
		return runRawSnapshot(config, endpoints, scope, scraper)
	case "resources":
		return runResourcesSnapshot(config, endpoints, scope, scraper)
	}

	m := map[string]float64{}
	for _, endpointIndex := range scope.EndpointIndexes {
		endpoint := endpoints[endpointIndex]
		scrapedMetrics, err := scraper(context.Background(), config.Kubeconfig, endpoint.SvcPath)
		if err != nil {
			return "", fmt.Errorf("%s: %w", endpoint.Name, err)
		}
		for name, value := range scrapedMetrics {
			m[name] += value
		}
	}

	return renderSnapshot(scope.Name, time.Now(), m, config.Output), nil
}

func runHealthSnapshot(config metrics.SnapshotConfig, endpoints []metrics.EndpointDef, scope metrics.ScopeDef) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()
	checks := collectHealthChecks(ctx, config.Kubeconfig, config.Namespace, endpoints, scope)
	return views.RenderHealthSnapshot(checks), nil
}

func runReposSnapshot(config metrics.SnapshotConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()

	repos, err := kubectl.GetRepositoryStatuses(ctx, config.Kubeconfig)
	if err != nil {
		return "", fmt.Errorf("get repositories: %w", err)
	}

	return views.RenderReposSnapshot(repos), nil
}

func runEventsSnapshot(config metrics.SnapshotConfig) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), snapshotTimeout)
	defer cancel()

	events, err := kubectl.GetPACEvents(ctx, config.Kubeconfig, config.Namespace)
	if err != nil {
		return "", fmt.Errorf("get events: %w", err)
	}

	return views.RenderEventsSnapshot(events), nil
}

func runRawSnapshot(config metrics.SnapshotConfig, endpoints []metrics.EndpointDef, scope metrics.ScopeDef, scraper metrics.ScrapeFunc) (string, error) {
	m := map[string]float64{}
	for _, endpointIndex := range scope.EndpointIndexes {
		endpoint := endpoints[endpointIndex]
		scrapedMetrics, err := scraper(context.Background(), config.Kubeconfig, endpoint.SvcPath)
		if err != nil {
			return "", fmt.Errorf("%s: %w", endpoint.Name, err)
		}
		for name, value := range scrapedMetrics {
			m[name] += value
		}
	}

	return renderRawSnapshot(scope.Name, time.Now(), m, config), nil
}

func runResourcesSnapshot(config metrics.SnapshotConfig, endpoints []metrics.EndpointDef, scope metrics.ScopeDef, scraper metrics.ScrapeFunc) (string, error) {
	m := map[string]float64{}
	for _, endpointIndex := range scope.EndpointIndexes {
		endpoint := endpoints[endpointIndex]
		scrapedMetrics, err := scraper(context.Background(), config.Kubeconfig, endpoint.SvcPath)
		if err != nil {
			return "", fmt.Errorf("%s: %w", endpoint.Name, err)
		}
		for name, value := range scrapedMetrics {
			m[name] += value
		}
	}

	usages, usageErr := kubectl.GetPACContainerUsages(context.Background(), config.Kubeconfig, config.Namespace)
	resourceCurrent := aggregateContainerUsages(usages)
	resourceHistory := make(map[string][]float64, len(resourceCurrent))
	for name, value := range resourceCurrent {
		resourceHistory[name] = []float64{value}
	}

	metricHistory := make(map[string][]float64, len(m))
	for name, value := range m {
		metricHistory[name] = []float64{value}
	}

	sections := buildResourceSections(scope, views.ResourceFocusMemory, metricHistory, map[string]float64{}, resourceHistory, map[string]float64{}, usages)
	return views.RenderResourcesSnapshot(scope.Name, time.Now(), views.ResourceFocusMemory, sections, errorString(usageErr), config.Output), nil
}

func renderRawSnapshot(scope string, collectedAt time.Time, m map[string]float64, config metrics.SnapshotConfig) string {
	rows := make([]metrics.MetricRow, 0, len(m))
	for name, value := range m {
		if !metrics.MetricAllowed(name, config.PacOnly, config.Filter) {
			continue
		}
		rows = append(rows, metrics.MetricRow{Name: name, Value: value})
	}
	metrics.SortRows(rows, config.SortMode)

	var builder strings.Builder
	if config.Output == "tsv" {
		fmt.Fprintf(&builder, "# scope=%s view=raw\n", scope)
		fmt.Fprintf(&builder, "# timestamp=%s\n", collectedAt.Format(time.RFC3339))
		builder.WriteString("metric\tvalue\n")
		for _, row := range rows {
			fmt.Fprintf(&builder, "%s\t%s\n", row.Name, metrics.FormatMetricNumber(row.Value))
		}
		return builder.String()
	}

	tabWriter := tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tabWriter, "scope:\t%s\n", scope)
	_, _ = fmt.Fprintf(tabWriter, "view:\traw\n")
	_, _ = fmt.Fprintf(tabWriter, "timestamp:\t%s\n", collectedAt.Format(time.RFC3339))
	_ = tabWriter.Flush()
	builder.WriteString("\n")

	tabWriter = tabwriter.NewWriter(&builder, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tabWriter, "METRIC\tVALUE")
	for _, row := range rows {
		_, _ = fmt.Fprintf(tabWriter, "%s\t%s\n", row.Name, metrics.FormatMetricNumber(row.Value))
	}
	_ = tabWriter.Flush()
	return builder.String()
}

func main() {
	kubeconfig := flag.String("kubeconfig", "", "path to kubeconfig (default: $KUBECONFIG or in-cluster)")
	namespace := flag.String("namespace", defaultNamespace, "namespace where PAC is running")
	interval := flag.Duration("interval", defaultInterval, "polling interval")
	pacOnly := flag.Bool("pac-only", false, "show only pac_ prefixed metrics in raw mode")
	once := flag.Bool("once", false, "scrape once, print a report, and exit")
	scopeFlag := flag.String("endpoint", "all", "scope to use: all, controller, or watcher")
	sortFlag := flag.String("sort", string(metrics.SortByDelta), "sort order for raw mode: delta or alpha")
	filter := flag.String("filter", "", "substring filter for raw metric names")
	output := flag.String("output", defaultSnapshotMode, "snapshot output format: table or tsv")
	viewFlag := flag.String("view", "", "view for --once mode: dashboard, raw, resources, health, repos, or events")
	flag.Parse()

	if *kubeconfig == "" {
		*kubeconfig = os.Getenv("KUBECONFIG")
	}

	if *namespace == defaultNamespace {
		if detected, err := kubectl.DetectNamespace(context.Background(), *kubeconfig); err == nil {
			*namespace = detected
		}
	}

	sortModeValue, err := metrics.NormalizeSortMode(*sortFlag)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	outputMode, err := metrics.NormalizeOutputMode(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *once {
		report, err := runSnapshot(metrics.SnapshotConfig{
			Kubeconfig: *kubeconfig,
			Namespace:  *namespace,
			Scope:      *scopeFlag,
			PacOnly:    *pacOnly,
			Filter:     *filter,
			SortMode:   sortModeValue,
			Output:     outputMode,
			View:       *viewFlag,
		}, kubectl.ScrapeMetrics)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Print(report)
		return
	}

	scopeValue, err := metrics.ScopeIndex(metrics.BuildScopes(metrics.BuildEndpoints(*namespace)), *scopeFlag)
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
		nil,
	))
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
