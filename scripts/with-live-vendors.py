#!/usr/bin/env python3
"""Validate non-secret supplier YAML and pass credentials only in child environment."""
import json
import math
import os
from pathlib import Path
import re
import subprocess
import sys
from urllib.parse import urlsplit

root = Path(__file__).resolve().parent.parent
env = dict(os.environ)
command = sys.argv[1:]
if not command:
    raise SystemExit("Usage: with-live-vendors.py COMMAND [ARGS...]")
path = Path(env.get("E2E_PROVIDER_CONFIG", str(root / "config_provider.yaml"))).resolve()
print("Loading supplier configuration: " + str(path), flush=True)
parsed = subprocess.run(["go", "run", "./e2e/providerconfig", str(path)], cwd=root, capture_output=True, text=True)
if parsed.returncode:
    raise SystemExit("Invalid provider YAML; check config_provider.yaml against docs/development/e2e-regression.md")
config = json.loads(parsed.stdout)
if config["version"] != 1:
    raise SystemExit("Provider config version must be 1")
providers = {}
for provider in config["providers"]:
    name = provider["id"]
    if not re.fullmatch(r"[A-Z][A-Z0-9_]*", name) or name in providers:
        raise SystemExit("Provider IDs must be unique uppercase environment-variable stems")
    if not re.fullmatch(r"[A-Z][A-Z0-9_]*", provider["key_env"]):
        raise SystemExit("Each provider must specify a key_env variable name")
    if provider["protocol"] not in ("openai", "anthropic") or not provider["models"]:
        raise SystemExit("Each provider requires protocol openai/anthropic and a nonempty models list")
    providers[name] = provider
vendors = [v.strip().upper() for v in env.get("E2E_LIVE_VENDORS", ",".join(p["id"] for p in providers.values() if p["enabled"])).split(",") if v.strip()]
if not vendors or len(set(vendors)) != len(vendors) or any(v not in providers for v in vendors):
    raise SystemExit("E2E_LIVE_VENDORS must select unique configured supplier IDs")
for name in list(env):
    if name.startswith("XHUB_REGRESSION_") and name.endswith("_KEY") and name[len("XHUB_REGRESSION_"):-4] not in vendors:
        del env[name]
source = env.get("E2E_CREDENTIAL_SOURCE", "environment")
credentials = {}
if source == "database":
    ids = [providers[v]["credential_name"] for v in vendors]
    if any(not re.fullmatch(r"[a-zA-Z0-9_-]+", name) for name in ids):
        raise SystemExit("Database credential_name must be a simple identifier")
    query = "SELECT body FROM public.kv WHERE kind='credentials' AND id IN (" + ",".join("'" + name + "'" for name in ids) + ")"
    result = subprocess.run(["docker", "exec", "xhub-postgres", "psql", "-X", "-U", "xhub", "-d", "xhub", "-qAt", "-c", query], capture_output=True, text=True)
    if result.returncode:
        raise SystemExit("Cannot read selected supplier credentials from PostgreSQL")
    for line in result.stdout.splitlines():
        row = json.loads(line)
        credentials[row["credential_name"]] = row["credential_values"].get("api_key", "")
elif source != "environment":
    raise SystemExit("E2E_CREDENTIAL_SOURCE must be environment or database")
selected = []
for vendor in vendors:
    p = dict(providers[vendor])
    prefix = "XHUB_REGRESSION_" + vendor
    key = env.get(p["key_env"]) or credentials.get(p["credential_name"])
    if not key:
        raise SystemExit("Missing " + p["key_env"] + "; configure environment or E2E_CREDENTIAL_SOURCE=database")
    base = env.get(prefix + "_BASE", p["base"]).strip()
    url = urlsplit(base)
    if url.scheme not in ("https", "http") or not url.hostname or any(c in base for c in "[]() \n\r\t") or url.username or url.password or url.query or url.fragment:
        raise SystemExit(prefix + "_BASE must be a plain URL (no Markdown brackets)")
    models = [m.strip() for m in env.get(prefix + "_MODELS", ",".join(p["models"])).split(",") if m.strip()]
    if not models:
        raise SystemExit(prefix + "_MODELS must contain at least one model")
    protocol = env.get(prefix + "_PROTOCOL", p["protocol"])
    if protocol not in ("openai", "anthropic"):
        raise SystemExit(prefix + "_PROTOCOL must be openai or anthropic")
    p.update(base=base.rstrip("/"), models=models, protocol=protocol)
    env[prefix + "_KEY"] = key
    env[prefix + "_BASE"] = p["base"]
    env[prefix + "_MODELS"] = ",".join(models)
    env[prefix + "_PROTOCOL"] = protocol
    selected.append(p)
    print("Live regression: " + vendor + " models=" + ",".join(models), flush=True)
scenarios = []
selected_by_id = {p["id"]: p for p in selected}
scenario_names = set()
for s in config["weighted_scenarios"] or []:
    if s["name"] in scenario_names or not re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]*", s["name"]):
        raise SystemExit("Weighted scenario names must be unique simple identifiers")
    scenario_names.add(s["name"])
    if any(d["provider"] not in providers for d in s["deployments"]):
        raise SystemExit("Weighted scenario references an unknown provider")
    if any(d["provider"] not in vendors for d in s["deployments"]):
        print("Excluded weighted scenario " + s["name"] + ": supplier not selected", flush=True)
        continue
    deployments = s["deployments"]
    if not s["name"] or not s["model_name"] or len(deployments) < 2 or s["requests"] <= 0 or s["requests"] > 100:
        raise SystemExit("Weighted scenarios require a name, model_name, 2+ deployments and 1..100 requests")
    ids = [d["id"] for d in deployments]
    if len(set(ids)) != len(ids) or any(not re.fullmatch(r"[A-Za-z0-9_-]+", i) for i in ids):
        raise SystemExit("Weighted deployment IDs must be distinct simple identifiers")
    weights = [d["weight"] for d in deployments]
    if any(w < 0 for w in weights) or sum(weights) <= 0:
        raise SystemExit("Weights must be nonnegative integers with a positive sum")
    cycle = sum(weights) // math.gcd(*weights)
    if s["requests"] % cycle:
        raise SystemExit("Weighted requests must span whole reduced-weight cycles: " + s["name"])
    for d in deployments:
        if not d["model"] or selected_by_id[d["provider"]]["protocol"] != "openai":
            raise SystemExit("Weighted scenarios currently require OpenAI chat deployments with a model")
    scenarios.append(s)
config.update(providers=selected, weighted_scenarios=scenarios)
env["E2E_PROVIDER_CONFIG"] = str(path)
env["E2E_PROVIDER_METADATA"] = json.dumps(config)
env["E2E_LIVE_VENDORS"] = ",".join(vendors)
env["XHUB_REGRESSION_LIVE"] = "1"
env["E2E_LIVE"] = "1"
os.execvpe(command[0], command, env)
