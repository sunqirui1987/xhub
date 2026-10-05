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

// ListProxyModels lists every proxy model. A nil store has no models, which is
// the same answer as an empty table.
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
func (s *Store) DeleteProxyModel(id string) error {
	_, err := s.Engine.ID(id).Delete(&proxyModelRow{})
	s.bust(new(proxyModelRow))
	return err
}

// PutConfig writes one namespaced configuration row. An existing row is updated and a missing row is inserted.
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
func (s *Store) DeleteConfig(namespace, key string) error {
	_, err := s.Engine.ID(coreIDs(namespace, key)).Delete(&configRow{})
	s.bust(new(configRow))
	return err
}

// ListConfig reads every configuration row in one namespace. A nil store has no
// configuration, so a caller that overlays it keeps the YAML baseline instead of
// crashing on a deployment that runs without framework records.
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
