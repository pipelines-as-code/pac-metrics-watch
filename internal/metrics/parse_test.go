package metrics

import "testing"

func TestInterestingMetricIncludesRuntimeCollectors(t *testing.T) {
	for _, name := range []string{
		"go_memstats_heap_inuse_bytes",
		"go_goroutines",
		"process_resident_memory_bytes",
	} {
		if !InterestingMetric(name) {
			t.Fatalf("InterestingMetric(%q) = false, want true", name)
		}
	}
}

func TestBuildRowsFromHistoryIncludesRuntimeCollectors(t *testing.T) {
	rows := BuildRowsFromHistory(
		map[string][]float64{
			"go_memstats_heap_inuse_bytes":   {64, 96},
			"process_resident_memory_bytes":  {128, 192},
			"pipelines_as_code_metric_total": {1, 2},
		},
		map[string]float64{
			"go_memstats_heap_inuse_bytes":   32,
			"process_resident_memory_bytes":  64,
			"pipelines_as_code_metric_total": 1,
		},
		false,
		"mem",
		SortByAlpha,
	)

	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Name != "go_memstats_heap_inuse_bytes" {
		t.Fatalf("rows[0].Name = %q, want go_memstats_heap_inuse_bytes", rows[0].Name)
	}
	if rows[1].Name != "process_resident_memory_bytes" {
		t.Fatalf("rows[1].Name = %q, want process_resident_memory_bytes", rows[1].Name)
	}
}
