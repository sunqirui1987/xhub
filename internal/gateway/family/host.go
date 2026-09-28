// Package family shapes catalog resources that do not have their own handler. Inference operations go to the data plane. The rest are key-value reads and writes.
package family

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/auth"
	"github.com/sunqirui1987/xhub/internal/logx"
	"github.com/sunqirui1987/xhub/internal/store"
)

// ProxyVersion is written into the catalog config resource and returned as litellm_version by the process health details.
const ProxyVersion = "xhub-dev"

// Host is what catalog resource handlers ask the process for. *gateway.Server implements it. This package does not import gateway.
type Host interface {
	RequireLLM(w http.ResponseWriter, r *http.Request) *auth.Principal
	RequireMixed(w http.ResponseWriter, r *http.Request) *auth.Principal
	RequireManage(w http.ResponseWriter, r *http.Request) *auth.Principal
	DB() *store.Store
	// DataPlane hands an already recognized inference operation to the upstream loop. Do not call it when op is empty.
	DataPlane(w http.ResponseWriter, r *http.Request, op string)
	// EnforceIdentityLimits checks the model allow-list, budget, and rate. On rejection it has already written the response and returns false.
	EnforceIdentityLimits(w http.ResponseWriter, path string, p *auth.Principal, alias string, est int) bool
}

// traceModule records that catalog resource routes are being mounted.
func traceModule(name string) {
	logx.Trace("mount %s", name)
}
