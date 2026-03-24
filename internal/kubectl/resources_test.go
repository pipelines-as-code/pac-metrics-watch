package kubectl

import "testing"

func TestParseTopPodContainers(t *testing.T) {
	out := `
controller-pod controller 25m 120Mi
watcher-pod watcher 5m 48Mi
ignored-pod other 1m 1Mi
`
	got, err := parseTopPodContainers(out, map[string]string{
		"controller-pod": "controller",
		"watcher-pod":    "watcher",
	})
	if err != nil {
		t.Fatalf("parseTopPodContainers returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].Component != "controller" || got[0].CPUmilli != 25 {
		t.Fatalf("got[0] = %#v", got[0])
	}
	if got[1].MemoryBytes <= 48*1024*1024-1 {
		t.Fatalf("got[1].MemoryBytes = %d, want about 48Mi", got[1].MemoryBytes)
	}
}

func TestParseCPUQuantity(t *testing.T) {
	tests := []struct {
		raw  string
		want int64
	}{
		{"250m", 250},
		{"2", 2000},
		{"1500u", 2},
	}

	for _, test := range tests {
		got, err := parseCPUQuantity(test.raw)
		if err != nil {
			t.Fatalf("parseCPUQuantity(%q) returned error: %v", test.raw, err)
		}
		if got != test.want {
			t.Fatalf("parseCPUQuantity(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
}

func TestParseBytesQuantity(t *testing.T) {
	tests := []struct {
		raw  string
		want int64
	}{
		{"64Mi", 64 * 1024 * 1024},
		{"1Gi", 1024 * 1024 * 1024},
		{"500M", 500 * 1000 * 1000},
	}

	for _, test := range tests {
		got, err := parseBytesQuantity(test.raw)
		if err != nil {
			t.Fatalf("parseBytesQuantity(%q) returned error: %v", test.raw, err)
		}
		if got != test.want {
			t.Fatalf("parseBytesQuantity(%q) = %d, want %d", test.raw, got, test.want)
		}
	}
}
