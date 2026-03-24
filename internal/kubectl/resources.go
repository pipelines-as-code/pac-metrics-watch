package kubectl

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ContainerUsage struct {
	PodName        string
	ContainerName  string
	Component      string
	CPUmilli       int64
	MemoryBytes    int64
}

func GetPACContainerUsages(ctx context.Context, kubeconfig, namespace string) ([]ContainerUsage, error) {
	pods, err := GetPACPods(ctx, kubeconfig, namespace)
	if err != nil {
		return nil, err
	}
	if len(pods) == 0 {
		return nil, nil
	}

	componentsByPod := make(map[string]string, len(pods))
	for _, pod := range pods {
		componentsByPod[pod.Name] = pod.Component
	}

	out, err := RunKubectl(ctx, kubeconfig,
		"top", "pod",
		"-n", namespace,
		"--containers",
		"--no-headers",
	)
	if err != nil {
		return nil, err
	}

	return parseTopPodContainers(string(out), componentsByPod)
}

func parseTopPodContainers(data string, componentsByPod map[string]string) ([]ContainerUsage, error) {
	lines := strings.Split(strings.TrimSpace(data), "\n")
	usages := make([]ContainerUsage, 0, len(lines))

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 4 {
			return nil, fmt.Errorf("parse kubectl top output: unexpected line %q", line)
		}

		podName := fields[0]
		component, ok := componentsByPod[podName]
		if !ok {
			continue
		}

		cpu, err := parseCPUQuantity(fields[len(fields)-2])
		if err != nil {
			return nil, fmt.Errorf("parse cpu quantity %q: %w", fields[len(fields)-2], err)
		}
		mem, err := parseBytesQuantity(fields[len(fields)-1])
		if err != nil {
			return nil, fmt.Errorf("parse memory quantity %q: %w", fields[len(fields)-1], err)
		}

		usages = append(usages, ContainerUsage{
			PodName:       podName,
			ContainerName: strings.Join(fields[1:len(fields)-2], " "),
			Component:     component,
			CPUmilli:      cpu,
			MemoryBytes:   mem,
		})
	}

	return usages, nil
}

func parseCPUQuantity(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasSuffix(raw, "n"):
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, "n"), 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(value / 1_000_000)), nil
	case strings.HasSuffix(raw, "u"):
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, "u"), 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(value / 1_000)), nil
	case strings.HasSuffix(raw, "m"):
		value, err := strconv.ParseFloat(strings.TrimSuffix(raw, "m"), 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(value)), nil
	default:
		value, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return 0, err
		}
		return int64(math.Round(value * 1000)), nil
	}
}

func parseBytesQuantity(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	units := map[string]float64{
		"Ki": 1024,
		"Mi": 1024 * 1024,
		"Gi": 1024 * 1024 * 1024,
		"Ti": 1024 * 1024 * 1024 * 1024,
		"K":  1000,
		"M":  1000 * 1000,
		"G":  1000 * 1000 * 1000,
		"T":  1000 * 1000 * 1000 * 1000,
	}

	for unit, scale := range units {
		if strings.HasSuffix(raw, unit) {
			value, err := strconv.ParseFloat(strings.TrimSuffix(raw, unit), 64)
			if err != nil {
				return 0, err
			}
			return int64(math.Round(value * scale)), nil
		}
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(value)), nil
}
