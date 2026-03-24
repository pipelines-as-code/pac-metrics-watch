package metrics

import (
	"context"
	"fmt"
	"strings"
)

type SortMode string

const (
	SortByDelta SortMode = "delta"
	SortByAlpha SortMode = "alpha"
)

type ViewMode string

const (
	ViewDashboard ViewMode = "dashboard"
	ViewRaw       ViewMode = "raw"
	ViewHealth    ViewMode = "health"
	ViewRepos     ViewMode = "repos"
	ViewEvents    ViewMode = "events"
	ViewResources ViewMode = "resources"
)

type LabeledSample struct {
	Labels map[string]string
	Value  float64
}

type MetricFamily struct {
	Name    string
	Samples []LabeledSample
	Total   float64
}

type MetricRow struct {
	Name    string
	Value   float64
	Delta   float64
	History []float64
}

type SignalDefinition struct {
	ID          string
	Title       string
	Kind        string
	Exact       []string
	Prefixes    []string
	Description string
	Why         string
}

type DashboardGroup struct {
	Title       string
	Description string
	Signals     []SignalDefinition
}

type DashboardRow struct {
	Group       string
	Description string
	Signal      SignalDefinition
	Value       float64
	Delta       float64
	History     []float64
	Sources     []string
	Available   bool
}

type SnapshotConfig struct {
	Kubeconfig string
	Namespace  string
	Scope      string
	PacOnly    bool
	Filter     string
	SortMode   SortMode
	Output     string
	View       string
}

type ScrapeFunc func(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error)

type EndpointDef struct {
	Name    string
	SvcPath string
}

type ScopeDef struct {
	Name            string
	EndpointIndexes []int
}

func BuildEndpoints(namespace string) []EndpointDef {
	base := fmt.Sprintf("/api/v1/namespaces/%s/services", namespace)
	return []EndpointDef{
		{Name: "controller", SvcPath: fmt.Sprintf("%s/pipelines-as-code-controller:9090/proxy/metrics", base)},
		{Name: "watcher", SvcPath: fmt.Sprintf("%s/pipelines-as-code-watcher:9090/proxy/metrics", base)},
	}
}

func BuildScopes(endpoints []EndpointDef) []ScopeDef {
	return []ScopeDef{
		{Name: "all", EndpointIndexes: []int{0, 1}},
		{Name: "controller", EndpointIndexes: []int{0}},
		{Name: "watcher", EndpointIndexes: []int{1}},
	}
}

func ScopeIndex(scopes []ScopeDef, name string) (int, error) {
	needle := strings.ToLower(strings.TrimSpace(name))
	for i, scope := range scopes {
		if scope.Name == needle {
			return i, nil
		}
	}
	return -1, fmt.Errorf("unsupported endpoint %q", name)
}

func NormalizeSortMode(raw string) (SortMode, error) {
	switch SortMode(strings.ToLower(strings.TrimSpace(raw))) {
	case "", SortByDelta:
		return SortByDelta, nil
	case SortByAlpha:
		return SortByAlpha, nil
	default:
		return "", fmt.Errorf("unsupported sort mode %q", raw)
	}
}

func NormalizeOutputMode(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "table":
		return "table", nil
	case "tsv":
		return "tsv", nil
	default:
		return "", fmt.Errorf("unsupported output mode %q", raw)
	}
}
