// Package logx writes process logs to the standard logger. Every line starts with trace, debug, info, or error.
// Secrets are removed here so a caller cannot print a bearer token or an sk- key by accident.
package logx

import (
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
)

// Trace records a step inside a call, such as entering the data plane.
// 调用：authz/authz.go、cache/cache.go、catalog/classify.go、catalog/cost.go
// 测试：无直接单测
// 参数 format（string）：日志格式串，按 fmt 动词使用，不是 HTTP 路径；args（...any）：填进格式串的值，不会当成 HTTP 正文写出。
// 返回：无。只写进程日志，不写 HTTP 响应。
func Trace(format string, args ...any) { write("trace", format, args...) }

// Debug records a decision that is useful when a call did not do what the operator expected, such as skipping a deployment.
// 调用：auth/auth.go、catalog/embed.go、dataplane/serve.go、gateway/catalog.go
// 测试：无直接单测
// 参数 format（string）：日志格式串，按 fmt 动词使用，不是 HTTP 路径；args（...any）：填进格式串的值，不会当成 HTTP 正文写出。
// 返回：无。只写进程日志，不写 HTTP 响应。
func Debug(format string, args ...any) { write("debug", format, args...) }

// Info records a normal outcome, such as the process listening or one HTTP request finishing.
// 调用：dataplane/log.go、dataplane/serve.go、gateway/engine.go、gateway/keys/generate.go
// 测试：files_test.go
// 参数 format（string）：日志格式串，按 fmt 动词使用，不是 HTTP 路径；args（...any）：填进格式串的值，不会当成 HTTP 正文写出。
// 返回：无。只写进程日志，不写 HTTP 响应。
func Info(format string, args ...any) { write("info", format, args...) }

// Error records a failure. The process keeps running unless the caller exits after this returns.
// 调用：auth/auth.go、authz/authz.go、authz/decide.go、authz/scope.go
// 测试：无直接单测
// 参数 format（string）：日志格式串，按 fmt 动词使用，不是 HTTP 路径；args（...any）：填进格式串的值，不会当成 HTTP 正文写出。
// 返回：无。只写进程日志，不写 HTTP 响应。
func Error(format string, args ...any) { write("error", format, args...) }

// 把一条日志写到标准日志。写出前先打码，避免 bearer 和 sk- 进入日志。
// 参数 level（string）：日志级别名，例如 trace、debug、info、error；format（string）：日志格式串，按 fmt 动词使用，不是 HTTP 路径；args（...any）：填进格式串的值，不会当成 HTTP 正文写出。
// 返回：无。只写进程日志，不写 HTTP 响应。
// 调用：Trace、Debug、Info、Error。
// 测试：无直接单测
func write(level, format string, args ...any) {
	log.Printf("%s %s", level, redact(fmt.Sprintf(format, args...)))
}

var (
	bearerValue = regexp.MustCompile(`(?i)bearer\s+\S+`)
	secretValue = regexp.MustCompile(`(?i)sk-[A-Za-z0-9_-]+`)
	httpURL     = regexp.MustCompile(`https?://[^\s"'<>]+`)
)

// redact strips bearer tokens, sk- keys, and full upstream URLs. A URL becomes its hostname. The replacement does not itself contain bearer, sk-, or a scheme.
// 参数 s（string）：即将写入日志的一行，可能含 bearer、sk- 或上游 URL。
// 返回 string（string）：打码后的日志行。bearer 和 sk- 换成星号，URL 只留主机名。
// 调用：仅在 log.go 内使用
// 测试：无直接单测
func redact(s string) string {
	s = bearerValue.ReplaceAllString(s, "***")
	s = secretValue.ReplaceAllString(s, "***")
	return httpURL.ReplaceAllStringFunc(s, func(raw string) string {
		u, err := url.Parse(strings.TrimRight(raw, `"'.,);`))
		if err != nil || u.Hostname() == "" {
			return "***"
		}
		return u.Hostname()
	})
}
