from http.server import BaseHTTPRequestHandler, HTTPServer
import json


class H(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        raw = self.rfile.read(length)
        stream = False
        try:
            body = json.loads(raw.decode("utf-8") or "{}")
            stream = bool(body.get("stream"))
        except json.JSONDecodeError:
            pass

        if stream:
            out = b"data: {\"echo\":\"stream ok\"}\n\ndata: [DONE]\n\n"
            self.send_response(200)
            self.send_header("content-type", "text/event-stream")
            self.send_header("content-length", str(len(out)))
            self.end_headers()
            self.wfile.write(out)
            return

        resp = {
            "id": "echo",
            "object": "chat.completion",
            "choices": [{
                "index": 0,
                "message": {"role": "assistant", "content": "echo backend ok"},
                "finish_reason": "stop"
            }]
        }
        out = json.dumps(resp).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(out)))
        self.end_headers()
        self.wfile.write(out)


if __name__ == "__main__":
    HTTPServer(("0.0.0.0", 8081), H).serve_forever()