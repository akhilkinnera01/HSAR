import os
import time
from concurrent import futures

import grpc

from hsar.v1 import signal_service_pb2_grpc
from hsar.v1 import signal_frame_pb2


PORT = int(os.getenv("PORT", "50051"))

class SignalService(signal_service_pb2_grpc.SignalServiceServicer):
    def ProcessSignal(self, request, context):
        print(f"[signal-engine] ProcessSignal called request_id={request.request_id} text_len={len(request.text_payload or '')}", flush=True)
        start = time.time()

        # Minimal Tier 1 heuristic placeholder (fast, deterministic)
        # Note: Proto field is 'text_payload', not 'text'
        text = (request.text_payload or "").strip()
        caps_ratio = 0.0
        if text:
            letters = [c for c in text if c.isalpha()]
            if letters:
                caps_ratio = sum(1 for c in letters if c.isupper()) / len(letters)

        # dumb-but-valid outputs
        frustration = 0.2
        urgency = 0.2
        if "!" in text or caps_ratio > 0.6:
            urgency = 0.8
        if any(w in text.lower() for w in ["hate", "stupid", "angry", "terrible"]):
            frustration = 0.9

        latency_ms = int((time.time() - start) * 1000)
        end_ms = int(time.time() * 1000)

        # Construct signals list as per proto definition
        signals_list = [
            signal_frame_pb2.Signal(name="frustration", value=frustration),
            signal_frame_pb2.Signal(name="urgency", value=urgency),
        ]

        return signal_frame_pb2.SignalFrame(
            tenant_id=request.tenant_id,
            request_id=request.request_id,
            # ts_request_ms removed as not in SignalRequest proto
            ts_start_ms=int(start * 1000),
            ts_end_ms=end_ms,

            tier=signal_frame_pb2.TIER_1, # Enum usage corrected (removed SignalFrame. prefix if imported directly, but checking import... generated code usually requires correct scoping. Step 190 shows enum is top level in proto, so signal_frame_pb2.TIER_1 should work)
            abstain=False,
            confidence=0.70,

            signals=signals_list,

            processing_latency_ms=latency_ms,
        )

def serve():
    server = grpc.server(futures.ThreadPoolExecutor(max_workers=8))
    signal_service_pb2_grpc.add_SignalServiceServicer_to_server(SignalService(), server)

    server.add_insecure_port(f"0.0.0.0:{PORT}")
    print(f"[signal-engine] listening on :{PORT}", flush=True)

    server.start()
    server.wait_for_termination()


if __name__ == "__main__":
    serve()
