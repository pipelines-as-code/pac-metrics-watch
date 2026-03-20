package metrics

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

func ParseMetrics(data string) (map[string]float64, error) {
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

func ParseWithLabels(data string) (map[string]*MetricFamily, error) {
	result := map[string]*MetricFamily{}
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

		rawName := parts[0]
		name := rawName
		labels := map[string]string{}

		if idx := strings.Index(rawName, "{"); idx >= 0 {
			name = rawName[:idx]
			labelStr := rawName[idx+1 : len(rawName)-1]
			labels = parseLabels(labelStr)
		}

		family, ok := result[name]
		if !ok {
			family = &MetricFamily{Name: name}
			result[name] = family
		}
		family.Samples = append(family.Samples, LabeledSample{
			Labels: labels,
			Value:  val,
		})
		family.Total += val
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan metrics: %w", err)
	}

	return result, nil
}

func parseLabels(s string) map[string]string {
	labels := map[string]string{}
	if s == "" {
		return labels
	}

	for _, pair := range splitLabels(s) {
		eqIdx := strings.Index(pair, "=")
		if eqIdx < 0 {
			continue
		}
		key := pair[:eqIdx]
		val := pair[eqIdx+1:]
		val = strings.Trim(val, "\"")
		labels[key] = val
	}
	return labels
}

// splitLabels splits a Prometheus label string by commas, respecting quoted values.
func splitLabels(s string) []string {
	var result []string
	var current strings.Builder
	inQuotes := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch {
		case ch == '"':
			inQuotes = !inQuotes
			current.WriteByte(ch)
		case ch == ',' && !inQuotes:
			result = append(result, current.String())
			current.Reset()
		default:
			current.WriteByte(ch)
		}
	}
	if current.Len() > 0 {
		result = append(result, current.String())
	}
	return result
}

func InterestingMetric(name string) bool {
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

func CanonicalMetricName(name string) string {
	for _, prefix := range []string{"pac_controller_", "pac_watcher_"} {
		if strings.HasPrefix(name, prefix) {
			return strings.TrimPrefix(name, prefix)
		}
	}
	return name
}

func MatchesSignal(name string, signal SignalDefinition) bool {
	canonical := CanonicalMetricName(name)
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

func MetricAllowed(name string, pacOnly bool, filter string) bool {
	if pacOnly {
		if !strings.HasPrefix(CanonicalMetricName(name), "pipelines_as_code_") {
			return false
		}
	} else if !InterestingMetric(name) {
		return false
	}

	if filter == "" {
		return true
	}
	return strings.Contains(strings.ToLower(name), strings.ToLower(filter))
}
