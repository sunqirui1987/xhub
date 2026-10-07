# plugin

The pre-upstream extension point. This package does not import gateway and does not ship a built-in extension. Callers register implementations by a stable `Name`.

`Call` is what an extension may read: `Op`, `Model` (the public alias), and `Path`. The body is not on this struct.

`Decision.Refuse == true` tells `dataplane.Serve` to skip the response cache and not contact the upstream. `Status`, `Code`, and `Message` are the error the data plane writes. `Header` is copied onto the response even when the call is allowed, so a client can see that the extension ran.

`New` returns an empty registry. The zero `Registry` value is not usable. `Register` appends in order. An empty name or a duplicate name returns an error and leaves the existing order unchanged. `Names` returns a copy.

`Run` calls `BeforeUpstream` in registration order. The first refusal stops the rest and keeps headers already set. An empty registry returns a zero `Decision`, and `Serve` continues. `Invoke` runs one name; a missing name returns an error and does not call any other extension.

`Serve` calls `Run` after guardrails and after `hooks.Begin`, and before the cache lookup. `ServeBypass` does not call it. A refusal here is not a guardrail block and is not a budget failure.

## What this package does not do

It does not persist decisions, does not read Redis, and does not know about teams. An extension that needs that context has to close over it itself. There is no hook after the upstream returns.

中文说明见同目录 `readme_cn.md`。
