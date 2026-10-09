# Live provider test configuration

[简体中文](readme_cn.md) · [Regression plan](../../docs/development/regression.md) · [Multi-provider scenarios](../../docs/development/multi-supplier-regression.md)

## Responsibility and data boundary

This package parses the non-secret metadata in `config_provider.yaml` for live-model regression and deterministic multi-provider scenarios. It does not read environment variables, load credentials, call providers, or start the gateway. `key_env` names a credential variable; the runner supplies its value separately. Parse errors never echo YAML input, which could contain a mistakenly pasted secret.

`Config` contains a version, providers, and weighted scenarios. Each `Provider` declares a stable ID, credential name, environment-variable name, API base, protocol, and models. A `Scenario` names the public model, request count, and deployments. Each `Deployment` has a stable ID, provider, upstream model, and nonnegative weight. Tests use deployment IDs to verify the split; an endpoint URL or upstream model is not the weight identity.

## Implementation and API

- [providerconfig.go](providerconfig.go) exports `Load(io.Reader) (Config, error)`. It decodes exactly one strict YAML document, rejects unknown fields, and validates references. Nil input, malformed YAML, extra documents, and invalid fields fail.
- `validate` requires version 1, well-formed unique provider IDs, nonempty credential names, valid environment-variable names, HTTP(S) bases without embedded credentials, query, or fragment, supported OpenAI/Anthropic protocols, and nonempty model lists. Weighted deployments must reference configured providers and their listed models.
- Each weighted scenario needs at least two deployments, globally unique deployment IDs, nonnegative weights with a positive total, and 1–100 requests spanning complete reduced-weight cycles. For a 3:7 split, a multiple of ten calls yields an exact count. `gcd` reduces the cycle, and the sum guards against integer overflow.
- `validHTTPURL` accepts API base paths while rejecting URL credentials, query strings, and fragments.

This package registers no HTTP endpoint. The runner selects providers, resolves `key_env`, creates deployments, sends inference calls, and verifies billing. A valid document does not prove a live provider is reachable.

## Verification

[providerconfig_test.go](providerconfig_test.go) covers shared models across providers, multiple connections to one provider, validation failures, weight cycles and overflow, unknown YAML fields, additional documents, and error sanitization. The [regression package](../regression/readme.md) tests the full multi-provider and live-weight paths.

```bash
go test ./internal/providerconfig -count=1
bash scripts/regression.sh
```
