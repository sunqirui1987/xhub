# XHub Documentation

**English** | [简体中文](README.zh-CN.md) · [Project home](../README.md)

Start with installation, then follow the user guide to connect a provider, create a workspace, and make your first request. Developers and coding agents can use the code map and implementation references below.

## Use and administer XHub

| Guide | What it covers | 中文 |
| --- | --- | --- |
| [Installation](getting-started.md) | Docker Compose, source setup, gateway and console configuration | [安装指南](getting-started.zh-CN.md) |
| [User and administration guide](user-guide.md) | Model setup, users, teams, projects, keys, routing, cost review, and troubleshooting | [客户使用与管理](user-guide.zh-CN.md) |

## Develop XHub

The central development references are in Chinese. Package-level `readme.md` and `readme_cn.md` files provide English and Chinese code guides.

| Reference | Purpose |
| --- | --- |
| [Developer and coding-agent guide](development/README.md) | Find the right module, preserve invariants, and choose checks for a change |
| [Architecture](development/architecture.md) | Services, request flow, ownership, and module boundaries |
| [Permissions](development/permissions.md) | Roles, visibility, secret access, and model scope |
| [Routing](development/routing.md) | Template inheritance, weights, retries, timeouts, and affinity |
| [Pricing and usage](development/pricing.md) | Rates, time windows, price snapshots, and persistence |
| [Quota implementation and verification](development/quota-chain.md) | Allocation invariants, request admission, spend attribution, and layered tests |
| [Runtime behavior and limits](development/runtime.md) | Cache, idempotency, protocols, media tasks, and operating limits |
| [Testing](development/testing.md) | Test layers, database isolation, focused commands, and optional demo data |
| [Feature implementation](development/implementation.md) | End-to-end contracts and source locations for core features |
| [HTTP API reference](development/api-reference.md) | Registered endpoints and their handler sources |
| [Configuration and storage](development/configuration.md) | Settings precedence, model fields, persistence, and runtime defaults |
| [Regression plan](development/regression.md) | File-by-file backend coverage, live models, template cases, and acceptance procedure |
| [Browser regression](development/e2e-regression.md) | Console flows, coverage matrix, environment and assertions |
| [Operations](development/operations.md) | Diagnose failures using call IDs, logs, state, and billing evidence |

## Maintain these documents

Document shipped behavior with code references. Update the relevant guide when behavior changes. Keep customer steps in the user guide and implementation details in the development references. Temporary plans, AI transcripts, review diaries, and test output do not belong here.

[`testdata/catalog.json`](testdata/catalog.json) is a test fixture consumed by automated checks; it is not a list of guaranteed provider capabilities. [`assets/`](assets/) contains the console screenshots used by the project home page.

- [组织、团队、个人与 API Key 额度管理](quota-management.md)：金额与 RPM/TPM 逐级固定分配、共享余额、单团队归属及不限额业务。
