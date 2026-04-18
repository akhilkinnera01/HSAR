import os
import time
from concurrent import futures

import grpc

from hsar.v1 import signal_frame_pb2, signal_service_pb2_grpc
from tier1 import extract_tier1


def now_ms() -> int:
    return int(time.time() * 1000)


class SignalService(signal_service_pb2_grpc.SignalServiceServicer):
    def ProcessSignal(self, request, context):
        start = now_ms()

        result = extract_tier1(request.text_payload)

        end = now_ms()

        signals = [
            signal_frame_pb2.Signal(name=s["name"], value=s["value"])
            for s in result["signals"]
        ]

        abstain_reason = signal_frame_pb2.AbstainReason.Value(
            result["abstain_reason"]
        )

        return signal_frame_pb2.SignalFrame(
            tenant_id=request.tenant_id or "default",
            request_id=request.request_id or "missing",
            tier=signal_frame_pb2.TIER_1,
            modality=signal_frame_pb2.MODALITY_TEXT,
            ts_start_ms=start,
            ts_end_ms=end,
            signals=signals,
            confidence=result["confidence"],
            abstain=result["abstain"],
            abstain_reason=abstain_reason,
            processing_latency_ms=max(0, end - start),
            meta=result["meta"],
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
