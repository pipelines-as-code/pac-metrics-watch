package kubectl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type PodStatus struct {
	Name     string
	Phase    string
	Ready    bool
	Restarts int
	Age      string
}

func GetPACPods(ctx context.Context, kubeconfig, namespace string) ([]PodStatus, error) {
	out, err := RunKubectl(ctx, kubeconfig,
		"get", "pods",
		"-n", namespace,
		"-l", "app.kubernetes.io/part-of=pipelines-as-code",
		"-o", "json",
	)
	if err != nil {
		return nil, err
	}

	var podList struct {
		Items []struct {
			Metadata struct {
				Name              string    `json:"name"`
				CreationTimestamp time.Time `json:"creationTimestamp"`
			} `json:"metadata"`
			Status struct {
				Phase             string `json:"phase"`
				ContainerStatuses []struct {
					Ready        bool `json:"ready"`
					RestartCount int  `json:"restartCount"`
				} `json:"containerStatuses"`
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &podList); err != nil {
		return nil, fmt.Errorf("parse pod list: %w", err)
	}

	pods := make([]PodStatus, 0, len(podList.Items))
	for _, item := range podList.Items {
		ready := true
		restarts := 0
		for _, cs := range item.Status.ContainerStatuses {
			if !cs.Ready {
				ready = false
			}
			restarts += cs.RestartCount
		}

		age := time.Since(item.Metadata.CreationTimestamp).Truncate(time.Second).String()

		pods = append(pods, PodStatus{
			Name:     item.Metadata.Name,
			Phase:    item.Status.Phase,
			Ready:    ready,
			Restarts: restarts,
			Age:      age,
		})
	}
	return pods, nil
}
