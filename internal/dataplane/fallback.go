package dataplane

import (
	"encoding/json"
	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/gateway/prefs"
	"github.com/sunqirui1987/xhub/internal/router"
	"strings"
)

// fallbackKind 分类上游失败，参数为状态和错误正文，返回 general/context/content 或空。
// 调用：适配与原生执行；仅识别上游错误包络，不把成功文本或本地护栏当作触发器。
func fallbackKind(status int, body []byte) string {
	if status >= 500 || status == 429 {
		return "general"
	}
	if status < 400 {
		return ""
	}
	var doc map[string]any
	if json.Unmarshal(body, &doc) != nil {
		return ""
	}
	raw, ok := doc["error"]
	if !ok {
		return ""
	}
	text, _ := json.Marshal(raw)
	s := strings.ToLower(string(text))
	for _, marker := range []string{"context_length_exceeded", "context_window_exceeded", "maximum context length", "prompt is too long", "input is too long"} {
		if strings.Contains(s, marker) {
			return "context"
		}
	}
	for _, marker := range []string{"content_policy_violation", "content_filter", "content filtering policy", "safety policy", "safety_violation"} {
		if strings.Contains(s, marker) {
			return "content"
		}
	}
	return ""
}

// fallbackQueue 保存请求私有的有序模型队列，最多访问 32 个不同模型以限制链放大。
// 已输出流式字节或响应继续请求不调用队列；重复、禁用和不兼容目标不重放。
type fallbackQueue struct {
	settings  prefs.RouteSettings
	pending   []string
	seen      map[string]bool
	current   string
	poolAlias string
	disabled  bool
}

// newFallbackQueue 初始化请求状态，参数为完整配置、起始模型和禁用标志。
// 返回私有队列，不修改共享设置；调用：两类数据面创建入口。
func newFallbackQueue(settings prefs.RouteSettings, alias string, disabled bool) *fallbackQueue {
	return &fallbackQueue{settings: settings, current: alias, poolAlias: alias, seen: map[string]bool{alias: true}, disabled: disabled}
}

// next 在当前模型耗尽后加入对应错误目标并选择下一兼容模型。
// 参数为错误类型、兼容部署与运行状态；返回部署和模型配置，耗尽或禁用时为空。
// 调用方发送前验证目标权限与预算；空池跳过，配置错误通过 Err 返回，无共享状态修改。
func (q *fallbackQueue) next(kind string, models []config.ModelEntry, state router.State) ([]config.ModelEntry, prefs.RouteSettings) {
	if q.disabled {
		return nil, q.settings
	}
	q.pending = append(q.pending, q.settings.ModelFallbacks[q.current].Targets(kind)...)
	for len(q.pending) > 0 && len(q.seen) < 32 {
		name := q.pending[0]
		q.pending = q.pending[1:]
		if q.seen[name] {
			continue
		}
		q.seen[name] = true
		q.current = name
		q.poolAlias = name
		settings := q.settings.ForModel(name)
		if settings.Err != nil {
			return nil, settings
		}
		state.Allocations = settings.Policy.Shares()
		pool := settings.Schedule(models, name, state, "")
		if len(pool) > 0 {
			return pool, settings
		}
		q.pending = append(q.pending, q.settings.ModelFallbacks[name].Targets("general")...)
	}
	return nil, q.settings
}

// useDeployment 选择本次失败的回退源，显式组链优先，否则采用当前真实成员。
// 参数 name 为部署公开模型名；两类数据面发送前调用，返回空，仅更新请求私有状态。
// poolAlias 在切换回退池时重置，避免第一次成员的配置粘住后续成员。
func (q *fallbackQueue) useDeployment(name string) {
	q.current = name
	if _, explicit := q.settings.ModelFallbacks[q.poolAlias]; explicit {
		q.current = q.poolAlias
	}
}

// hasFallbackTargets 判断三类错误是否任一可重放；原生入口选择缓冲响应时调用，无修改。
func hasFallbackTargets(p router.FallbackPolicy) bool {
	return len(p.Fallbacks)+len(p.ContextWindow)+len(p.ContentPolicy) > 0
}
