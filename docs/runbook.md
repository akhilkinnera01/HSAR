# HSAR Operational Runbook

Procedures for moving from shadow to enforce, operating the kill switch, reading governance health on Grafana, and rolling policy versions.

See [architecture.md](architecture.md) for mode definitions and [threat-model.md](threat-model.md) for blast-radius context.

---

## Prerequisites

- Local stack: `make up` (or production equivalent)
- Default dev key: `Authorization: Bearer dev-key-1`
- Observability (dashboard section): `make up-observability`

Environment variables on **proxy** service (`docker-compose.yml`):

| Variable | Purpose | Default |
|----------|---------|---------|
| `MODE` | `shadow`, `canary`, `enforce` | `shadow` |
| `CANARY_PCT` | Inline enforce cohort % | `0` |
| `ENFORCE_KILL_SWITCH` | Instant passthrough | `false` |
| `POLICY_PATH` | Policy YAML (in image) | `/policies/standard-safety-policy.yaml` |

---

## 1. Shadow baseline

**Purpose**: Safe default—counterfactual traces only, no mutation.

```bash
MODE=shadow ENFORCE_KILL_SWITCH=false docker compose up -d proxy
make smoke
```

**Verify**:

- HTTP 200 on chat completion
- Proxy logs contain `policy_trace` with `enforce_applied=false`

**Rollback**: N/A (recommended starting state).

---

## 2. Canary rollout

**Purpose**: Inline enforce on a deterministic subset before full rollout.

> **Warning**: Enforce mutations affect live traffic. Always canary before full enforce.

```bash
MODE=canary CANARY_PCT=10 ENFORCE_KILL_SWITCH=false docker compose up --build -d proxy
```

**Verify**:

- Send requests with different `X-Request-ID` values
- ~10% show `enforce_applied=true` in `policy_trace` (inline path)
- Remainder show shadow-only traces

```bash
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer dev-key-1" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: canary-check-1" \
  -d '{"messages":[{"role":"user","content":"test"}]}'
docker compose logs proxy | grep policy_trace | tail -3
```

**Rollback**:

```bash
MODE=shadow CANARY_PCT=0 docker compose up -d proxy
```

---

## 3. Full enforce

**Purpose**: Inline governance on all qualifying requests.

```bash
MODE=enforce ENFORCE_KILL_SWITCH=false docker compose up --build -d proxy
./scripts/smoke-enforce.sh
```

**Verify**:

- `smoke-enforce.sh` passes
- High-frustration prompts show governance actions in `policy_trace`

**Rollback**:

```bash
MODE=shadow docker compose up -d proxy
```

---

## 4. Kill switch activation

**Purpose**: Immediately stop all enforcement mutations while keeping traces.

```bash
ENFORCE_KILL_SWITCH=true MODE=enforce docker compose up -d proxy
```

**Verify**:

```bash
curl -s -X POST http://localhost:8080/v1/chat/completions \
  -H "Authorization: Bearer dev-key-1" \
  -H "Content-Type: application/json" \
  -H "X-Request-ID: kill-switch-check" \
  -d '{"messages":[{"role":"user","content":"THIS IS UNACCEPTABLE!!!"}]}'
curl -s http://localhost:8080/metrics | grep hsar_kill_switch_passthrough
```

- Requests return 200 (passthrough)
- `hsar_kill_switch_passthrough_total` increases after traffic
- No request mutation despite angry prompt

**Rollback**:

```bash
ENFORCE_KILL_SWITCH=false docker compose up -d proxy
```

---

## 5. Dashboard triage

**Start stack**:

```bash
make up-observability
```

| URL | Service |
|-----|---------|
| http://localhost:3000 | Grafana (admin/admin) |
| http://localhost:9090 | Prometheus |
| http://localhost:8080/metrics | Proxy metrics |

Import [`dashboards/hsar.json`](../dashboards/hsar.json). Panels map to:

| Panel | Prometheus query (from dashboard) | Incident signal |
|-------|-----------------------------------|-----------------|
| Inline latency p99 | `histogram_quantile(0.99, sum(rate(hsar_inline_duration_seconds_bucket[5m])) by (le))` | Added governance latency |
| Policy eval p99 | `histogram_quantile(0.99, sum(rate(hsar_policy_duration_seconds_bucket[5m])) by (le))` | Policy engine slowness |
| Fail-open rate | `sum(rate(hsar_fail_open_total[5m]))` | Perception/policy/budget passthrough |
| Abstain rate | `rate(hsar_abstain_total[5m])` | Model declining to classify |
| Kill-switch passthrough | `rate(hsar_kill_switch_passthrough_total[5m])` | Operator kill switch active |
| Actions by type | `sum by (action_type) (rate(hsar_action_applied_total[5m]))` | Governance mix |
| Policy transitions | `sum(rate(hsar_policy_state_transition_total[5m]))` | FSM flapping |

**Verify**: Send sample traffic; panels update within ~15s (scrape interval).

Compare latency SLOs: [benchmarks.md — Runtime SLOs](benchmarks.md#runtime-slos-proxy-load--chaos).

---

## 6. Policy version rollout

Policies load at **proxy startup** from `POLICY_PATH` (see `internal/policy/policy.go`). There is no hot reload in MVP.

**Steps**:

1. Edit `policies/standard-safety-policy.yaml` (bump `policy_version`).
2. Rebuild and restart proxy:

```bash
docker compose up --build -d proxy
```

3. Send test traffic and confirm new version in logs:

```bash
docker compose logs proxy | grep policy_version | tail -5
```

**Verify**: `policy_trace` shows updated `policy_version`.

**Rollback**: Restore previous YAML from git and `docker compose up --build -d proxy`.

---

## Related documentation

- [Architecture](architecture.md)
- [Policy engine](policy-engine.md)
- [Demo](demo.md) — `make demo`
- [Threat model](threat-model.md)