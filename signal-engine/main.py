import os
import time
from concurrent import futures

import grpc

from heuristics import analyze_text
from hsar.v1 import signal_frame_pb2
from hsar.v1 import signal_service_pb2_grpc

PORT = int(os.getenv("PORT", "50051"))


class SignalService(signal_service_pb2_grpc.SignalServiceServicer):
    def ProcessSignal(self, request, context):
        print(
            f"[signal-engine] ProcessSignal called request_id={request.request_id} "
            f"text_len={len(request.text_payload or '')}",
            flush=True,
        )
        start = time.time()

        frame = analyze_text(request.text_payload or "")
        latency_ms = int((time.time() - start) * 1000)
        end_ms = int(time.time() * 1000)

        frame.tenant_id = request.tenant_id
        frame.request_id = request.request_id
        frame.ts_start_ms = int(start * 1000)
        frame.ts_end_ms = end_ms
        frame.processing_latency_ms = latency_ms

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