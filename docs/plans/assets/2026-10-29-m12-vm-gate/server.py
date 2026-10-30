#!/usr/bin/env python3
"""Serves cast.html on 127.0.0.1:8765 and appends POSTed results to results.jsonl."""
import http.server, os
os.chdir(os.path.dirname(os.path.abspath(__file__)))
class Handler(http.server.SimpleHTTPRequestHandler):
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        with open('results.jsonl', 'ab') as f:
            f.write(body + b'\n')
        self.send_response(204); self.end_headers()
    def log_message(self, *args):
        pass
http.server.ThreadingHTTPServer(('127.0.0.1', 8765), Handler).serve_forever()
