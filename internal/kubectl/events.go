package kubectl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type K8sEvent struct {
	Time    time.Time
	Type    string
	Reason  string
	Message string
	Object  string
}

func GetPACEvents(ctx context.Context, kubeconfig, namespace string) ([]K8sEvent, error) {
	out, err := RunKubectl(ctx, kubeconfig,
		"get", "events",
		"-n", namespace,
		"--sort-by=.lastTimestamp",
		"-o", "json",
	)
	if err != nil {
		return nil, err
	}

	var eventList struct {
		Items []struct {
			LastTimestamp *time.Time `json:"lastTimestamp"`
			EventTime    *time.Time `json:"eventTime"`
			Type         string     `json:"type"`
			Reason       string     `json:"reason"`
			Message      string     `json:"message"`
			InvolvedObject struct {
				Kind string `json:"kind"`
				Name string `json:"name"`
			} `json:"involvedObject"`
			Source struct {
				Component string `json:"component"`
			} `json:"source"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &eventList); err != nil {
		return nil, fmt.Errorf("parse events: %w", err)
	}

	events := make([]K8sEvent, 0, len(eventList.Items))
	for _, item := range eventList.Items {
		if !isPACRelated(item.InvolvedObject.Kind, item.InvolvedObject.Name, item.Source.Component) {
			continue
		}

		t := time.Time{}
		if item.LastTimestamp != nil {
			t = *item.LastTimestamp
		} else if item.EventTime != nil {
			t = *item.EventTime
		}

		events = append(events, K8sEvent{
			Time:    t,
			Type:    item.Type,
			Reason:  item.Reason,
			Message: item.Message,
			Object:  item.InvolvedObject.Kind + "/" + item.InvolvedObject.Name,
		})
	}

	// Reverse so newest is first
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}

	return events, nil
}

func isPACRelated(kind, name, component string) bool {
	pacPrefixes := []string{
		"pipelines-as-code",
		"pac-",
		"repository",
	}
	for _, prefix := range pacPrefixes {
		if len(name) >= len(prefix) && name[:len(prefix)] == prefix {
			return true
		}
		if len(component) >= len(prefix) && component[:len(prefix)] == prefix {
			return true
		}
	}
	pacKinds := []string{"Repository", "PipelineRun"}
	for _, k := range pacKinds {
		if kind == k {
			return true
		}
	}
	return false
}
