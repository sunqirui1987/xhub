# Qiniu video-task transports

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

seedance.go registers the qiniu_contents_generation protocol with api.qnaigc.com as its protocol default. It supports POST /v3/contents/generations/tasks and GET the task path with /{id}, without a list action. XHub does not register a fixed Qiniu credential provider. Administrators select Custom or Custom OpenAI, enter their own base and key, and explicitly select this transport on the model.
The model field is model and task ID field is id; the qiniu prefix is removed upstream. Four Seedance entries cover standard, Fast, Mini (260615), and 2.5 (260628), with explicit rates from the Qiniu Modelink market source. Credential names, model names, and hostnames do not infer the transport.
Dataplane owns sending, tenant task affinity, polling, and terminal settlement. Registration tests validate descriptions; real task tests require explicit bypass configuration and consume provider quota. Creation success alone does not prove completion or billing accuracy.

## Source responsibilities and entry points

### seedance.go

Internal implementation and protocol boundaries:  [seedance.go](seedance.go)。

### fal.go

[fal.go](fal.go) registers 100 concrete Fal creation endpoints across nine shared queue transports for Doubao/Dreamina Seedance, Kling, Vidu, Veo and MiniMax. Upstream uses Key authorization and request_id; creation paths and seller pricing IDs are registered separately. Shared provider.FalBilling extracts actual output tokens or seconds and selects explicit pricing variants. Vidu Q2 duration tiers and MiniMax composite input/free-image rules remain unpriced with measured usage retained. See [Fal configuration, billing boundaries and live evidence](../../../docs/development/qiniu-fal.md).

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../../logx/readme.md), [internal/provider](../readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [seedance_test.go](seedance_test.go) | `TestQiniuSeedanceKeepsTheBytedancePrefix` |
| [billing_test.go](billing_test.go) | `TestSeedanceMeasuredBands`, `TestSeedanceOnlySettlesExplicitSuccess`, `TestSeedanceSearchWithoutRateIsUnpriced` |
| [fal_test.go](fal_test.go) | Concrete route coverage, seller rates, final usage, missing context and incomplete pricing |

```bash
go test ./internal/provider/qiniu -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.

## Async billing

The native transport uses shared provider.SeedanceBilling() to record the creation model, time, resolution, and reference-video presence without retaining prompts or media URLs. Only successful results without business errors supply completion tokens. Actual resolution overrides the request. Seconds and total/prompt tokens do not substitute for video output tokens. Fal transports use provider.FalBilling() for queue envelopes and bare results.

Measured bands select wiv/woiv or 1080p/4K equivalents. Missing bands, missing context, or searches without a query rate remain unpriced, with measurements retained in the snapshot. The log drawer displays unknown pricing. Explicit rate tables do not fall through to catalog or flat prices; deliberately configured flat output prices remain supported.

Context shares the seven-day Redis affinity TTL; without Redis it is process-local. Stable IDs prevent repeated successful polls charging twice. Client polling triggers settlement. Background reconciliation, budget reservation, automatic repricing, supplier account reconciliation, and creation-time price-version locking are not implemented.

See [live evidence and reproduction](../../../docs/development/seedance-qiniu-validation.md). The paid live test requires XHUB_QINIU_SEEDANCE_LIVE=1 and QINIU_API_KEY in the environment.

The separate paid Fal test is TestQiniuFalLive in internal/dataplane/qiniu_live_test.go; it requires XHUB_QINIU_FAL_LIVE=1 and QINIU_API_KEY. Each run creates a paid video. Fal creation does not replay transport failures or HTTP 5xx responses because upstream may already have accepted the task.
