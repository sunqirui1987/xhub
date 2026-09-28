// Package keys registers virtual-key routes next to their handlers. The process only mounts this module.
package keys

import (
	"net/http"

	"github.com/sunqirui1987/xhub/internal/httpx"
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceMount sync.Once

// Module creates, lists, updates, and rotates virtual keys.
func Module(h Host) httpx.Module {
	logTraceOnceMount.Do(func() { logx.Trace("enter keys.Module") })

	traceModule("keys")
	return httpx.Bind("keys", func(reg httpx.Registrar) {
		reg.Handle("POST /key/generate", func(w http.ResponseWriter, r *http.Request) { Generate(h, w, r) })
		reg.Handle("POST /key/service-account/generate", func(w http.ResponseWriter, r *http.Request) { ServiceAccount(h, w, r) })
		reg.Handle("GET /key/list", func(w http.ResponseWriter, r *http.Request) { List(h, w, r) })
		reg.Handle("GET /key/info", func(w http.ResponseWriter, r *http.Request) { Info(h, w, r) })
		reg.Handle("POST /v2/key/info", func(w http.ResponseWriter, r *http.Request) { Info(h, w, r) })
		reg.Handle("POST /key/delete", func(w http.ResponseWriter, r *http.Request) { Delete(h, w, r) })
		reg.Handle("POST /key/block", func(w http.ResponseWriter, r *http.Request) { Block(h, w, r) })
		reg.Handle("POST /key/unblock", func(w http.ResponseWriter, r *http.Request) { Unblock(h, w, r) })
		reg.Handle("POST /key/update", func(w http.ResponseWriter, r *http.Request) { Update(h, w, r) })
		reg.Handle("POST /key/bulk_update", func(w http.ResponseWriter, r *http.Request) { BulkUpdate(h, w, r) })
		reg.Handle("POST /key/regenerate", func(w http.ResponseWriter, r *http.Request) { Regenerate(h, w, r) })
		reg.Handle("POST /key/{key}/regenerate", func(w http.ResponseWriter, r *http.Request) { Regenerate(h, w, r) })
		reg.Handle("POST /key/{key}/reset_spend", func(w http.ResponseWriter, r *http.Request) { ResetSpend(h, w, r) })
		reg.Handle("GET /key/aliases", func(w http.ResponseWriter, r *http.Request) { Aliases(h, w, r) })
		reg.Handle("POST /key/health", func(w http.ResponseWriter, r *http.Request) { Health(h, w, r) })
	})
}
