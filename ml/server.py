"""
HTTP server exposing POST /run to trigger train + predict pipeline on demand.
Runs alongside the cron schedule as the main process.
"""

import os
import subprocess
import threading
from flask import Flask, jsonify, request

app = Flask(__name__)
_lock = threading.Lock()
_running = False

ML_SECRET = os.environ.get("ML_SECRET", "")


def _authenticate(req) -> bool:
    if not ML_SECRET:
        return True
    return req.headers.get("X-ML-Secret") == ML_SECRET


@app.get("/health")
def health():
    return jsonify({"status": "ok"})


@app.post("/run")
def run():
    global _running

    if not _authenticate(request):
        return jsonify({"error": "unauthorized"}), 401

    if not _lock.acquire(blocking=False):
        return jsonify({"error": "pipeline already running"}), 409

    _running = True

    def execute():
        global _running
        try:
            subprocess.run(["python", "train.py"], check=True)
            subprocess.run(["python", "predict.py"], check=True)
        finally:
            _running = False
            _lock.release()

    threading.Thread(target=execute, daemon=True).start()
    return jsonify({"status": "started"}), 202


@app.get("/status")
def status():
    return jsonify({"running": _running})


if __name__ == "__main__":
    port = int(os.environ.get("PORT", os.environ.get("ML_PORT", 8081)))
    app.run(host="0.0.0.0", port=port)
