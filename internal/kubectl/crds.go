package kubectl

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type PipelineRunInfo struct {
	Name      string
	Status    string
	EventType string
	SHA       string
	Completed time.Time
}

type RepositoryStatus struct {
	Namespace    string
	Name         string
	PipelineRuns []PipelineRunInfo
}

func CheckRepositoryCRD(ctx context.Context, kubeconfig string) (bool, error) {
	_, err := RunKubectl(ctx, kubeconfig,
		"get", "crd", "repositories.pipelinesascode.tekton.dev",
		"--no-headers",
	)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func CheckConfigMap(ctx context.Context, kubeconfig, namespace string) (bool, error) {
	_, err := RunKubectl(ctx, kubeconfig,
		"get", "configmap", "pipelines-as-code",
		"-n", namespace,
		"--no-headers",
	)
	if err != nil {
		return false, nil
	}
	return true, nil
}

func GetRepositoryStatuses(ctx context.Context, kubeconfig string) ([]RepositoryStatus, error) {
	out, err := RunKubectl(ctx, kubeconfig,
		"get", "repositories.pipelinesascode.tekton.dev",
		"-A", "-o", "json",
	)
	if err != nil {
		return nil, err
	}

	var repoList struct {
		Items []struct {
			Metadata struct {
				Name      string `json:"name"`
				Namespace string `json:"namespace"`
			} `json:"metadata"`
			Status *struct {
				Conditions []struct {
					Status string `json:"status"`
				} `json:"conditions"`
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
			} `json:"status"`
		} `json:"items"`
	}
	if err := json.Unmarshal(out, &repoList); err != nil {
		return nil, fmt.Errorf("parse repository list: %w", err)
	}

	repos := make([]RepositoryStatus, 0, len(repoList.Items))
	for _, item := range repoList.Items {
		repo := RepositoryStatus{
			Namespace: item.Metadata.Namespace,
			Name:      item.Metadata.Name,
		}

		if item.Status != nil {
			for _, pr := range item.Status.PipelineRunStatus {
				info := PipelineRunInfo{
					Name: pr.PipelineRunName,
				}
				if len(pr.Status.Conditions) > 0 {
					info.Status = pr.Status.Conditions[0].Reason
				}
				if pr.EventType != nil {
					info.EventType = *pr.EventType
				}
				if pr.SHA != nil && len(*pr.SHA) >= 7 {
					info.SHA = (*pr.SHA)[:7]
				} else if pr.SHA != nil {
					info.SHA = *pr.SHA
				}
				if pr.CompletionTime != nil {
					info.Completed = *pr.CompletionTime
				}
				repo.PipelineRuns = append(repo.PipelineRuns, info)
			}
		}
		repos = append(repos, repo)
	}
	return repos, nil
}
