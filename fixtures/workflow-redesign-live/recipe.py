#!/usr/bin/env python3
"""Finite deploy/ready/check/cleanup commands for one private local Go service.

Usage: python3 fixtures/workflow-redesign-live/recipe.py <command> <run-id>
The service uses HTTP over a private Unix socket, never a remote host or port.
A 3-item quote must cost 375 cents. Readiness alone does not establish this.
"""

import hashlib
import http.client
import json
import os
from pathlib import Path
import re
import secrets
import socket
import subprocess
import sys
import time

SOURCE = "fixtures/workflow-redesign-live/service.go"
GENERATED = {"state.json", "service", "service.sock", "server.log", "stopped"}


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def target():
    revision = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    committed = subprocess.check_output(["git", "show", f"{revision}:{SOURCE}"])
    require(Path(SOURCE).read_bytes() == committed, "service source differs from selected commit")
    return revision, hashlib.sha256(committed).hexdigest()


def load(root, run_id):
    require(not root.is_symlink() and root.is_dir(), "owned directory missing or symlinked")
    require(root.stat().st_uid == os.getuid(), "owned directory belongs to another user")
    state = json.loads((root / "state.json").read_text())
    require(state["runID"] == run_id, "run ownership mismatch")
    return state


class LocalHTTP(http.client.HTTPConnection):
    def __init__(self, root):
        super().__init__("localhost", timeout=2)
        self.root = root

    def connect(self):
        self.sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
        self.sock.settimeout(self.timeout)
        self.sock.connect(str(self.root / "service.sock"))


def request(root, state, method, path, payload=None):
    connection = LocalHTTP(root)
    try:
        body = None if payload is None else json.dumps(payload)
        connection.request(method, path, body, {
            "Authorization": "Bearer " + state["token"],
            "Content-Type": "application/json",
        })
        response = connection.getresponse()
        data = response.read()
        require(response.status in (200, 204), f"{path}: HTTP {response.status}")
        return json.loads(data) if data else None
    finally:
        connection.close()


def wait_ready(root, state):
    deadline = time.monotonic() + 10
    while True:
        try:
            require(request(root, state, "GET", "/health") == {"ready": True}, "not ready")
            return
        except (OSError, http.client.HTTPException, RuntimeError):
            if time.monotonic() >= deadline:
                raise RuntimeError("readiness deadline exceeded; run cleanup")
            time.sleep(0.05)


def deploy(root, run_id):
    revision, source_hash = target()
    root.mkdir(mode=0o700)  # Existing run resources require explicit cleanup, never overwrite.
    state = {"runID": run_id, "repoCommit": revision, "sourceSha256": source_hash,
             "token": secrets.token_urlsafe(24), "started": False}
    state_path = root / "state.json"
    state_path.write_text(json.dumps(state))
    state_path.chmod(0o600)
    subprocess.run(["go", "build", "-trimpath", "-ldflags", "-X main.revision=" + revision,
                    "-o", str(root / "service"), SOURCE], check=True)
    state["artifactSha256"] = digest(root / "service")
    state["started"] = True
    state_path.write_text(json.dumps(state))
    with (root / "server.log").open("wb") as log:
        child = subprocess.Popen([str(root / "service"), str(root)], stdin=subprocess.DEVNULL,
                                 stdout=log, stderr=log, start_new_session=True, env={})
    try:
        wait_ready(root, state)
    except BaseException:
        # This Popen is the process we just created, not a PID read from an old file.
        child.terminate()
        child.wait(timeout=5)
        (root / "stopped").write_text("stopped\n")
        raise
    print(json.dumps({"result": "deployed", "repoCommit": revision,
                      "artifactSha256": state["artifactSha256"], "target": str(root / "service.sock")}))


def check(root, run_id):
    state = load(root, run_id)
    revision, source_hash = target()
    require(state["repoCommit"] == revision and state["sourceSha256"] == source_hash,
            "stale deployment: selected revision/source differs")
    require(digest(root / "service") == state["artifactSha256"], "deployed artifact was replaced")
    observed = request(root, state, "POST", "/quote", {"quantity": 3})
    expected = {"quantity": 3, "totalCents": 375, "revision": revision}
    require(observed == expected, "business assertion failed: " + json.dumps(observed))
    print(json.dumps({"result": "business-pass", "repoCommit": revision,
                      "sourceSha256": source_hash, "artifactSha256": state["artifactSha256"],
                      "target": str(root / "service.sock"), "observed": observed}))


def cleanup(root, run_id):
    if not root.exists() and not root.is_symlink():
        print(json.dumps({"result": "cleanup-pass", "runID": run_id, "absent": True}))
        return
    state = load(root, run_id)
    if state["started"] and not (root / "stopped").exists():
        request(root, state, "POST", "/shutdown")
        deadline = time.monotonic() + 5
        while not (root / "stopped").exists() or (root / "service.sock").exists():
            require(time.monotonic() < deadline, "shutdown unproven; preserving run resources")
            time.sleep(0.05)
    require(not (root / "service.sock").exists(), "live socket remains; refusing removal")
    require({p.name for p in root.iterdir()} <= GENERATED, "unexpected files; refusing removal")
    for name in GENERATED:
        (root / name).unlink(missing_ok=True)
    root.rmdir()
    require(not root.exists(), "owned resources remain")
    print(json.dumps({"result": "cleanup-pass", "runID": run_id, "absent": True}))


def main():
    require(len(sys.argv) == 3, "usage: recipe.py deploy|ready|check|cleanup <run-id>")
    action, run_id = sys.argv[1:]
    require(re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9-]{0,39}", run_id), "invalid run id")
    root = Path("/tmp") / ("test-design-workflow-" + run_id)
    if action == "deploy":
        deploy(root, run_id)
    elif action == "ready":
        wait_ready(root, load(root, run_id))
        print(json.dumps({"result": "ready"}))
    elif action == "check":
        check(root, run_id)
    elif action == "cleanup":
        cleanup(root, run_id)
    else:
        raise RuntimeError("unknown command: " + action)


if __name__ == "__main__":
    try:
        main()
    except (OSError, RuntimeError, ValueError, KeyError, subprocess.SubprocessError,
            http.client.HTTPException) as error:
        print(str(error), file=sys.stderr)
        sys.exit(1)
