import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const proxyURL = __ENV.PROXY_URL || 'http://localhost:8080';
const baselineURL = __ENV.BASELINE_URL || 'http://localhost:8081';
const apiKey = __ENV.API_KEY || 'dev-key-1';
const scenario = __ENV.SCENARIO || 'proxy';
const duration = __ENV.DURATION || '2m';
const vus = Number(__ENV.VUS || 50);

const latencyTrend = new Trend('hsar_latency_ms', true);

export const options = {
  vus,
  duration,
  summaryTrendStats: ['avg', 'min', 'med', 'max', 'p(90)', 'p(95)', 'p(99)'],
  thresholds: {
    http_req_failed: ['rate==0'],
  },
};

function payload() {
  return JSON.stringify({
    model: 'echo',
    messages: [{ role: 'user', content: 'benchmark ping' }],
  });
}

function headers() {
  return {
    'Content-Type': 'application/json',
    Authorization: `Bearer ${apiKey}`,
    'X-Request-ID': `bench-${__VU}-${__ITER}-${Date.now()}`,
  };
}

export default function () {
  const base = scenario === 'baseline' ? baselineURL : proxyURL;
  const res = http.post(`${base}/v1/chat/completions`, payload(), { headers: headers() });
  check(res, { 'status is 200': (r) => r.status === 200 });
  latencyTrend.add(res.timings.duration);
  sleep(0.1);
}

export function handleSummary(data) {
  const p50 = data.metrics.hsar_latency_ms?.values?.['p(50)'] ?? data.metrics.http_req_duration?.values?.['p(50)'];
  const p99 = data.metrics.hsar_latency_ms?.values?.['p(99)'] ?? data.metrics.http_req_duration?.values?.['p(99)'];
  return {
    stdout: JSON.stringify({ scenario, p50_ms: p50, p99_ms: p99, failed_rate: data.metrics.http_req_failed?.values?.rate ?? 0 }),
  };
}