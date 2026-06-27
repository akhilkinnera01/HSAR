# HSAR
### Human Signal Aware AI Runtime

HSAR is a runtime governance control plane that sits between users and foundation models to improve last-mile AI reliability.

It observes lightweight human interaction signals in real time and governs model behavior using bounded, deterministic controls, without retraining models or increasing GPU usage.

---

## Why HSAR Exists

Modern AI failures are rarely caused by model incapability. They arise from unmanaged human–system feedback loops under real-world latency, cost, and safety constraints.

Prompt engineering, agent frameworks, and RLHF optimize model behavior in isolation. HSAR addresses a different problem: **governing AI behavior at runtime when human interaction begins to fail**.

---

## What HSAR Does

- Runs as a fail-open sidecar between users and models  
- Forecasts interaction failure risk using CPU-only signal inference  
- Applies deterministic, reversible controls to model behavior  
- Degrades gracefully under load or failure  
- Works across proprietary and open-weight models  

HSAR introduces reliability without becoming a new single point of failure.

---

## High-Level Architecture

<p align="center">
  <img src="HSAR Inference Pipeline.png" alt="HSAR Inference Pipeline" width="800">
</p>

HSAR intercepts requests, extracts human interaction signals, evaluates YAML policy rules, and forwards OpenAI-compatible chat completions to a configured upstream. Shadow mode logs counterfactual traces; enforce and canary modes apply bounded request mutations with a 30 ms fail-open budget.

If HSAR exceeds its latency budget or fails, requests pass through unmodified. See [Architecture overview](docs/architecture.md) for the three-plane design and request lifecycle.

---

## Design Principles

- Fail-open by default  
- Strictly bounded latency and cost  
- Deterministic degradation  
- Monotonic safety guarantees  
- Model and vendor agnostic  

---

## Project Status

**Steel thread verified** — proxy, signal engine, and echo backend run end-to-end with tested fail-open behavior and measured runtime SLOs.

| Capability | Status |
|---|---|
| Shadow signal analysis | ✅ Tested |
| Fail-open on engine outage | ✅ Tested (`make smoke`, `make bench-chaos`) |
| Unit + integration tests | ✅ `make test` |
| CI (lint, test, vuln scan) | ✅ `.github/workflows/ci.yml` |
| Real upstream + streaming | ✅ Tested (`make test`, auth in `make smoke`) |
| Tenant API key auth (401) | ✅ Tested |
| Per-tenant rate limiting (429) | ✅ Tested |
| Trained `failure_risk` model (ONNX, CPU) | ✅ Tested (`make test`) |
| Policy engine (shadow counterfactual traces) | ✅ Tested (`make test`) |
| Enforce mode (inline mutation + fail-open) | ✅ Tested (`make test`, `scripts/smoke-enforce.sh`) |
| Canary rollout + kill switch | ✅ Tested (`make test`) |
| OTel metrics/traces + `/metrics` | ✅ Tested (`make test`) |
| Load + chaos benchmarks | ✅ Measured (`make bench-load`, `make bench-chaos` → [docs/benchmarks.md](docs/benchmarks.md)) |
| Grafana dashboard | ✅ [`dashboards/hsar.json`](dashboards/hsar.json) + `make up-observability` |
| End-to-end demo | ✅ `make demo` → [docs/demo.md](docs/demo.md) |

### Runtime SLOs (measured 2026-06-27)

| SLO | Target | Result |
|-----|--------|--------|
| Shadow added p99 | < 5 ms | **pass** |
| Enforce added p99 | < 30 ms | **pass** |
| Chaos fail-open success | 100% | **pass** |
| Chaos p99 drift | ≤ 2× healthy | **pass** (1.07×) |

Full numbers: [docs/benchmarks.md — Runtime SLOs](docs/benchmarks.md#runtime-slos-proxy-load--chaos).

<p align="center">
  <img src="docs/assets/grafana-dashboard.png" alt="HSAR Grafana governance dashboard" width="700">
</p>

---

## Quick Start

```bash
make test          # unit tests (Go + Python)
make up            # docker compose up (echo upstream)
make smoke         # auth + shadow + fail-open check
make demo          # enforce-mode governance walkthrough (2 min)
make up-observability  # Prometheus + Grafana + OTel collector
make bench-load    # runtime SLO load harness (requires k6)
make bench-chaos   # fail-open under outage (requires k6)

# Optional: real model via Ollama
docker compose --profile ollama up -d
# set UPSTREAM_BASE_URL=http://ollama:11434 in proxy env
```

Use `Authorization: Bearer dev-key-1` (default dev tenant) for chat requests.

### Demo preview

<p align="center">
  <img src="docs/assets/demo.gif" alt="HSAR make demo — calm passthrough then governance" width="700">
</p>

---

## Documentation

- [Architecture overview](docs/architecture.md) — three planes, lifecycle, fail-open guarantees
- [Threat model](docs/threat-model.md) — trust boundaries, privacy, residual risks
- [Demo guide](docs/demo.md) — `make demo` prerequisites and GIF regeneration
- [Operational runbook](docs/runbook.md) — enforce rollout, kill switch, dashboard triage
- [Policy engine design](docs/policy-engine.md) — FSM, shadow + enforce rollout, kill switch
- [Signal model benchmarks](docs/benchmarks.md) — held-out metrics + Runtime SLOs
- [failure_risk model card](signal-engine/models/failure_risk/model_card.md) — data, limits, version
- [Architecture status](current_architecture.txt) — implementation snapshot
- [Contributing](CONTRIBUTING.md) — setup, tests, CI gates

---

## License

Apache 2.0