package views

import (
	"strings"
	"testing"
	"time"

	"github.com/openshift-pipelines/pipelines-as-code/hack/pac-metrics-watch/internal/kubectl"
)

func TestRenderHealthSnapshot(t *testing.T) {
	checks := []HealthCheck{
		{Name: "Pod: pac-controller", Status: "pass", Detail: "Phase=Running"},
		{Name: "Repository CRD", Status: "fail", Detail: "not found"},
		{Name: "ConfigMap", Status: "warn", Detail: "not found in pac-ns"},
	}

	output := RenderHealthSnapshot(checks)

	if !strings.Contains(output, "[PASS]") {
		t.Error("output should contain [PASS]")
	}
	if !strings.Contains(output, "[FAIL]") {
		t.Error("output should contain [FAIL]")
	}
	if !strings.Contains(output, "[WARN]") {
		t.Error("output should contain [WARN]")
	}
	if !strings.Contains(output, "pac-controller") {
		t.Error("output should contain check name")
	}
	if !strings.Contains(output, "Phase=Running") {
		t.Error("output should contain check detail")
	}
}

func TestRenderHealthSnapshotEmpty(t *testing.T) {
	output := RenderHealthSnapshot(nil)
	if !strings.Contains(output, "PAC Installation Health") {
		t.Error("output should contain title even with no checks")
	}
}

func TestRenderReposSnapshot(t *testing.T) {
	completionTime := time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC)
	repos := []kubectl.RepositoryStatus{
		{
			Namespace: "test-ns",
			Name:      "my-repo",
			PipelineRuns: []kubectl.PipelineRunInfo{
				{
					Name:      "pr-run-1",
					Status:    "Succeeded",
					EventType: "push",
					SHA:       "abc1234",
					Completed: completionTime,
				},
			},
		},
		{
			Namespace: "other-ns",
			Name:      "empty-repo",
		},
	}

	output := RenderReposSnapshot(repos)

	if !strings.Contains(output, "test-ns") {
		t.Error("output should contain namespace")
	}
	if !strings.Contains(output, "my-repo") {
		t.Error("output should contain repo name")
	}
	if !strings.Contains(output, "Succeeded") {
		t.Error("output should contain status")
	}
	if !strings.Contains(output, "empty-repo") {
		t.Error("output should contain empty repo")
	}
	if !strings.Contains(output, "n/a") {
		t.Error("output should contain n/a for empty repo")
	}
}

func TestRenderEventsSnapshot(t *testing.T) {
	events := []kubectl.K8sEvent{
		{
			Time:    time.Date(2025, 3, 15, 10, 0, 0, 0, time.UTC),
			Type:    "Normal",
			Reason:  "Started",
			Message: "Pipeline started successfully",
			Object:  "PipelineRun/pr-1",
		},
		{
			Time:    time.Time{},
			Type:    "Warning",
			Reason:  "Failed",
			Message: "Pipeline failed due to timeout",
			Object:  "PipelineRun/pr-2",
		},
	}

	output := RenderEventsSnapshot(events)

	if !strings.Contains(output, "Normal") {
		t.Error("output should contain event type")
	}
	if !strings.Contains(output, "Warning") {
		t.Error("output should contain warning type")
	}
	if !strings.Contains(output, "Started") {
		t.Error("output should contain reason")
	}
	if !strings.Contains(output, "unknown") {
		t.Error("output should contain 'unknown' for zero time")
	}
}

func TestRenderEventsSnapshotTruncatesLongMessages(t *testing.T) {
	longMsg := strings.Repeat("x", 100)
	events := []kubectl.K8sEvent{
		{
			Time:    time.Now(),
			Type:    "Normal",
			Reason:  "Test",
			Message: longMsg,
			Object:  "Pod/test",
		},
	}

	output := RenderEventsSnapshot(events)

	if strings.Contains(output, longMsg) {
		t.Error("output should truncate long messages")
	}
	if !strings.Contains(output, "...") {
		t.Error("output should contain ellipsis for truncated messages")
	}
}

func TestRenderResourcesSnapshot(t *testing.T) {
	output := RenderResourcesSnapshot("all", time.Unix(0, 0).UTC(), ResourceFocusMemory, []ComponentResources{
		{
			Name:         "Controller",
			PrimaryTitle: "Controller Container Memory",
			PrimaryValue: "64MiB",
			PrimaryDelta: "n/a",
			Stats: []ResourceStat{
				{Label: "RSS", Value: "48MiB", Delta: "n/a", Available: true},
			},
		},
	}, "", "table")

	if !strings.Contains(output, "view:\tresources") {
		t.Fatalf("output missing resources view metadata: %q", output)
	}
	if !strings.Contains(output, "Controller Container Memory") {
		t.Fatalf("output missing primary metric title: %q", output)
	}
	if !strings.Contains(output, "RSS") {
		t.Fatalf("output missing stats row: %q", output)
	}
}
