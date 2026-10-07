// Package store persists gateway records in PostgreSQL. Dashboard-saved models override a YAML entry with the same name.
package store

import "github.com/sunqirui1987/xhub/internal/logx"

// ProxyModel is a model the dashboard stored in the database. When the name also exists in YAML, this row wins.
type ProxyModel struct {
	ID        string
	ModelName string
	Params    map[string]any
	Info      map[string]any
}

// traceProxyModels records that the process is reading dashboard-saved models. Names and parameters stay out of the line.
// 参数：无。
// 调用：store/keys.go
// 测试：无直接单测
// 返回：无。已记一条正在读取控制台保存的模型的日志。名字和参数不进这一行。
func traceProxyModels() {
	logx.Trace("list proxy models")
}
