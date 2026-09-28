# gateway/family

## Purpose

`family` answers catalog routes that do not have a dedicated handler. It shapes list, detail, and write JSON for those resources, and it forwards real inference operations to the data plane. The process mounts one route per `routes.json` entry and calls `ServeDataPlane`, `ServeMixed`, or `ServeMgmt`.

## How a request is dispatched

- A data-plane path such as `/v1/chat/completions` goes to `ServeDataPlane`, which calls `dataplane.Serve`.
- A management path such as a CRUD resource that has no module of its own goes to `ServeMgmt`.
- A mixed path can be either, and `ServeMixed` chooses from the method and the body.

You normally do not call these functions yourself. You call the HTTP path. The master key or a virtual key is required exactly as `catalog.AuthOf` classifies that path.

## Public version

`ProxyVersion` is the string health details return as `litellm_version`. The process sets `gateway.Version` to the same value. Clients that check the header should treat `xhub-dev` as this build unless you change the constant.

## What this package does not do

It does not own users, keys, or the router-settings page. Those have their own modules and win because they are registered first.

中文使用说明见同目录的 readme_cn.md。
