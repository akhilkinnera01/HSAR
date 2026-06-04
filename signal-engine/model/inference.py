"""ONNX model inference with calibration and SignalFrame assembly."""

from __future__ import annotations

import json
import os
import time
from pathlib import Path

import numpy as np
import onnxruntime as ort

from hsar.v1 import signal_frame_pb2

from model.abstain import (
    abstain_frame,
    post_model_abstain,
    pre_model_abstain,
)
from model.features import extract_features

ROOT = Path(__file__).resolve().parents[1]
DEFAULT_MODEL_PATH = ROOT / "models" / "failure_risk" / "model.onnx"
DEFAULT_CALIBRATION_PATH = ROOT / "models" / "failure_risk" / "calibration.json"


class ModelAnalyzer:
    def __init__(
        self,
        model_path: str | Path | None = None,
        calibration_path: str | Path | None = None,
    ) -> None:
        self.model_path = Path(
            model_path or os.getenv("MODEL_PATH", str(DEFAULT_MODEL_PATH))
        )
        self.calibration_path = Path(
            calibration_path
            or os.getenv("CALIBRATION_PATH", str(DEFAULT_CALIBRATION_PATH))
        )
        self._session: ort.InferenceSession | None = None
        self._calibration: dict | None = None
        self._input_name: str | None = None
        self._load_error: str | None = None
        self._try_load()

    @property
    def available(self) -> bool:
        return self._session is not None and self._calibration is not None

    @property
    def load_error(self) -> str | None:
        return self._load_error

    def _try_load(self) -> None:
        try:
            if not self.model_path.exists():
                raise FileNotFoundError(f"missing model: {self.model_path}")
            if not self.calibration_path.exists():
                raise FileNotFoundError(
                    f"missing calibration: {self.calibration_path}"
                )
            self._session = ort.InferenceSession(
                str(self.model_path),
                providers=["CPUExecutionProvider"],
            )
            self._input_name = self._session.get_inputs()[0].name
            self._calibration = json.loads(
                self.calibration_path.read_text(encoding="utf-8")
            )
            self._load_error = None
        except Exception as exc:  # noqa: BLE001 — fail-open to heuristic
            self._session = None
            self._calibration = None
            self._load_error = str(exc)

    def _raw_score(self, text: str) -> float:
        if self._session is None or self._input_name is None:
            raise RuntimeError("model not loaded")
        features = extract_features(text).reshape(1, -1).astype(np.float32)
        outputs = self._session.run(None, {self._input_name: features})
        probs = outputs[1]
        if isinstance(probs, list):
            # skl2onnx ZipMap: [{0: p0, 1: p1}]
            return float(probs[0][1])
        probs_arr = np.asarray(probs)
        if probs_arr.ndim == 2:
            return float(probs_arr[0][1])
        return float(probs_arr[1])

    def _calibrate(self, raw_score: float) -> float:
        if self._calibration is None:
            return raw_score
        thresholds = np.array(self._calibration["thresholds"], dtype=np.float64)
        calibrated = np.array(self._calibration["calibrated"], dtype=np.float64)
        return float(np.interp(raw_score, thresholds, calibrated))

    def analyze(self, text: str) -> signal_frame_pb2.SignalFrame:
        start = time.time()

        pre_reason = pre_model_abstain(text)
        if pre_reason is not None:
            frame = abstain_frame(pre_reason)
            self._stamp_latency(frame, start)
            return frame

        if not self.available:
            raise RuntimeError(self._load_error or "model unavailable")

        raw = self._raw_score(text)
        calibrated = self._calibrate(raw)
        tau = float(self._calibration.get("abstain_tau", 0.55))

        if post_model_abstain(calibrated, tau):
            frame = abstain_frame(signal_frame_pb2.ABSTAIN_LOW_CONFIDENCE)
            self._stamp_latency(frame, start)
            return frame

        frame = signal_frame_pb2.SignalFrame(
            abstain=False,
            confidence=calibrated,
            tier=signal_frame_pb2.TIER_1,
            signals=[
                signal_frame_pb2.Signal(name="failure_risk", value=calibrated),
            ],
        )
        frame.meta["inference_source"] = "model"
        frame.meta["model_version"] = self._calibration.get(
            "model_version", "failure_risk-unknown"
        )
        frame.meta["feature_schema_version"] = self._calibration.get(
            "feature_schema_version", "fv-unknown"
        )
        self._stamp_latency(frame, start)
        return frame

    @staticmethod
    def _stamp_latency(frame: signal_frame_pb2.SignalFrame, start: float) -> None:
        end_ms = int(time.time() * 1000)
        frame.ts_start_ms = int(start * 1000)
        frame.ts_end_ms = end_ms
        frame.processing_latency_ms = end_ms - frame.ts_start_ms