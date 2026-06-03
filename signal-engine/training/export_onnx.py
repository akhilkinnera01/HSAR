"""Export selected sklearn pipeline to ONNX and persist calibration map."""

from __future__ import annotations

import json
import pickle
from pathlib import Path

import numpy as np
from skl2onnx import convert_sklearn
from skl2onnx.common.data_types import FloatTensorType
from sklearn.isotonic import IsotonicRegression
from sklearn.model_selection import train_test_split

from model.features import FEATURE_NAMES, extract_features
from training.prepare_data import conversation_safe_split, load_dataset, write_dataset
from training.train import PICKLE_PATH, train_candidates

ROOT = Path(__file__).resolve().parents[1]
ARTIFACT_DIR = ROOT / "models" / "failure_risk"
ONNX_PATH = ARTIFACT_DIR / "model.onnx"
CALIBRATION_PATH = ARTIFACT_DIR / "calibration.json"
MODEL_VERSION = "failure_risk-1.0.0"
FEATURE_SCHEMA_VERSION = "fv-1"
DEFAULT_ABSTAIN_TAU = 0.55


def _fit_calibration(pipeline, rows: list[dict]) -> dict:
    x = np.vstack([extract_features(r["text"]) for r in rows])
    y = np.array([int(r["label_failure"]) for r in rows], dtype=np.int64)
    _, x_cal, _, y_cal = train_test_split(
        x, y, test_size=0.25, random_state=42, stratify=y
    )
    raw_scores = pipeline.predict_proba(x_cal)[:, 1]

    iso = IsotonicRegression(out_of_bounds="clip")
    iso.fit(raw_scores, y_cal)

    # Persist sorted breakpoints for runtime interpolation
    thresholds = np.linspace(0.0, 1.0, 101)
    calibrated = iso.predict(thresholds)

    return {
        "method": "isotonic",
        "model_version": MODEL_VERSION,
        "feature_schema_version": FEATURE_SCHEMA_VERSION,
        "feature_names": list(FEATURE_NAMES),
        "thresholds": thresholds.tolist(),
        "calibrated": [float(v) for v in calibrated],
        "abstain_tau": DEFAULT_ABSTAIN_TAU,
    }


def export() -> None:
    write_dataset()
    if not PICKLE_PATH.exists():
        train_candidates()

    with PICKLE_PATH.open("rb") as f:
        pipeline = pickle.load(f)

    rows = load_dataset()
    train_rows, _ = conversation_safe_split(rows)
    calib = _fit_calibration(pipeline, train_rows)
    CALIBRATION_PATH.write_text(json.dumps(calib, indent=2), encoding="utf-8")

    initial_type = [("input", FloatTensorType([None, len(FEATURE_NAMES)]))]
    onnx_model = convert_sklearn(
        pipeline,
        initial_types=initial_type,
        target_opset=15,
    )
    ARTIFACT_DIR.mkdir(parents=True, exist_ok=True)
    with ONNX_PATH.open("wb") as f:
        f.write(onnx_model.SerializeToString())

    print(f"exported {ONNX_PATH}")
    print(f"wrote {CALIBRATION_PATH}")


if __name__ == "__main__":
    export()