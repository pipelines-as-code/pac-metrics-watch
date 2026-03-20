# pac-metrics-watch

`pac-metrics-watch` is a standalone debugging tool for Pipelines-as-Code
metrics. It scrapes the PAC controller and watcher metrics endpoints through
`kubectl`, combines the useful PAC signals, and presents them as either:

- an interactive TUI dashboard
- a one-shot report with `--once`

The default dashboard focuses on the metrics that are most helpful when
debugging PAC behavior:

- Git provider API requests
- PipelineRuns created
- Running PipelineRuns
- PipelineRun duration
- Workqueue pressure
- Controller-runtime reconcile health



https://github.com/user-attachments/assets/0bca096f-a621-4e6b-a42b-532daf112630



## Build

Build the tool with:

```bash
make
```

This writes the binary to:

```bash
bin/pac-metrics-watch
```

## Run

Always run the built binary from `bin/`:

```bash
./bin/pac-metrics-watch
```

Useful variants:

```bash
./bin/pac-metrics-watch -endpoint controller
./bin/pac-metrics-watch --once
./bin/pac-metrics-watch --once -output tsv
```

## Metrics Source

The tool scrapes PAC through the Kubernetes service proxy:

- `pipelines-as-code-controller:9090`
- `pipelines-as-code-watcher:9090`

The default scope is `all`, which combines controller and watcher metrics.

## TUI Modes

### Dashboard view

This is the default view. It groups the most important PAC metrics into:

- PAC Flow
- Queue Health
- Reconcile Health

Each row includes:

- current value
- recent delta
- short trend sparkline
- a short explanation of why the signal matters

### Raw view

Raw view exposes the underlying metric families for deeper inspection. It keeps
the fast navigation, sorting, and filtering from the original tool.

## TUI Keys

- `d`: dashboard view
- `r`: raw view
- `tab`: switch scope between `all`, `controller`, and `watcher`
- `j` / `k` or arrow keys: move selection
- `f`: PAC-only filter in raw view
- `s`: change raw sort mode
- `/`: filter raw metric names
- `q`: quit

## Snapshot Mode

Use `--once` to print a single report and exit:

```bash
./bin/pac-metrics-watch --once
```

This is useful for quick checks, scripting, or capturing a single PAC metrics
snapshot without opening the full TUI.

## Development

### Tests

Run:

```bash
go test ./...
```

### Pre-commit

Install the local hooks with:

```bash
pre-commit install
```

Then run them manually with:

```bash
pre-commit run --all-files
```

### Release

This directory includes a standalone Goreleaser config:

```bash
goreleaser release --clean
```

For a local dry run:

```bash
goreleaser build --snapshot --clean
```
