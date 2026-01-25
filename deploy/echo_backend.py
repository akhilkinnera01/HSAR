from http.server import BaseHTTPRequestHandler, HTTPServer
import json

class H(BaseHTTPRequestHandler):
    def do_POST(self):
        length = int(self.headers.get("content-length", "0"))
        _ = self.rfile.read(length)

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
