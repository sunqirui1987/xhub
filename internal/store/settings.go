package store

import "encoding/json"

func (s *Store) migrateConfig() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS proxy_config (
  namespace TEXT NOT NULL,
  key TEXT NOT NULL,
  value_json TEXT NOT NULL,
  PRIMARY KEY (namespace, key)
);`)
	return err
}

func (s *Store) PutConfig(namespace, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.DB.Exec(`INSERT INTO proxy_config (namespace, key, value_json) VALUES (?,?,?)
		ON CONFLICT (namespace, key) DO UPDATE SET value_json=EXCLUDED.value_json`, namespace, key, string(raw))
	return err
}

func (s *Store) DeleteConfig(namespace, key string) error {
	_, err := s.DB.Exec(`DELETE FROM proxy_config WHERE namespace=? AND key=?`, namespace, key)
	return err
}

func (s *Store) ListConfig(namespace string) (map[string]any, error) {
	rows, err := s.DB.Query(`SELECT key, value_json FROM proxy_config WHERE namespace=?`, namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]any{}
	for rows.Next() {
		var key, raw string
		if err := rows.Scan(&key, &raw); err != nil {
			return nil, err
		}
		var v any
		if json.Unmarshal([]byte(raw), &v) != nil {
			v = raw
		}
		out[key] = v
	}
	return out, rows.Err()
}
