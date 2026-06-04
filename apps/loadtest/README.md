# OPC API Load Test

k6 load test that exercises the public surface of the OPC API and asserts
on the SLOs defined in the Phase 1 spec.

## Scenario

| Phase  | Duration | VUs | Notes                                |
|--------|----------|-----|--------------------------------------|
| Ramp   | 30s      | 0→50| Linear ramp                          |
| Hold   | 60s      | 50  | Steady-state                         |
| Ramp   | 10s      | 50→0| Linear ramp down                     |

Total runtime: ~100s.

## Endpoints

- `GET /healthz` — must respond 100% successfully.
- `GET /topics` — list read; p95 must stay under 100 ms.
- `GET /ai/topics` — expected to 503 without an `ANTHROPIC_API_KEY`; the
  SLO we enforce is latency (<500 ms p95), not status, so the run is
  green in CI even when the key is unset.

## Prerequisites

- [k6](https://k6.io/docs/get-started/installation/) v0.49 or later.
- The API server running and reachable.

## Run

```bash
# Local dev (port 48080 by default)
k6 run apps/loadtest/k6-basic.js

# Custom base URL
k6 run -e BASE_URL=https://staging.example.com apps/loadtest/k6-basic.js

# Capture a JSON summary for post-processing
k6 run --out json=summary.json -e BASE_URL=http://localhost:48080 apps/loadtest/k6-basic.js
```

## SLO Thresholds

Defined as k6 `thresholds`; the run fails if any are violated:

| Check                | Threshold          |
|----------------------|--------------------|
| `healthz_ok`         | rate == 1.0        |
| `topics_latency`     | p95 < 100 ms       |
| `ai_topics_latency`  | p95 < 500 ms       |
| `http_req_failed`    | rate < 0.01        |

## CI Integration

Wire into the CI workflow (`.github/workflows/ci.yml`) with a step
that boots the API in the background, waits for `/healthz`, runs k6,
and tears the server down. The `web-tests` and `go-tests` jobs are
good neighbours for this — same Postgres-free, SQLite-backed runtime.

## Prometheus Correlation

While the load test runs, scrape `GET /metrics` on the API in
parallel (e.g. with `watch -n 5 curl -s $BASE_URL/metrics | grep
opc_http_requests_total`) to see the counters climbing in real time.
This is the manual "did the metrics actually record?" sanity check
called out in Phase 1.5 Task D.
