#!/usr/bin/env python3
"""Pass live credentials to a child process without echoing or storing secrets."""
import json
import os
import subprocess
import sys

env = dict(os.environ)
command = sys.argv[1:]
if not command:
    raise SystemExit("Usage: with-live-vendors.py COMMAND [ARGS...]")
vendors = [v.strip().upper() for v in env.get("E2E_LIVE_VENDORS", "FENNO").split(",") if v.strip()]
if not vendors or any(v not in ("FENNO", "QINIU") for v in vendors):
    raise SystemExit("E2E_LIVE_VENDORS must select FENNO and/or QINIU")
env["E2E_LIVE_VENDORS"] = ",".join(vendors)
# An explicitly selected subset must not accidentally call other env suppliers.
for name in list(env):
    if name.startswith("XHUB_REGRESSION_") and name.endswith("_KEY") and name[len("XHUB_REGRESSION_"):-4] not in vendors:
        del env[name]
if env.get("E2E_CREDENTIAL_SOURCE", "environment") == "database":
    # Read only the two explicitly selected existing suppliers, never the full DB.
    query = "SELECT body FROM public.kv WHERE kind='credentials' AND id IN ('fennoai','qiniu')"
    result = subprocess.run(["docker", "exec", "xhub-postgres", "psql", "-X", "-U", "xhub", "-d", "xhub", "-qAt", "-c", query], capture_output=True, text=True)
    if result.returncode:
        raise SystemExit("Cannot read configured Fenno/Qiniu credentials from PostgreSQL")
    for line in result.stdout.splitlines():
        row = json.loads(line)
        vendor = {"fennoai": "FENNO", "qiniu": "QINIU"}[row["credential_name"]]
        if vendor not in vendors: continue
        values = row["credential_values"]
        prefix = "XHUB_REGRESSION_" + vendor
        env.setdefault(prefix + "_KEY", values.get("api_key", ""))
        env.setdefault(prefix + "_BASE", values.get("api_base", "").rstrip("/"))
        env.setdefault(prefix + "_MODELS", {"FENNO": "gpt-5.5", "QINIU": "moonshotai/kimi-k2.5"}[vendor])
elif env.get("E2E_CREDENTIAL_SOURCE", "environment") != "environment":
    raise SystemExit("E2E_CREDENTIAL_SOURCE must be environment or database")
for vendor in vendors:
    prefix = "XHUB_REGRESSION_" + vendor.strip().upper()
    for suffix in ("_KEY", "_BASE", "_MODELS"):
        if not env.get(prefix + suffix):
            raise SystemExit("Missing " + prefix + suffix + "; configure environment or E2E_CREDENTIAL_SOURCE=database")
env["XHUB_REGRESSION_LIVE"] = "1"
env["E2E_LIVE"] = "1"
os.execvpe(command[0], command, env)
