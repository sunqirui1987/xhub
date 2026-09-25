// 控制台保存的模型。和配置文件同名时以数据库这一行为准。
package store

// ProxyModel 是控制台写进数据库的模型。和 YAML 同名时数据库行优先。
type ProxyModel struct {
	ID        string
	ModelName string
	Params    map[string]any
	Info      map[string]any
}
