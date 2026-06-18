#!/usr/bin/env python3
"""HTTP server for /root/workspace/opc/docs/ on port 19788.

Fixes the 乱码 issue: the default Python http.server sends `text/markdown`
without a charset, so browsers default to Latin-1 and Chinese shows as
mojibake. This wrapper forces `charset=utf-8` on every text/* response
and serves everything as bytes-safe."""

import http.server
import os
import sys
import socketserver

PORT = int(os.environ.get("PORT", "19788"))
ROOT = os.environ.get("ROOT", "/root/workspace/opc/docs")

# Map file extension → MIME type with explicit charset
MIME = {
    ".html": "text/html; charset=utf-8",
    ".htm":  "text/html; charset=utf-8",
    ".md":   "text/markdown; charset=utf-8",
    ".json": "application/json; charset=utf-8",
    ".txt":  "text/plain; charset=utf-8",
    ".log":  "text/plain; charset=utf-8",
    ".mp4":  "video/mp4",
    ".webm": "video/webm",
    ".jpg":  "image/jpeg",
    ".jpeg": "image/jpeg",
    ".png":  "image/png",
    ".gif":  "image/gif",
    ".webp": "image/webp",
    ".svg":  "image/svg+xml; charset=utf-8",
    ".css":  "text/css; charset=utf-8",
    ".js":   "application/javascript; charset=utf-8",
}

def guess_type(path):
    ext = os.path.splitext(path)[1].lower()
    return MIME.get(ext, "application/octet-stream")


class UTF8Handler(http.server.SimpleHTTPRequestHandler):
    def end_headers(self):
        # Make sure every text response declares utf-8
        ct = self.headers.get("Content-Type", "")
        if ct.startswith("text/") and "charset" not in ct:
            self.send_header("Content-Type", ct + "; charset=utf-8")
        super().end_headers()

    def guess_type(self, path):
        return guess_type(path)


if __name__ == "__main__":
    os.chdir(ROOT)
    print(f"serving {ROOT} on 0.0.0.0:{PORT}")
    socketserver.TCPServer.allow_reuse_address = True
    with socketserver.TCPServer(("0.0.0.0", PORT), UTF8Handler) as httpd:
        httpd.serve_forever()
