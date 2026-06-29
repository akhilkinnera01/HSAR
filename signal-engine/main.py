import os
import time
from concurrent import futures

import grpc

from heuristics import analyze_text
from hsar.v1 import signal_service_pb2_grpc

try:
    from model.inference import ModelAnalyzer
except ImportError:  # pragma: no cover
    ModelAnalyzer = None  # type: ignore[misc, assignment]

PORT = int(os.getenv("PORT", "50051"))


def _heuristic_frame(text: str):
    frame = analyze_text(text)
    frame.meta["inference_source"] = "heuristic"
    frame.meta["model_version"] = "heuristic-v1"
    return frame


class SignalService(signal_service_pb2_grpc.SignalServiceServicer):
    def __init__(self) -> None:
        self._analyzer = None
        if ModelAnalyzer is not None:
            self._analyzer = ModelAnalyzer()
            if self._analyzer.available:
                print("[signal-engine] failure_risk model loaded", flush=True)
            else:
                print(
                    f"[signal-engine] model unavailable, using heuristic fallback: "
                    f"{self._analyzer.load_error}",
                    flush=True,
                )

    def ProcessSignal(self, request, context):
        print(
            f"[signal-engine] ProcessSignal called request_id={request.request_id} "
            f"text_len={len(request.text_payload or '')}",
            flush=True,
        )
        start = time.time()

        text = request.text_payload or ""
        try:
            if self._analyzer is not None and self._analyzer.available:
                frame = self._analyzer.analyze(text)
            else:
                frame = _heuristic_frame(text)
        except Exception as exc:  # noqa: BLE001 — fail-open per request
            print(f"[signal-engine] inference error, heuristic fallback: {exc}", flush=True)
            frame = _heuristic_frame(text)

        if not frame.ts_start_ms:
            latency_ms = int((time.time() - start) * 1000)
            end_ms = int(time.time() * 1000)
            frame.ts_start_ms = int(start * 1000)
            frame.ts_end_ms = end_ms
            frame.processing_latency_ms = latency_ms

        frame.tenant_id = request.tenant_id
        frame.request_id = request.request_id
        return frame


def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    signal_service_pb2_grpc.add_SignalServiceServicer_to_server(SignalService(), server)

    server.add_insecure_port(f"0.0.0.0:{PORT}")
    print(f"[signal-engine] listening on :{PORT}", flush=True)

    server.start()
    server.wait_for_termination()


if __name__ == "__main__":
    serve()