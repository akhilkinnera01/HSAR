"""Train candidate classifiers for failure_risk."""

from __future__ import annotations

import json
import pickle
from dataclasses import dataclass
from pathlib import Path

import numpy as np
from sklearn.ensemble import HistGradientBoostingClassifier
from sklearn.linear_model import LogisticRegression
from sklearn.metrics import average_precision_score
from sklearn.model_selection import train_test_split
from sklearn.pipeline import Pipeline
from sklearn.preprocessing import StandardScaler

from model.features import extract_features
from training.prepare_data import conversation_safe_split, load_dataset, write_dataset

ROOT = Path(__file__).resolve().parents[1]
ARTIFACT_DIR = ROOT / "models" / "failure_risk"
PICKLE_PATH = ARTIFACT_DIR / "selected_model.pkl"
META_PATH = ARTIFACT_DIR / "train_meta.json"


@dataclass
class CandidateResult:
    name: str
    pipeline: Pipeline
    val_pr_auc: float


def _rows_to_xy(rows: list[dict]) -> tuple[np.ndarray, np.ndarray]:
    x = np.vstack([extract_features(r["text"]) for r in rows])
    y = np.array([int(r["label_failure"]) for r in rows], dtype=np.int64)
    return x, y


def train_candidates(seed: int = 42) -> CandidateResult:
    write_dataset()
    rows = load_dataset()
    train_rows, _ = conversation_safe_split(rows)

    x, y = _rows_to_xy(train_rows)
    x_train, x_val, y_train, y_val = train_test_split(
        x, y, test_size=0.2, random_state=seed, stratify=y
    )

    candidates = [
        (
            "logistic_regression",
            Pipeline(
                [
                    ("scaler", StandardScaler()),
                    (
                        "clf",
                        LogisticRegression(max_iter=1000, random_state=seed),
                    ),
                ]
            ),
        ),
        (
            "hist_gradient_boosting",
            Pipeline(
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
        ),
    ]

    results: list[CandidateResult] = []
    for name, pipe in candidates:
        pipe.fit(x_train, y_train)
        val_scores = pipe.predict_proba(x_val)[:, 1]
        pr_auc = float(average_precision_score(y_val, val_scores))
        results.append(CandidateResult(name=name, pipeline=pipe, val_pr_auc=pr_auc))

    winner = max(results, key=lambda r: r.val_pr_auc)
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    with PICKLE_PATH.open("wb") as f:
        pickle.dump(winner.pipeline, f)

    meta = {
        "selected": winner.name,
        "candidates": [
            {"name": r.name, "val_pr_auc": r.val_pr_auc} for r in results
        ],
    }
    META_PATH.write_text(json.dumps(meta, indent=2), encoding="utf-8")
    print(json.dumps(meta, indent=2))
    return winner


if __name__ == "__main__":
    train_candidates()