"""Evaluate candidate models and emit benchmark metrics."""

from __future__ import annotations

import json
import time
from pathlib import Path

import numpy as np
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import (
    average_precision_score,
    brier_score_loss,
    roc_auc_score,
)
from sklearn.model_selection import train_test_split
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from model.features import extract_features
from training.prepare_data import conversation_safe_split, load_dataset, write_dataset
ROOT = Path(__file__).resolve().parents[1]
BENCHMARK_JSON = ROOT / "models" / "failure_risk" / "benchmark_metrics.json"
OPERATING_THRESHOLD = 0.5
ABSTAIN_TAU = 0.55


def _ece(y_true: np.ndarray, probs: np.ndarray, n_bins: int = 10) -> float:
    bins = np.linspace(0.0, 1.0, n_bins + 1)
    ece = 0.0
    for i in range(n_bins):
        mask = (probs >= bins[i]) & (probs < bins[i + 1])
        if not np.any(mask):
            continue
        acc = float(np.mean(y_true[mask]))
        conf = float(np.mean(probs[mask]))
        ece += abs(acc - conf) * (np.sum(mask) / len(y_true))
    return float(ece)


def _abstention_rate(probs: np.ndarray, tau: float = ABSTAIN_TAU) -> float:
    certainty = np.maximum(probs, 1.0 - probs)
    return float(np.mean(certainty < tau))


def _confusion_at_threshold(
    y_true: np.ndarray, probs: np.ndarray, threshold: float = OPERATING_THRESHOLD
) -> dict:
    preds = (probs >= threshold).astype(int)
    tp = int(np.sum((preds == 1) & (y_true == 1)))
    tn = int(np.sum((preds == 0) & (y_true == 0)))
    fp = int(np.sum((preds == 1) & (y_true == 0)))
    fn = int(np.sum((preds == 0) & (y_true == 1)))
    return {
        "threshold": threshold,
        "tp": tp,
        "tn": tn,
        "fp": fp,
        "fn": fn,
    }


def _latency_percentiles(pipeline, texts: list[str], n_iter: int = 200) -> dict:
    samples = texts[: min(len(texts), n_iter)]
    timings: list[float] = []
    for text in samples:
        x = extract_features(text).reshape(1, -1)
        start = time.perf_counter()
        pipeline.predict_proba(x)
        timings.append((time.perf_counter() - start) * 1000)
    arr = np.array(timings)
    return {
        "p50_ms": float(np.percentile(arr, 50)),
        "p99_ms": float(np.percentile(arr, 99)),
        "samples": len(timings),
    }


def evaluate(seed: int = 42) -> dict:
    write_dataset()
    rows = load_dataset()
    train_rows, test_rows = conversation_safe_split(rows)

    x_train_full = np.vstack([extract_features(r["text"]) for r in train_rows])
    y_train_full = np.array([int(r["label_failure"]) for r in train_rows], dtype=np.int64)
    x_test = np.vstack([extract_features(r["text"]) for r in test_rows])
    y_test = np.array([int(r["label_failure"]) for r in test_rows], dtype=np.int64)

    x_train, x_val, y_train, y_val = train_test_split(
        x_train_full, y_train_full, test_size=0.2, random_state=seed, stratify=y_train_full
    )

    candidates = {
        "logistic_regression": Pipeline(
            [
                ("scaler", StandardScaler()),
                ("clf", LogisticRegression(max_iter=1000, random_state=seed)),
            ]
        ),
        "hist_gradient_boosting": Pipeline(
            [
                (
                    "clf",
                    HistGradientBoostingClassifier(
                        max_depth=4,
                        learning_rate=0.1,
                        max_iter=200,
                        random_state=seed,
                    ),
                ),
            ]
        ),
    }

    report: dict = {
        "dataset": {
            "total_rows": len(rows),
            "train_rows": len(train_rows),
            "test_rows": len(test_rows),
            "test_source": "public_only_conversation_holdout",
            "positive_rate_test": float(np.mean(y_test)),
        },
        "candidates": [],
        "selected": None,
    }

    best_name = None
    best_pr = -1.0
    test_texts = [r["text"] for r in test_rows]

    for name, pipe in candidates.items():
        pipe.fit(x_train, y_train)
        val_probs = pipe.predict_proba(x_val)[:, 1]
        test_probs = pipe.predict_proba(x_test)[:, 1]
        val_pr = float(average_precision_score(y_val, val_probs))
        metrics = {
            "name": name,
            "val_pr_auc": val_pr,
            "test_pr_auc": float(average_precision_score(y_test, test_probs)),
            "test_roc_auc": float(roc_auc_score(y_test, test_probs)),
            "test_brier": float(brier_score_loss(y_test, test_probs)),
            "test_ece": _ece(y_test, test_probs),
            "test_abstention_rate": _abstention_rate(test_probs),
            "confusion_at_threshold": _confusion_at_threshold(y_test, test_probs),
            "latency": _latency_percentiles(pipe, test_texts),
        }
        report["candidates"].append(metrics)
        if val_pr > best_pr:
            best_pr = val_pr
            best_name = name

    report["selected"] = best_name
    BENCHMARK_JSON.parent.mkdir(parents=True, exist_ok=True)
    BENCHMARK_JSON.write_text(json.dumps(report, indent=2), encoding="utf-8")
    print(json.dumps(report, indent=2))
    return report


if __name__ == "__main__":
    evaluate()