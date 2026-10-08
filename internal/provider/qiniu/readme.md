# Qiniu official video-task transport

[简体中文](readme_cn.md) · [Feature implementation reference](../../../docs/development/implementation.md)

## Responsibilities and behavior

seedance.go registers qiniu_contents_generation with api.qnaigc.com as the default base. It supports POST /v3/contents/generations/tasks and GET the task path with /{id}, without a list action.
The model field is model and task ID field is id; the qiniu prefix is removed upstream. Three Seedance entries are contributed without usable baseline prices. Deployments can override base and credentials.
Dataplane owns sending, tenant task affinity, polling, and terminal settlement. Registration tests validate descriptions; real task tests require explicit bypass configuration and consume provider quota. Creation success alone does not prove completion or billing accuracy.

## Source responsibilities and entry points

### seedance.go

Internal implementation and protocol boundaries:  [seedance.go](seedance.go)。

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Dependencies

[internal/logx](../../logx/readme.md), [internal/provider](../readme.md).

## Verification and maintenance

| Test file | Scenario entry points |
| --- | --- |
| [seedance_test.go](seedance_test.go) | `TestQiniuSeedanceKeepsTheBytedancePrefix` |

```bash
go test ./internal/provider/qiniu -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
