// Package gateway is the process that listens for HTTP and owns the shared
// Server value. Subpackages (identity, keys, models, usage, prefs, guard,
// family) mount their own routes as modules. They do not import this package.
// This package implements their Host interfaces in wire.go.
//
// A request enters Handler in engine.go and then takes one of three paths:
//
//  1. A bypass endpoint (provider.Match, kind bypass) is handled in bypass.go
//     and forwarded by dataplane.ServeBypass. The chat loop is not used.
//  2. Anything else is dispatched by the Gin engine. Modules registered in
//     routes.go are mounted first. ingress.go then mounts every remaining
//     catalog path. The first method and path wins; a later registration is skipped.
//  3. A catalog path that is ordinary inference (handlerFor's default) calls
//     dataPlane in limits.go, which calls dataplane.Serve. Images, audio,
//     rerank, videos, responses, files, and realtime have their own handlers
//     in family, and those handlers also end in that same data plane.
//
// Spend rows for every one of those paths are written by recordSpend in
// spend.go. The call note (provider, TTFT, session, deployment) is attached
// with AnnotateCall before that write. Prompt storage is optional and keeps
// the request headers, body, and response on the same row.
//
// There is no "/" route. A path that matches neither a module nor the catalog
// nor a bypass endpoint is a JSON 404 from the engine's NoRoute.
package gateway
