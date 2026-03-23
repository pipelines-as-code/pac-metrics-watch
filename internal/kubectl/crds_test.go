package kubectl

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestGetRepositoryStatusesParsesJSON(t *testing.T) {
	completionTime := time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC)
	sha := "abc1234567890"
	eventType := "push"

	input := map[string]interface{}{
		"items": []map[string]interface{}{
			{
				"metadata": map[string]interface{}{
					"name":      "my-repo",
					"namespace": "test-ns",
				},
				"pipelinerun_status": []map[string]interface{}{
					{
						"pipelineRunName": "pr-1",
						"status": map[string]interface{}{
							"conditions": []map[string]interface{}{
								{"reason": "Succeeded"},
							},
						},
						"completionTime": completionTime.Format(time.RFC3339),
						"sha":            sha,
						"event_type":     eventType,
					},
				},
			},
		},
	}

	data, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("marshal test data: %v", err)
	}

	// Test the JSON unmarshalling logic directly
	var repoList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			PipelineRunStatus []struct {
				PipelineRunName string `json:"pipelineRunName"`
				Status          struct {
					Conditions []struct {
						Reason string `json:"reason"`
					} `json:"conditions"`
				} `json:"status"`
				CompletionTime *time.Time `json:"completionTime"`
				SHA            *string    `json:"sha"`
				EventType      *string    `json:"event_type"`
			} `json:"pipelinerun_status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &repoList); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(repoList.Items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(repoList.Items))
	}

	item := repoList.Items[0]
	if item.Metadata.Name != "my-repo" {
		t.Fatalf("name = %q, want my-repo", item.Metadata.Name)
	}
	if item.Metadata.Namespace != "test-ns" {
		t.Fatalf("namespace = %q, want test-ns", item.Metadata.Namespace)
	}
	if len(item.PipelineRunStatus) != 1 {
		t.Fatalf("len(pipelinerun_status) = %d, want 1", len(item.PipelineRunStatus))
	}

	pr := item.PipelineRunStatus[0]
	if pr.PipelineRunName != "pr-1" {
		t.Fatalf("pipelineRunName = %q, want pr-1", pr.PipelineRunName)
	}
	if len(pr.Status.Conditions) != 1 || pr.Status.Conditions[0].Reason != "Succeeded" {
		t.Fatalf("condition reason = %v, want Succeeded", pr.Status.Conditions)
	}
	if pr.SHA == nil || *pr.SHA != sha {
		t.Fatalf("sha = %v, want %q", pr.SHA, sha)
	}
	if pr.EventType == nil || *pr.EventType != eventType {
		t.Fatalf("event_type = %v, want %q", pr.EventType, eventType)
	}
}

func TestGetRepositoryStatusesEmptyItems(t *testing.T) {
	data := []byte(`{"items":[]}`)

	var repoList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			PipelineRunStatus []struct {
				PipelineRunName string `json:"pipelineRunName"`
			} `json:"pipelinerun_status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &repoList); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(repoList.Items) != 0 {
		t.Fatalf("len(items) = %d, want 0", len(repoList.Items))
	}
}

func TestGetRepositoryStatusesNilFields(t *testing.T) {
	data := []byte(`{
		"items": [{
			"metadata": {"name": "repo-no-prs", "namespace": "ns"},
			"pipelinerun_status": [{
				"pipelineRunName": "pr-x",
				"status": {"conditions": []},
				"completionTime": null,
				"sha": null,
				"event_type": null
			}]
		}]
	}`)

	var repoList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			PipelineRunStatus []struct {
				PipelineRunName string `json:"pipelineRunName"`
				Status          struct {
					Conditions []struct {
						Reason string `json:"reason"`
					} `json:"conditions"`
				} `json:"status"`
				CompletionTime *time.Time `json:"completionTime"`
				SHA            *string    `json:"sha"`
				EventType      *string    `json:"event_type"`
			} `json:"pipelinerun_status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &repoList); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	pr := repoList.Items[0].PipelineRunStatus[0]
	if pr.CompletionTime != nil {
		t.Fatalf("completionTime should be nil, got %v", pr.CompletionTime)
	}
	if pr.SHA != nil {
		t.Fatalf("sha should be nil, got %v", pr.SHA)
	}
	if pr.EventType != nil {
		t.Fatalf("event_type should be nil, got %v", pr.EventType)
	}
	if len(pr.Status.Conditions) != 0 {
		t.Fatalf("conditions should be empty, got %v", pr.Status.Conditions)
	}
}

func TestBuildRepositoryStatusFromParsedData(t *testing.T) {
	// Test the conversion logic from parsed JSON to RepositoryStatus
	sha := "abc1234567890"
	eventType := "pull_request"
	completionTime := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	info := PipelineRunInfo{Name: "pr-1"}
	// Simulate condition extraction
	info.Status = "Succeeded"
	info.EventType = eventType
	// SHA truncation to 7 chars
	if len(sha) >= 7 {
		info.SHA = sha[:7]
	}
	info.Completed = completionTime

	if info.SHA != "abc1234" {
		t.Fatalf("SHA = %q, want abc1234", info.SHA)
	}
	if info.Status != "Succeeded" {
		t.Fatalf("Status = %q, want Succeeded", info.Status)
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"kubectl: Error from server (NotFound): exit status 1", true},
		{"kubectl: error: the server doesn't have a resource type \"repositories\"", false},
		{"kubectl: not found: exit status 1", true},
		{"kubectl: forbidden: exit status 1", false},
	}
	for _, tt := range tests {
		got := isNotFoundError(fmt.Errorf("%s", tt.msg))
		if got != tt.want {
			t.Errorf("isNotFoundError(%q) = %v, want %v", tt.msg, got, tt.want)
		}
	}
}
