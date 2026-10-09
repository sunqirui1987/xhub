// Package prefs 保留控制台设置接口；路由解析与执行统一位于 internal/router。
package prefs

import "github.com/sunqirui1987/xhub/internal/router"

type RouteSettings = router.RouteSettings
type ScopeRef = router.ScopeRef
type ScopeLookup = router.ScopeLookup

const BuiltinSource = router.BuiltinSource

var BuiltinDocument = router.BuiltinDocument
var BuiltinSettings = router.BuiltinSettings
var Resolve = router.Resolve
var RequestChain = router.RequestChain
var ValidateRouteTemplateDocument = router.ValidateRouteTemplateDocument
var TemplateRouting = router.TemplateRouting
var ValidateTemplateCatalog = router.ValidateTemplateCatalog
