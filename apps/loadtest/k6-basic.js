// k6 load test for the OPC API.
//
// Scenario:
//   - ramp 0 → 50 VUs over 30s
//   - hold 50 VUs for 60s
//   - ramp 50 → 0 over 10s
//
// Endpoints exercised:
//   - GET /healthz   — must respond 100% successfully (liveness probe).
//   - GET /topics    — must keep p95 < 100ms (CRUD SLO from the spec).
//   - GET /ai/topics — expected to 503 without an ANTHROPIC_API_KEY,
//     but must return FAST (we assert the response time, not the status).
//
// The k6 `checks` API is used so a pass/fail line is emitted per
// endpoint on every summary run — the CI job can grep the JSON
// summary for failed checks to fail the build.
//
// Run:
//   k6 run -e BASE_URL=http://localhost:48080 apps/loadtest/k6-basic.js
//   k6 run --out json=summary.json -e BASE_URL=... apps/loadtest/k6-basic.js

import http from 'k6/http';
import { check } from 'k6';
import { Trend } from 'k6/metrics';

// BASE_URL defaults to the local dev port from .env.example. The
// `-e BASE_URL=...` form overrides it for staging / CI runs.
const BASE_URL = __ENV.BASE_URL || 'http://localhost:48080';

// Endpoint paths in one place — if a route is renamed or prefixed
// (e.g. /v1/ai/topics) only these constants need to change. Keep
// this list in sync with cmd/server/main.go and the OpenAPI spec.
const HEALTHZ_PATH = '/healthz';
const TOPICS_PATH = '/topics';
const AI_TOPICS_PATH = '/ai/topics';

// Per-endpoint latency trends. k6's built-in `http_req_duration`
// aggregates across all URLs, but we want a per-endpoint view in
// the summary so the operator can see at a glance whether /topics
// p95 is the SLO violator.
const topicsLatency = new Trend('topics_latency', true);
const healthLatency = new Trend('health_latency', true);
const aiTopicsLatency = new Trend('ai_topics_latency', true);

export const options = {
  stages: [
    { duration: '30s', target: 50 },
    { duration: '60s', target: 50 },
    { duration: '10s', target: 0 },
  ],
  thresholds: {
    // SLOs from docs/superpowers/specs/opc-phase1-ip-design.md:
    //   /healthz: 100% success
    //   /topics:   p95 < 100ms
    // /ai/topics is allowed to fail (no key in CI) but must be fast
    // so a slow upstream doesn't skew the rest of the scenario.
    //
    // http_req_failed is intentionally NOT set globally — /ai/topics
    // returns 503 when ANTHROPIC_API_KEY is missing, which would
    // contribute to that rate and falsely fail CI. The per-endpoint
    // checks below are the authoritative success signal.
    'checks{check:healthz_ok}': ['rate==1.0'],
    'topics_latency': ['p(95)<100'],
    'ai_topics_latency': ['p(95)<500'],
  },
};

export default function () {
  // 1) /healthz — liveness probe. One call per iteration.
  const healthRes = http.get(`${BASE_URL}${HEALTHZ_PATH}`);
  healthLatency.add(healthRes.timings.duration);
  check(healthRes, {
    healthz_ok: (r) => r.status === 200,
  }, { check: 'healthz_ok' });

  // 2) /topics — main CRUD read. Most traffic in a real session
  //    hits this route (list view), so we hit it 3x per iteration
  //    to model a realistic read-heavy mix.
  for (let i = 0; i < 3; i++) {
    const topicsRes = http.get(`${BASE_URL}${TOPICS_PATH}`);
    topicsLatency.add(topicsRes.timings.duration);
    check(topicsRes, {
      topics_ok: (r) => r.status === 200,
    }, { check: 'topics_ok' });
  }

  // 3) /ai/topics — expected to be 503 without a key, but the
  //    SLO we care about is latency. We check latency only, not
  //    status, so the run is green in CI even when ANTHROPIC_API_KEY
  //    is unset. The 500ms budget assumes the absence-of-key branch
  //    is detected cheaply (env check, no upstream call); if the
  //    handler is ever changed to attempt a request before failing
  //    fast, this threshold should be revisited.
  const aiRes = http.get(`${BASE_URL}${AI_TOPICS_PATH}`);
  aiTopicsLatency.add(aiRes.timings.duration);
  check(aiRes, {
    ai_topics_fast: (r) => r.timings.duration < 500,
  }, { check: 'ai_topics_fast' });
}
