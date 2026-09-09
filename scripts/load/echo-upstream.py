#!/usr/bin/env python3
"""Server echo OpenAI-compatible untuk uji beban Route-X staging.

Menjawab /chat/completions (non-stream + SSE) dan /models dengan format OpenAI.
Tanpa dependensi, tanpa internet. Latency buatan 5-15ms agar realistis.
Jalankan: python3 scripts/load/echo-upstream.py 19091
"""
import json
import random
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 19091

CHAT_BODY = {
    "id": "chatcmpl-echo",
    "object": "chat.completion",
    "created": 1757328000,
    "model": "gpt-5",
    "choices": [{
        "index": 0,
        "message": {"role": "assistant", "content": "siap"},
        "finish_reason": "stop",
    }],
    "usage": {"prompt_tokens": 8, "completion_tokens": 1, "total_tokens": 9},
}

CHUNKS = ["si", "ap"]


class Handler(BaseHTTPRequestHandler):
    def log_message(self, *a):
        pass

    def _read_json(self):
        try:
            n = int(self.headers.get("Content-Length", 0))
        except ValueError:
            n = 0
        raw = self.rfile.read(n) if n > 0 else b"{}"
        try:
            return json.loads(raw or b"{}")
        except ValueError:
            return {}

    def do_GET(self):
        if self.path in ("/models", "/v1/models"):
            body = {"object": "list", "data": [{
                "id": "gpt-5", "object": "model",
                "created": 1757328000, "owned_by": "echo",
            }]}.copy()
            raw = json.dumps(body).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            self.wfile.write(raw)
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        if self.path not in ("/chat/completions", "/v1/chat/completions"):
            self.send_response(404)
            self.end_headers()
            return
        req = self._read_json()
        time.sleep(random.uniform(0.005, 0.015))
        if req.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.end_headers()
            for i, delta in enumerate(CHUNKS):
                frame = {"id": "chatcmpl-echo", "object": "chat.completion.chunk",
                         "created": 1757328000, "model": "gpt-5",
                         "choices": [{"index": 0, "delta": {"content": delta},
                                      "finish_reason": None}]}
                try:
                    self.wfile.write(("data: " + json.dumps(frame) + "\n\n").encode())
                    self.wfile.flush()
                except (BrokenPipeError, ConnectionResetError):
                    return
                time.sleep(0.01)
            final = {"id": "chatcmpl-echo", "object": "chat.completion.chunk",
                     "created": 1757328000, "model": "gpt-5",
                     "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]}
            try:
                self.wfile.write(("data: " + json.dumps(final) + "\n\n").encode())
                self.wfile.write(b"data: [DONE]\n\n")
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
        else:
            raw = json.dumps(CHAT_BODY).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(raw)))
            self.end_headers()
            try:
                self.wfile.write(raw)
            except (BrokenPipeError, ConnectionResetError):
                pass


if __name__ == "__main__":
    srv = ThreadingHTTPServer(("127.0.0.1", PORT), Handler)
    print("echo-upstream di 127.0.0.1:%d" % PORT, flush=True)
    srv.serve_forever()
