// log.go shapes log lines. Hostnames replace full URLs, and key material
// is replaced before a line is written. The metrics line is the one place
// a finished call records TTFT and tokens per second.

package dataplane

import (
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
)

var secretInErr = regexp.MustCompile(`sk-[A-Za-z0-9_\-]+|(?i)bearer\s+\S+`)

// safeErr 把错误字符串里的 URL 收成主机名，并把 sk- 和 Bearer 换成 ***。
//
// 参数 err：上游或编码错误。nil 时返回空串。
// 返回：可以写进日志的文本。
// 调用：Serve 在编码失败和拨号失败时。测试：failure_log_test.go assertNoSecrets 检查日志行不含密钥。
func safeErr(err error) string {
	if err == nil {
		return ""
	}
	s := secretInErr.ReplaceAllString(err.Error(), "***")
	return regexp.MustCompile(`https?://[^\s"'<>]+`).ReplaceAllStringFunc(s, func(raw string) string {
		host := baseHost(strings.TrimRight(raw, `",)`))
		if host == "" {
			return "***"
		}
		return host
	})
}

// baseHost 取上游地址的主机名。用户信息、路径和查询串不进日志。
//
// 参数 apiBase：部署上的 api_base，或错误文本里的 URL。
// 返回：主机名。解析失败或没有主机时返回空串。
// 调用：safeErr、Serve 的调试日志。无单独测试。
func baseHost(apiBase string) string {
	u, err := url.Parse(apiBase)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

// trimBase 去掉 api_base 末尾的斜杠，避免和端点路径拼出双斜杠。
//
// 参数 s：原始根地址。
// 返回：去掉末尾 / 的地址。空串保持空串。
// 调用：Serve 拼上游 URL，official.go bypassAuth。无单独测试。
func trimBase(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

// logMetrics 把这次调用的 token、首字时间和总耗时写成一条进程日志。
// 参数 path（string）：入站路径，写进日志；model（string）：对外模型名；cacheHit（bool）：命中缓存时不计算每秒 token；pt（int）：输入 token；ct（int）：输出 token；ttft（time.Duration）：首字时间；elapsed（time.Duration）：从请求开始到结束的耗时。
// 返回：无。只写一条 Info 日志。
// 调用：Serve 的缓存命中、流式成功和非流式成功。测试：failure_log_test.go 检查这行指标出现。
func logMetrics(path, model string, cacheHit bool, pt, ct int, ttft, elapsed time.Duration) {
	window := elapsed - ttft
	if window <= 0 {
		window = elapsed
	}
	perSec := 0.0
	if !cacheHit {
		perSec = tokensPerSecond(ct, window)
	}
	logx.Info("process path=%s step=metrics model=%s cache_hit=%t ttft=%s prompt_tokens=%d completion_tokens=%d total_tokens=%d tokens_per_s=%.2f", path, model, cacheHit, ttft, pt, ct, pt+ct, perSec)
}

// tokensPerSecond 用完成 token 除以生成窗口。窗口或 token 为 0 时返回 0，不返回无穷大。
//
// 参数 completion：完成 token。window：总耗时减去首字节时间，不足时调用方改传总耗时。
// 返回：每秒 token 数。
// 调用：logMetrics。无单独测试。
func tokensPerSecond(completion int, window time.Duration) float64 {
	if completion <= 0 || window <= 0 {
		return 0
	}
	return float64(completion) / window.Seconds()
}
