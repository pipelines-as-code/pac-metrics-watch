package kubectl

import (
	"context"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/metrics"
)

func ScrapeMetrics(ctx context.Context, kubeconfig, svcPath string) (map[string]float64, error) {
	out, err := RunKubectl(ctx, kubeconfig, "get", "--raw", svcPath)
	if err != nil {
		return nil, err
	}
	return metrics.ParseMetrics(string(out))
}

func ScrapeRawMetrics(ctx context.Context, kubeconfig, svcPath string) (string, error) {
	out, err := RunKubectl(ctx, kubeconfig, "get", "--raw", svcPath)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
