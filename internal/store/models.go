package store

import (
	"encoding/json"
	"time"
)

// ProxyModel is a dashboard-created deployment. Config-file models are not stored here.
type ProxyModel struct {
	ID        string
	ModelName string
	Params    map[string]any
	Info      map[string]any
}

func (s *Store) migrateProxyModels() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS proxy_models (
  id TEXT PRIMARY KEY,
  model_name TEXT NOT NULL,
  litellm_params_json TEXT NOT NULL,
  model_info_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);`)
	return err
}

func (s *Store) UpsertProxyModel(m ProxyModel) error {
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
	_, err = s.DB.Exec(`INSERT INTO proxy_models (id, model_name, litellm_params_json, model_info_json, updated_at)
		VALUES (?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
		  model_name=excluded.model_name,
		  litellm_params_json=excluded.litellm_params_json,
		  model_info_json=excluded.model_info_json,
		  updated_at=excluded.updated_at`,
		m.ID, m.ModelName, string(params), string(info), time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) ListProxyModels() ([]ProxyModel, error) {
	rows, err := s.DB.Query(`SELECT id, model_name, litellm_params_json, model_info_json FROM proxy_models`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ProxyModel
	for rows.Next() {
		var id, name, paramsJSON, infoJSON string
		if err := rows.Scan(&id, &name, &paramsJSON, &infoJSON); err != nil {
			return nil, err
		}
		m := ProxyModel{ID: id, ModelName: name}
		_ = json.Unmarshal([]byte(paramsJSON), &m.Params)
		_ = json.Unmarshal([]byte(infoJSON), &m.Info)
		if m.Params == nil {
			m.Params = map[string]any{}
		}
		if m.Info == nil {
			m.Info = map[string]any{}
		}
		out = append(out, m)
	}
	if out == nil {
		out = []ProxyModel{}
	}
	return out, rows.Err()
}

func (s *Store) DeleteProxyModel(id string) error {
	_, err := s.DB.Exec(`DELETE FROM proxy_models WHERE id=?`, id)
	return err
}
