package dataplane

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/sunqirui1987/xhub/internal/config"
	"github.com/sunqirui1987/xhub/internal/llm"
	"github.com/sunqirui1987/xhub/internal/provider"
)

// errDialogueCredentials 区分连接配置缺失与协议编码错误，供数据面返回上游鉴权错误。
var errDialogueCredentials = errors.New("upstream address and credentials required")

// callerProtocol 将明确用户操作转换为协议 ID；参数为操作名，返回协议或空值。
// 调用：统一对话编解码；不从供应商或 URL 猜测上游协议。
func callerProtocol(op string) string {
	return map[string]string{"chat": "openai-chat", "responses": "openai-responses", "messages": "anthropic-messages"}[op]
}

// dialogueRequestBody 复制统一对话正文，并移除已经由网关消费、不能传给协议转换器的代理字段。
// 参数 body 是护栏可能已经原地改写的入站正文；返回独立浅拷贝，保留改写后的消息和真正的协议字段。
// 调用场景是 Serve 在 ParseDialogue 前整理请求；未知协议能力仍交给解析器明确拒绝。
// guardrails 决定本次执行的规则，litellm_trace_id 只用于网关追踪，两者都不是上游对话能力。
func dialogueRequestBody(body map[string]any) map[string]any {
	out := make(map[string]any, len(body))
	for key, value := range body {
		out[key] = value
	}
	delete(out, "guardrails")
	delete(out, "litellm_trace_id")
	return out
}

// buildDialogue 使用注册执行配置生成路径、鉴权与正文。参数为部署及已解析对话；返回请求或能力错误。
// 调用：统一入口；只使用注册 create 操作，禁止客户端指定目标地址。
func buildDialogue(dep config.ModelEntry, d llm.Dialogue) (llm.Upstream, error) {
	t, ok := provider.Execution(dep)
	if !ok {
		return llm.Upstream{}, fmt.Errorf("explicit upstream execution profile required")
	}
	payload, err := llm.EncodeDialogue(d, t.Protocol, dep.ParamString("model", ""))
	if err != nil {
		return llm.Upstream{}, err
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return llm.Upstream{}, err
	}
	for _, a := range t.Actions {
		if a.Name != "create" {
			continue
		}
		hit := provider.Hit{Transport: t, Action: a}
		base, key := bypassAuth(hit, dep)
		if base == "" || key == "" {
			return llm.Upstream{}, errDialogueCredentials
		}
		header := http.Header{"Content-Type": []string{"application/json"}}
		for k, v := range t.Headers {
			header.Set(k, v)
		}
		value := key
		if t.Auth.Prefix != "" {
			value = t.Auth.Prefix + " " + value
		}
		if t.Auth.Header == "" {
			return llm.Upstream{}, fmt.Errorf("registered authentication header required")
		}
		header.Set(t.Auth.Header, value)
		return llm.Upstream{URL: bypassDeploymentURL(base, hit, dep), Header: header, Body: raw}, nil
	}
	return llm.Upstream{}, fmt.Errorf("create operation is not registered")
}

// buildRegisteredOperation 使用部署显式选择的传输构造非对话请求。
// 参数 dep 是已经附加凭据的部署，body 是用户请求，op 仅用于错误定位；返回固定注册路径、鉴权头和 JSON 正文。
// 调用场景是 embedding、图片、音频、重排等统一入口；函数不会根据供应商名、模型前缀或主机名推断协议。
// 缺少地址或密钥时返回 errDialogueCredentials；未登记 create 动作、鉴权头或序列化失败时返回具体错误，且不修改调用方正文。
func buildRegisteredOperation(dep config.ModelEntry, body map[string]any, op string) (llm.Upstream, error) {
	t, ok := provider.Execution(dep)
	if !ok {
		return llm.Upstream{}, fmt.Errorf("explicit upstream execution profile required for %s", op)
	}
	for _, a := range t.Actions {
		if a.Name != "create" {
			continue
		}
		hit := provider.Hit{Transport: t, Action: a}
		base, key := bypassAuth(hit, dep)
		if base == "" || key == "" {
			return llm.Upstream{}, errDialogueCredentials
		}
		if t.Auth.Header == "" {
			return llm.Upstream{}, fmt.Errorf("registered authentication header required")
		}
		payload := make(map[string]any, len(body)+1)
		for k, v := range body {
			payload[k] = v
		}
		llm.StripProxyParams(payload)
		if t.ModelField != "" && a.Model == "" {
			payload[t.ModelField] = provider.OfficialID(t.StripPrefix, dep.ParamString("model", ""))
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return llm.Upstream{}, err
		}
		header := http.Header{"Content-Type": []string{"application/json"}}
		for k, v := range t.Headers {
			header.Set(k, v)
		}
		value := key
		if t.Auth.Prefix != "" {
			value = t.Auth.Prefix + " " + value
		}
		header.Set(t.Auth.Header, value)
		return llm.Upstream{URL: bypassDeploymentURL(base, hit, dep), Header: header, Body: raw}, nil
	}
	return llm.Upstream{}, fmt.Errorf("create operation is not registered for %s", op)
}
