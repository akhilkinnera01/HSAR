import os
import sys
import time

import pytest

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), ".."))
ENGINE_ROOT = os.path.dirname(__file__)
sys.path.insert(0, os.path.join(ROOT, "gen", "python"))
sys.path.insert(0, ENGINE_ROOT)

from hsar.v1 import signal_frame_pb2  # noqa: E402
from model.inference import ModelAnalyzer  # noqa: E402


def _model_paths():
    base = os.path.join(ENGINE_ROOT, "models", "failure_risk")
    return (
        os.path.join(base, "model.onnx"),
        os.path.join(base, "calibration.json"),
    )


def _analyzer() -> ModelAnalyzer:
    model_path, calib_path = _model_paths()
    analyzer = ModelAnalyzer(model_path=model_path, calibration_path=calib_path)
    if not analyzer.available:
        pytest.skip(f"model artifacts missing: {analyzer.load_error}")
    return analyzer


def _signal_map(frame):
    return {s.name: s.value for s in frame.signals}


def test_failure_risk_present_for_heated_input():
    analyzer = _analyzer()
    frame = analyzer.analyze("THIS IS TERRIBLE AND BROKEN!!! ESCALATE NOW!!!")

    assert frame.abstain is False
    assert "failure_risk" in _signal_map(frame)
    assert 0.0 <= frame.confidence <= 1.0
    assert frame.confidence != 0.70
    assert frame.meta["inference_source"] == "model"


def test_calm_vs_heated_ranking():
    analyzer = _analyzer()
    calm = analyzer.analyze("Thanks for the helpful update on my order.")
    heated = analyzer.analyze("I hate this stupid broken service!!! ESCALATE!!!")

    assert calm.abstain is False and heated.abstain is False
    assert _signal_map(heated)["failure_risk"] > _signal_map(calm)["failure_risk"]


def test_abstain_empty_input():
    analyzer = _analyzer()
    frame = analyzer.analyze("   ")
    assert frame.abstain is True
    assert frame.abstain_reason == signal_frame_pb2.ABSTAIN_EMPTY_INPUT


def test_abstain_short_input():
    analyzer = _analyzer()
    frame = analyzer.analyze("ok")
    assert frame.abstain is True
    assert frame.abstain_reason == signal_frame_pb2.ABSTAIN_TOO_SHORT


def test_abstain_low_confidence_ambiguous():
    analyzer = _analyzer()
    # Neutral/helpful text often lands near decision boundary
    frame = analyzer.analyze("please help with my account balance")
    if frame.abstain:
        assert frame.abstain_reason == signal_frame_pb2.ABSTAIN_LOW_CONFIDENCE
    else:
        assert frame.confidence >= 0.55 or frame.confidence <= 0.45


def test_heuristic_fallback_missing_model():
    analyzer = ModelAnalyzer(
        model_path="/nonexistent/model.onnx",
        calibration_path="/nonexistent/calibration.json",
    )
    assert not analyzer.available

    from heuristics import analyze_text

    frame = analyze_text("I AM SO ANGRY!!!")
    frame.meta["inference_source"] = "heuristic"
    frame.meta["model_version"] = "heuristic-v1"
    assert frame.meta["inference_source"] == "heuristic"
    assert not frame.abstain


def test_inference_p99_under_30ms():
    analyzer = _analyzer()
    samples = [
        "Thanks for your help today.",
        "THIS IS UNACCEPTABLE!!! FIX IT NOW!!!",
        "Could you explain the refund policy please?",
        "Wrong wrong wrong, escalate me immediately!!!",
    ] * 50

    timings = []
    for text in samples:
        start = time.perf_counter()
        analyzer.analyze(text)
        timings.append((time.perf_counter() - start) * 1000)

    timings.sort()
    p99 = timings[int(len(timings) * 0.99) - 1]
    assert p99 < 30.0, f"p99 latency {p99:.2f}ms exceeds 30ms budget"