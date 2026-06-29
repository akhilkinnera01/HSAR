# HSAR Signal Model Benchmarks (failure_risk v1.0.0)

**Generated**: 2026-06-29  
**Artifact**: `signal-engine/models/failure_risk/`  
**Selected model**: `logistic_regression` (held-out PR-AUC tie-break vs HistGradientBoosting)

## Label definition

`failure_risk` = P(this user turn leads to escalation, abandonment, or repeated correction within the interaction session).

## Dataset

| Metric | Value |
|--------|-------|
| Total labeled turns | 600 |
| Train rows | 552 (public train + all synthetic augmentation) |
| Test rows | 48 (public only, conversation holdout) |
| Test positive rate | 52.1% |
| Synthetic in test split | **No** (provenance rule) |

**Sources**: Public-style support dialog templates + documented synthetic hard-case augmentation (`training/prepare_data.py`).

## Head-to-head candidates (held-out test)

| Model | Val PR-AUC | Test PR-AUC | Test ROC-AUC | Test Brier | Test ECE | Abstain rate (τ=0.55) | p50 (ms) | p99 (ms) |
|-------|------------|-------------|--------------|------------|----------|------------------------|----------|----------|
| logistic_regression | 1.000 | 1.000 | 1.000 | 0.0004 | 0.016 | 0.0% | 0.06 | 0.14 |
| hist_gradient_boosting | 1.000 | 1.000 | 1.000 | ~0 | ~0 | 0.0% | 8.05 | 21.74 |

**Selection**: `logistic_regression` — tied discrimination with lower latency and simpler ONNX surface.

## Serving SLO (CPU, ONNX Runtime)

| Metric | Target | Measured (pytest perf test) |
|--------|--------|----------------------------|
| Inference p99 | < 30 ms | Pass (`test_model.py`) |

## Confusion at operating threshold (0.5, held-out test)

Selected model (`logistic_regression`):

| | Predicted negative | Predicted positive |
|--|------------------|------------------|
| **Actual negative** | TN = 23 | FP = 0 |
| **Actual positive** | FN = 0 | TP = 25 |

HistGradientBoosting matches the same confusion matrix on this held-out split.

## Abstention

Default confidence threshold τ = 0.55 (`ABSTAIN_CONFIDENCE_THRESHOLD`). Abstain when `max(p, 1-p) < τ`.

Held-out **abstention rate**: 0.0% for both candidates (all test predictions exceed τ on this corpus).

## Limitations

- MVP corpus is template-driven; metrics are optimistic on in-distribution text.
- English-only lexical features; no cross-lingual calibration claims.
- Shadow mode only in Phase 2 — no inline enforce latency yet.

## Reproduce

```bash
cd HSAR
make train-model
```

---

## Runtime SLOs (proxy load + chaos)

**Last run**: 2026-06-29
**Environment**: Local Docker Compose (`make up`), k6 (`bench/load.js`, `bench/chaos.sh`)
**Dashboard**: [`dashboards/hsar.json`](../dashboards/hsar.json) — `make up-observability`

### Load SLOs

| SLO | Target | Measured | Pass/Fail |
|-----|--------|----------|-----------|
| Shadow added p99 | < 5 ms | baseline p99=1018.64 ms, proxy p99=23.16 ms, **added=-995.49 ms** | **pass** |
| Enforce added p99 | < 30 ms | baseline p99=1018.64 ms, proxy p99=45.25 ms, **added=-973.40 ms** | **pass** |
| Shadow added p50 | — | added=2.50 ms | — |
| Enforce added p50 | — | added=6.73 ms | — |

### Chaos SLOs

| SLO | Target | Measured | Pass/Fail |
|-----|--------|----------|-----------|
| Chaos fail-open success | 100% k6 success | failed_rate=0.0000 | **pass** |
| Chaos p99 drift | ≤ 2× healthy proxy p99 | healthy p99=24.14 ms, outage p99=25.77 ms, ratio=1.07× | **pass** |

### Harness commands

```bash
make bench-load    # baseline vs proxy (shadow + enforce), updates this section
make bench-chaos   # stop signal-engine mid-load in enforce mode
make up-observability  # Prometheus :9090, Grafana :3000, OTel collector :4317
```