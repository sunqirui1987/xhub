#!/usr/bin/env python3
"""Stop leftover e2e listeners on 3000/4000/4010."""
from __future__ import annotations

import os
import signal
import subprocess
import time

PORTS = (3000, 4000, 4010)


def pids(port: int) -> list[int]:
    try:
        out = subprocess.check_output(
            ["lsof", "-nP", f"-iTCP:{port}", "-sTCP:LISTEN", "-t"],
            text=True,
        )
    except (subprocess.CalledProcessError, FileNotFoundError):
        return []
    found = []
    for line in out.split():
        try:
            found.append(int(line))
        except ValueError:
            continue
    return found


def stop(sig: int) -> None:
    for port in PORTS:
        for pid in pids(port):
            try:
                os.kill(pid, sig)
            except ProcessLookupError:
                pass


def main() -> None:
    stop(signal.SIGTERM)
    deadline = time.time() + 2
    while time.time() < deadline:
        stop(signal.SIGKILL)
        if not any(pids(port) for port in PORTS):
            return
        time.sleep(0.2)
    stop(signal.SIGKILL)


if __name__ == "__main__":
    main()
