package components

import (
	"strings"
	"testing"
)

func TestCounterSeries(t *testing.T) {
	got := counterSeries([]float64{3, 7, 8, 13})
	want := []float64{4, 1, 5}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestChartRangePadsFlatSeries(t *testing.T) {
	minVal, maxVal := chartRange([]float64{5, 5, 5})
	if minVal >= 5 {
		t.Fatalf("minVal = %v, want value below 5", minVal)
	}
	if maxVal <= 5 {
		t.Fatalf("maxVal = %v, want value above 5", maxVal)
	}
}

func TestRenderDetailPaneCounterNoDeltaActivity(t *testing.T) {
	output := RenderDetailPane("Workqueue Adds", "counter", "42", "+0", "desc", "source", []float64{42, 42, 42}, 80)
	if !strings.Contains(output, "No delta activity") {
		t.Fatalf("output = %q, want no-delta message", output)
	}
}

func TestRenderDetailPaneGaugeRendersChartSummary(t *testing.T) {
	output := RenderDetailPane("Active Workers", "gauge", "6", "+1", "desc", "source", []float64{2, 4, 6, 4}, 80)
	if !strings.Contains(output, "min=") {
		t.Fatalf("output = %q, want min annotation", output)
	}
	if !strings.Contains(output, "samples=4") {
		t.Fatalf("output = %q, want sample count", output)
	}
}

func TestRenderDetailPaneFlatGaugeStillRenders(t *testing.T) {
	output := RenderDetailPane("Queue Depth", "gauge", "5", "+0", "desc", "source", []float64{5, 5, 5}, 36)
	if strings.Contains(output, "Not enough data for chart") {
		t.Fatalf("output = %q, did not expect insufficient-data message", output)
	}
	if !strings.Contains(output, "samples=3") {
		t.Fatalf("output = %q, want sample count", output)
	}
}
