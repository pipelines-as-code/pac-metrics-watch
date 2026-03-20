# pac-metrics-watch

## Purpose

`pac-metrics-watch` is a local debugging tool for Pipelines-as-Code metrics.
It scrapes the PAC controller and watcher metrics endpoints through `kubectl`,
aggregates the signals, and presents them either as:

- an interactive TUI dashboard
- a one-shot snapshot report with `--once`

The dashboard is opinionated. It highlights the PAC signals that are most
useful when debugging webhook flow, PipelineRun activity, queue pressure, and
controller health.

## Build And Run

Always build this tool with:

```bash
make
```

This writes the binary to:

```bash
bin/pac-metrics-watch
```

Run the tool from that built binary:

```bash
./bin/pac-metrics-watch
```

Do not run an older binary from the repo root or another location when
debugging changes here.

## What It Scrapes

The tool reads PAC metrics through Kubernetes service proxy endpoints:

- `pipelines-as-code-controller:9090`
- `pipelines-as-code-watcher:9090`

The default scope is `all`, which combines controller and watcher metrics into a
single view. You can also scope to one component with `-endpoint controller` or
`-endpoint watcher`.

## Modes

### Interactive dashboard

Default mode is a TUI dashboard focused on curated PAC signals:

- Git provider API requests
- PipelineRuns created
- Running PipelineRuns
- PipelineRun duration
- Workqueue depth/adds/retries/queue time
- Controller-runtime reconcile health

The TUI also has a raw metrics view for drilling into individual Prometheus
metric families.

### Snapshot mode

Use `--once` to scrape once, print a report, and exit:

```bash
./bin/pac-metrics-watch --once
```

Useful variants:

```bash
./bin/pac-metrics-watch --once -endpoint controller
./bin/pac-metrics-watch --once -output tsv
```

## TUI Keys

- `d`: dashboard view
- `r`: raw metrics view
- `tab`: switch scope between `all`, `controller`, and `watcher`
- `j` / `k` or arrow keys: move selection
- `f`: PAC-only filter in raw view
- `s`: sort raw view
- `/`: filter raw metric names
- `q`: quit

## Implementation Notes

- The tool uses a single-flight polling loop. It does not allow overlapping
  scrapes.
- Scrape errors include real `kubectl` output when available.
- Metrics are aggregated by metric family name. Label values are intentionally
  collapsed in the main dashboard.
- In raw mode, `pac-only` means actual PAC business metrics after stripping the
  exporter prefix, not every `pac_controller_*` or `pac_watcher_*` runtime
  metric.

## Typical Workflow

1. Build with `make`
2. Run `./bin/pac-metrics-watch`
3. Start in dashboard view
4. Switch to raw view only when you need lower-level detail
5. Use `--once` for quick checks or shell-friendly output
