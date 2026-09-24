// Package llm 是 xhub 的模型调用内核。
//
// LiteLLM 把同一种能力拆在几百个 Python 类里：OpenAI 聊天配置是基类，
// 大多数供应商只改默认地址、密钥来源和少量参数名。那种结构靠继承复用。
// Go 没有继承，如果按类逐个翻译，会得到一堆只改一个字符串的类型，调用方
// 仍然要自己拼 URL、自己组请求头。所以这里不照着 Python 包名铺目录，
// 而是按一次上游调用真正要决定的三件事来组织：
//
//  1. 打到哪个地址（Endpoint）
//  2. 带什么请求头（Headers）
//  3. 请求体和响应体怎么改写成对方的协议（Encode / Decode）
//
// 本包不发起 HTTP，也不读数据库。凭证是否齐全、选哪一个部署、失败后换
// 下一个部署，由网关数据面和 router 负责。它们把已经填好的 api_base、
// api_key 和模型名交进来，本包只回答「这次请求长什么样」。
// 这样测试可以比较地址和报文，而不必连接真实厂商。
//
// 供应商分成两类：
//
//   - OpenAI 兼容。聊天、向量、图像、语音、responses 都走同一套路径规则，
//     只是 api_base 不同。Z.ai、DashScope、Groq、Together、vLLM 等都属于
//     这一类，不各自复制一份实现。
//   - 协议不同。Azure 把模型放进部署路径，Anthropic 使用 Messages API，
//     Gemini 与 Vertex 使用 generateContent。它们有独立的编码函数。
//
// 未知但非空的供应商按 OpenAI 兼容处理。LiteLLM 里大量供应商就是这样
// 接到 OpenAI SDK 上的。空的供应商名不是一种协议，调用方应在进本包之前
// 当成凭证或配置错误。
package llm
