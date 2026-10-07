// Package store reads and writes dashboard-saved proxy models and namespaced
// configuration rows. Identity and usage live in internal/iam.
package store

import (
	"encoding/json"
	"time"

	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
	"xorm.io/builder"
)

var logTraceOnceKeys sync.Once

// UpsertProxyModel updates a proxy model by id, or inserts it when the row is missing.
// 参数 m（ProxyModel）：正在累加或展示的ProxyModel。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/models/admin.go、gateway/models/builtin.go
// 测试：builtin_providers_test.go、chains_test.go
func (s *Store) UpsertProxyModel(m ProxyModel) error {
	logTraceOnceKeys.Do(func() { logx.Trace("enter store.UpsertProxyModel") })

	if m.Params == nil {
		m.Params = map[string]any{}
	}
	if m.Info == nil {
		m.Info = map[string]any{}
	}
	params, err := json.Marshal(m.Params)
	if err != nil {
		return err
	}
	info, err := json.Marshal(m.Info)
	if err != nil {
		return err
	}
	row := proxyModelRow{ID: m.ID, ModelName: m.ModelName, Params: string(params), Info: string(info), UpdatedAt: time.Now().UTC().Format(time.RFC3339)}
	n, err := s.Engine.ID(m.ID).Cols("model_name", "litellm_params_json", "model_info_json", "updated_at").Update(&row)
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err = s.Engine.Insert(&row); err != nil {
			return err
		}
	}
	s.bust(new(proxyModelRow))
	return nil
}

// ListProxyModels lists every proxy model. A nil store has no models, which is the same answer as an empty table.
// 参数：无。
// 调用：gateway/models/admin.go、gateway/models/builtin.go
// 测试：builtin_providers_test.go、chains_test.go
// 返回 []ProxyModel（[]ProxyModel）：库存里的模型行；error（error）：失败原因，nil 表示成功。
func (s *Store) ListProxyModels() ([]ProxyModel, error) {
	traceProxyModels()
	if s == nil {
		return nil, nil
	}
	var rows []proxyModelRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]ProxyModel, 0, len(rows))
	for _, r := range rows {
		m := ProxyModel{ID: r.ID, ModelName: r.ModelName}
		_ = json.Unmarshal([]byte(r.Params), &m.Params)
		_ = json.Unmarshal([]byte(r.Info), &m.Info)
		if m.Params == nil {
			m.Params = map[string]any{}
		}
		if m.Info == nil {
			m.Info = map[string]any{}
		}
		out = append(out, m)
	}
	return out, nil
}

// DeleteProxyModel deletes a proxy model by id and clears the cache.
// 参数 id（string）：部署、模型或凭据 id。空串表示没有选定。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/models/admin.go、gateway/models/builtin.go
// 测试：builtin_providers_test.go、chains_test.go
func (s *Store) DeleteProxyModel(id string) error {
	_, err := s.Engine.ID(id).Delete(&proxyModelRow{})
	s.bust(new(proxyModelRow))
	return err
}

// PutConfig writes one namespaced configuration row. An existing row is updated and a missing row is inserted.
// 参数 namespace（string）：放入配置使用的namespace。空串表示调用方没有提供这项；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥；value（any）：JSON 里读出的动态值。数字、字符串和对象都要接住，类型不符时按零值而不是 panic。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/models/cost_reload.go、gateway/prefs/settings.go
// 测试：无直接单测
func (s *Store) PutConfig(namespace, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	row := configRow{Namespace: namespace, Key: key, ValueJSON: string(raw)}
	n, err := s.Engine.ID(coreIDs(namespace, key)).Cols("value_json").Update(&row)
	if err != nil {
		return err
	}
	if n == 0 {
		if _, err = s.Engine.Insert(&row); err != nil {
			return err
		}
	}
	s.bust(new(configRow))
	return nil
}

// DeleteConfig deletes one namespaced configuration row and clears the cache.
// 参数 namespace（string）：删除配置使用的namespace。空串表示调用方没有提供这项；key（string）：上游或调用方的密钥。空串表示还不能转发或还没有密钥。
// 返回 error（error）：失败原因，nil 表示这一步成功。
// 调用：gateway/prefs/settings.go
// 测试：无直接单测
func (s *Store) DeleteConfig(namespace, key string) error {
	_, err := s.Engine.ID(coreIDs(namespace, key)).Delete(&configRow{})
	s.bust(new(configRow))
	return err
}

// ListConfig reads every configuration row in one namespace. A nil store has no configuration, so a caller that overlays it keeps the YAML baseline instead of crashing on a deployment that runs without framework records.
// 参数 namespace（string）：配置命名空间，例如 general 或 router。
// 返回：该空间里键到 JSON 值的表。Store 为 nil 时返回空表而不是错误，调用方就继续用 YAML。库错误时表为 nil。
// 调用：gateway 合并数据库里的设置覆盖。
// 测试：无直接单测。
func (s *Store) ListConfig(namespace string) (map[string]any, error) {
	if s == nil {
		return map[string]any{}, nil
	}
	var rows []configRow
	if err := s.Engine.Where(builder.Eq{"namespace": namespace}).Find(&rows); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, r := range rows {
		var v any
		if json.Unmarshal([]byte(r.ValueJSON), &v) != nil {
			v = r.ValueJSON
		}
		out[r.Key] = v
	}
	return out, nil
}
