#!/usr/bin/env python3
"""
Mock Upstream Server untuk pengujian End-to-End dan Load Testing Route-X.
Mendukung OpenAI API /v1/models dan /v1/chat/completions (streaming SSE & non-streaming).
"""

from http.server import HTTPServer, BaseHTTPRequestHandler
import json
import os
import sys
import time

PORT = int(os.environ.get("MOCK_UPSTREAM_PORT", "9099"))

class MockHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        # Heningkan log default akses HTTP untuk menjaga keluaran pengujian tetap bersih
        pass

    def do_GET(self):
        if "/models" in self.path:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"object":"list","data":[{"id":"gpt-5","object":"model"}]}')
            return
        if self.path in ("/healthz", "/readyz"):
            self.send_response(200)
            self.send_header("Content-Type", "text/plain")
            self.end_headers()
            self.wfile.write(b"ok\n")
            return
        self.send_response(404)
        self.end_headers()

    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode('utf-8')
        try:
            req = json.loads(body)
        except Exception:
            req = {}

        if req.get("stream", False):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.end_headers()
            chunk = {
                "id": "chatcmpl-mock",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": "gpt-5",
                "choices": [{"index": 0, "delta": {"content": "Halo dari mock upstream Route-X!"}, "finish_reason": None}]
            }
            self.wfile.write(f"data: {json.dumps(chunk)}\n\n".encode('utf-8'))
            self.wfile.flush()
            end_chunk = {
                "id": "chatcmpl-mock",
                "object": "chat.completion.chunk",
                "created": int(time.time()),
                "model": "gpt-5",
                "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}],
                "usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
            }
            self.wfile.write(f"data: {json.dumps(end_chunk)}\n\ndata: [DONE]\n\n".encode('utf-8'))
            self.wfile.flush()
        else:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            resp = {
                "id": "chatcmpl-mock",
                "object": "chat.completion",
                "created": int(time.time()),
                "model": "gpt-5",
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": "Halo dari mock upstream Route-X!"},
                    "finish_reason": "stop"
                }],
                "usage": {"prompt_tokens": 10, "completion_tokens": 8, "total_tokens": 18}
            }
            self.wfile.write(json.dumps(resp).encode('utf-8'))

if __name__ == "__main__":
    server_address = ('127.0.0.1', PORT)
    httpd = HTTPServer(server_address, MockHandler)
    print(f"Mock upstream server berjalan di http://127.0.0.1:{PORT}", file=sys.stderr)
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        pass
