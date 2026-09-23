#!/usr/bin/env python3
"""Stop leftover e2e listeners on 3000/4000/4010."""
from __future__ import annotations

import os
import signal
import subprocess

PORTS = (3000, 4000, 4010)


def main() -> None:
    for port in PORTS:
        try:
            out = subprocess.check_output(["lsof", "-ti", f"tcp:{port}"], text=True)
        except (subprocess.CalledProcessError, FileNotFoundError):
            continue
        for pid in out.split():
            try:
                os.kill(int(pid), signal.SIGTERM)
            except (ProcessLookupError, ValueError):
                pass


if __name__ == "__main__":
    main()
