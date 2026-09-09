#!/usr/bin/env python3
"""Server echo konten-terkontrol untuk uji filter + kuota Route-X staging.

Respons dipilih dari kata kunci di prompt terakhir:
- prompt mengandung RAHASIA-E2E -> jawab mengandung sk_live_RAHASIA (memicu
  filter blocked_pattern substring bila operator memasangnya).
- prompt mengandung LAMBAT-E2E -> sleep 3 detik lalu jawab (uji timeout).
- selain itu -> jawab "siap" normal seperti echo-upstream.py.

Tanpa dependensi. Jalankan: python3 scripts/load/echo-konten.py 19093
"""
import json
import sys
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 19093

NORMAL = "siap"
RAHASIA = "berikut kuncinya «redacted:sk_live_…» jangan disebar"


def teks_untuk(prompt: str):
    if "LAMBAT-E2E" in prompt:
        time.sleep(3)
        return NORMAL
    if "RAHASIA-E2E" in prompt:
        return RAHASIA
    return NORMAL


def prompt_terakhir(req: dict) -> str:
    try:
        msgs = req.get("messages", [])
        if msgs and isinstance(msgs[-1], dict):
            return str(msgs[-1].get("content", ""))
    except (AttributeError, IndexError):
        pass
    return ""


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
                "created": 1757328000, "owned_by": "echo-konten",
            }]}
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
        teks = teks_untuk(prompt_terakhir(req))
        if req.get("stream"):
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Cache-Control", "no-cache")
            self.send_header("Connection", "keep-alive")
            self.end_headers()
            frame = {"id": "chatcmpl-konten", "object": "chat.completion.chunk",
                     "created": 1757328000, "model": "gpt-5",
                     "choices": [{"index": 0, "delta": {"content": teks},
                                  "finish_reason": None}]}
            try:
                self.wfile.write(("data: " + json.dumps(frame) + "\n\n").encode())
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                return
            final = {"id": "chatcmpl-konten", "object": "chat.completion.chunk",
                     "created": 1757328000, "model": "gpt-5",
                     "choices": [{"index": 0, "delta": {}, "finish_reason": "stop"}]}
            try:
                self.wfile.write(("data: " + json.dumps(final) + "\n\n").encode())
                self.wfile.write(b"data: [DONE]\n\n")
                self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass
        else:
            body = {"id": "chatcmpl-konten", "object": "chat.completion",
                    "created": 1757328000, "model": "gpt-5",
                    "choices": [{"index": 0,
                                 "message": {"role": "assistant", "content": teks},
                                 "finish_reason": "stop"}],
                    "usage": {"prompt_tokens": 8, "completion_tokens": 4,
                              "total_tokens": 12}}
            raw = json.dumps(body).encode()
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
    print("echo-konten di 127.0.0.1:%d" % PORT, flush=True)
    srv.serve_forever()
