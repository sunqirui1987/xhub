package llm

import "net/http"

// Upstream 保存执行器产生的上游请求，不包含供应商推断或地址默认值。
// URL、Header、Body 由显式执行配置和协议编码器构造，供数据面发送。
type Upstream struct {
 URL string
 Header http.Header
 Body []byte
}
