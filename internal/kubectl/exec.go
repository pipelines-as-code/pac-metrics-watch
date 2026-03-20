package kubectl

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func RunKubectl(ctx context.Context, kubeconfig string, args ...string) ([]byte, error) {
	if kubeconfig != "" {
		args = append([]string{"--kubeconfig", kubeconfig}, args...)
	}
	cmd := exec.CommandContext(ctx, "kubectl", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		output := strings.TrimSpace(string(out))
		if output == "" {
			return nil, fmt.Errorf("kubectl: %w", err)
		}
		return nil, fmt.Errorf("kubectl: %s: %w", output, err)
	}
	return out, nil
}
