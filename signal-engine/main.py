import os
import time
from concurrent import futures

import grpc

from hsar.v1 import signal_service_pb2, signal_service_pb2_grpc
from hsar.v1 import signal_frame_pb2


def now_ms() -> int:
    return int(time.time() * 1000)


class SignalService(signal_service_pb2_grpc.SignalServiceServicer):
    def ProcessSignal(self, request, context):
        start = now_ms()

        # Stub logic: always return a deterministic dummy signal
        # This proves wiring + contracts, not intelligence.
        signals = [
            signal_frame_pb2.Signal(name="frustration_risk", value=0.12),
            signal_frame_pb2.Signal(name="failure_risk", value=0.05),
        ]

        end = now_ms()
        return signal_frame_pb2.SignalFrame(
            tenant_id=request.tenant_id or "default",
            request_id=request.request_id or "missing",
            tier=signal_frame_pb2.TIER_1,
            modality=signal_frame_pb2.MODALITY_TEXT,
            ts_start_ms=start,
            ts_end_ms=end,
            signals=signals,
            confidence=0.90,
            abstain=False,
            abstain_reason=signal_frame_pb2.ABSTAIN_REASON_UNSPECIFIED,
            processing_latency_ms=max(0, end - start),
            meta={"engine": "stub_v1"},
        )


def serve():
    port = os.getenv("SIGNAL_ENGINE_PORT", "50051")
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    signal_service_pb2_grpc.add_SignalServiceServicer_to_server(SignalService(), server)
    server.add_insecure_port(f"0.0.0.0:{port}")
    server.start()
    print(f"[signal-engine] listening on :{port}")
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
