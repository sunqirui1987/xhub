# Backend module map

[简体中文](README.zh-CN.md) · [Feature implementation reference](../docs/development/implementation.md)

## Responsibilities and behavior

This tree implements identity, the management control plane, inference execution, routing, pricing, and persistence. Command packages compose the services; [cmd/regression](../cmd/regression/readme.md) contains business regression tests and their shared fixtures. Exported Go symbols are implementation seams inside the repository; HTTP contracts and configuration documents are the client boundary.
Inference resolves a session or virtual key, reloads ownership, checks action permissions, model access, budgets, and limits, selects one routing document, and executes an eligible deployment. Successful usage is priced with a recorded rate snapshot and historical ownership. Cache and affinity are committed only after success.
iam owns identity and accounting; store owns framework configuration and deployments; live owns Redis hot state; cache owns process-local response bytes. Scope totals are different views of one charge. Never concatenate a second upstream response after stream output or acknowledge a queue item before durable commit.

## Subdirectories and collaboration

| Directory | Responsibility |
| --- | --- |
| [auth](auth/readme.md) | Caller authentication |
| [authz](authz/readme.md) | Action authorization and query scopes |
| [cache](cache/readme.md) | Process-local response cache |
| [catalog](catalog/readme.md) | Model catalog, rates, and normalized usage |
| [config](config/readme.md) | Startup configuration and model definitions |
| [dataplane](dataplane/readme.md) | Inference execution and settlement |
| [gateway](gateway/readme.md) | HTTP gateway and module composition |
| [hooks](hooks/readme.md) | In-flight request accounting |
| [httpx](httpx/readme.md) | HTTP errors and module contracts |
| [iam](iam/readme.md) | Identity resources and transactional accounting |
| [live](live/readme.md) | Redis hot state and usage queue |
| [llm](llm/readme.md) | Upstream construction and protocol adaptation |
| [logx](logx/readme.md) | Runtime logging and secret redaction |
| [plugin](plugin/readme.md) | Upstream extension registry |
| [provider](provider/readme.md) | Provider capabilities and transports |
| [router](router/readme.md) | Deployment filtering, ordering, and weighted split |
| [store](store/readme.md) | Framework configuration and deployment storage |

## Source responsibilities and entry points

This directory contains resources, subpackages, or tests without independent production Go implementation.

## External HTTP boundary

This directory registers no direct HTTP route. Higher layers call its Go API; trace catalog dispatch or host calls through the dependency chain.

## Verification and maintenance

There are no direct test files here. Integration tests prove executed paths rather than every internal failure branch.

```bash
go test ./internal/... -count=1
```

Use XHUB_REGRESSION_STRICT=1 for database acceptance and inspect skips. Redis and live providers require separate configuration. Update this reference and feature documentation after contract changes.
