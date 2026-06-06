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

| Model | Val PR-AUC | Test PR-AUC | Test ROC-AUC | Test Brier | Test ECE | p50 (ms) | p99 (ms) |
|-------|------------|-------------|--------------|------------|----------|----------|----------|
| logistic_regression | 1.000 | 1.000 | 1.000 | 0.0004 | 0.016 | 0.05 | 0.11 |
| hist_gradient_boosting | 1.000 | 1.000 | 1.000 | ~0 | ~0 | 7.53 | 13.75 |

**Selection**: `logistic_regression` — tied discrimination with lower latency and simpler ONNX surface.

## Serving SLO (CPU, ONNX Runtime)

| Metric | Target | Measured (pytest perf test) |
|--------|--------|----------------------------|
| Inference p99 | < 30 ms | Pass (`test_model.py`) |

## Abstention

Default confidence threshold τ = 0.55 (`ABSTAIN_CONFIDENCE_THRESHOLD`). Abstain when `max(p, 1-p) < τ`.

## Limitations

- MVP corpus is template-driven; metrics are optimistic on in-distribution text.
- English-only lexical features; no cross-lingual calibration claims.
- Shadow mode only in Phase 2 — no inline enforce latency yet.

## Reproduce

```bash
cd HSAR
make train-model
```