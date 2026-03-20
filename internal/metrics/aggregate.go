package metrics

import (
	"math"
	"sort"
)

var DashboardGroups = []DashboardGroup{
	{
		Title:       "PAC Flow",
		Description: "Core business signals for webhook traffic and PipelineRun activity.",
		Signals: []SignalDefinition{
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
		Signals: []SignalDefinition{
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
		Signals: []SignalDefinition{
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

func SortRows(rows []MetricRow, mode SortMode) {
	switch mode {
	case SortByAlpha:
		sort.Slice(rows, func(i, j int) bool {
			return rows[i].Name < rows[j].Name
		})
	default:
		sort.Slice(rows, func(i, j int) bool {
			left := math.Abs(rows[i].Delta)
			right := math.Abs(rows[j].Delta)
			if left == right {
				return rows[i].Name < rows[j].Name
			}
			return left > right
		})
	}
}

func BuildRowsFromHistory(history map[string][]float64, delta map[string]float64, pacOnly bool, filter string, mode SortMode) []MetricRow {
	rows := make([]MetricRow, 0, len(history))
	for name, hist := range history {
		if !MetricAllowed(name, pacOnly, filter) {
			continue
		}
		value := 0.0
		if len(hist) > 0 {
			value = hist[len(hist)-1]
		}
		rows = append(rows, MetricRow{
			Name:    name,
			Value:   value,
			Delta:   delta[name],
			History: hist,
		})
	}
	SortRows(rows, mode)
	return rows
}

func AggregateHistories(history map[string][]float64, names []string) []float64 {
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

func BuildDashboardRows(history map[string][]float64, delta map[string]float64) []DashboardRow {
	rows := make([]DashboardRow, 0, 12)
	for _, group := range DashboardGroups {
		for _, signal := range group.Signals {
			sources := make([]string, 0, 2)
			totalDelta := 0.0
			for name := range history {
				if MatchesSignal(name, signal) {
					sources = append(sources, name)
					totalDelta += delta[name]
				}
			}
			sort.Strings(sources)

			row := DashboardRow{
				Group:       group.Title,
				Description: group.Description,
				Signal:      signal,
				Delta:       totalDelta,
				Sources:     sources,
				Available:   len(sources) > 0,
			}
			if row.Available {
				row.History = AggregateHistories(history, sources)
				if len(row.History) > 0 {
					row.Value = row.History[len(row.History)-1]
				}
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func SignalRowsFromMetrics(m map[string]float64) []DashboardRow {
	history := make(map[string][]float64, len(m))
	for name, value := range m {
		history[name] = []float64{value}
	}
	return BuildDashboardRows(history, map[string]float64{})
}
