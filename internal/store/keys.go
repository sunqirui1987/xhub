// 密钥、花费日志、代理模型和配置的读写。
package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"xorm.io/builder"
)

// 把密钥转成表行。空模型列表和时间会补上默认值。
func tokenTo(k Key) tokenRow {
	if k.ModelsJSON == "" {
		k.ModelsJSON = "[]"
	}
	if k.KeyType == "" {
		k.KeyType = "default"
	}
	created := k.CreatedAt
	if created.IsZero() {
		created = time.Now().UTC()
	}
	row := tokenRow{
		Token: k.TokenHash, KeyAlias: k.KeyAlias, KeyName: k.KeyName, UserID: k.UserID,
		TeamID: k.TeamID, OrgID: k.OrganizationID, ProjectID: k.ProjectID, AgentID: k.AgentID,
		BudgetID: k.BudgetID, KeyType: k.KeyType, ModelsJSON: k.ModelsJSON,
		MaxBudget: fptr(k.MaxBudget), SoftBudget: fptr(k.SoftBudget), Spend: k.Spend,
		TPM: iptr(k.TPMLimit), RPM: iptr(k.RPMLimit), MaxParallel: iptr(k.MaxParallel),
		Duration: k.BudgetDuration, MetadataJSON: k.MetadataJSON, TagsJSON: k.TagsJSON,
		CreatedAt: created.UTC().Format(time.RFC3339),
	}
	if k.Blocked.Valid {
		v := boolInt(k.Blocked.Bool)
		row.Blocked = &v
	}
	if k.ExpiresAt.Valid {
		row.ExpiresAt = k.ExpiresAt.Time.UTC().Format(time.RFC3339)
	}
	if k.BudgetResetAt.Valid {
		row.ResetAt = k.BudgetResetAt.Time.UTC().Format(time.RFC3339)
	}
	return row
}

// 把表行转回密钥。
func tokenFrom(r tokenRow) Key {
	k := Key{
		TokenHash: r.Token, KeyAlias: r.KeyAlias, KeyName: r.KeyName, UserID: r.UserID,
		TeamID: r.TeamID, OrganizationID: r.OrgID, ProjectID: r.ProjectID, AgentID: r.AgentID,
		BudgetID: r.BudgetID, KeyType: r.KeyType, ModelsJSON: nz(r.ModelsJSON, "[]"),
		MaxBudget: nullF(r.MaxBudget), SoftBudget: nullF(r.SoftBudget), Spend: r.Spend,
		TPMLimit: nullI(r.TPM), RPMLimit: nullI(r.RPM), MaxParallel: nullI(r.MaxParallel),
		BudgetDuration: r.Duration, MetadataJSON: r.MetadataJSON, TagsJSON: r.TagsJSON,
		CreatedAt: parseRFC(r.CreatedAt),
	}
	if r.Blocked != nil {
		k.Blocked = sql.NullBool{Bool: *r.Blocked != 0, Valid: true}
	}
	if t := parseRFC(r.ExpiresAt); !t.IsZero() {
		k.ExpiresAt = sql.NullTime{Time: t, Valid: true}
	}
	if t := parseRFC(r.ResetAt); !t.IsZero() {
		k.BudgetResetAt = sql.NullTime{Time: t, Valid: true}
	}
	return k
}

// 插入一把密钥，并清掉密钥缓存。
func (s *Store) InsertKey(k Key) error {
	_, err := s.Engine.Insert(tokenTo(k))
	s.bust(new(tokenRow))
	return err
}

// 按哈希更新密钥上可改的字段。找不到返回无行。
func (s *Store) UpdateKey(k Key) error {
	row := tokenTo(k)
	n, err := s.Engine.ID(k.TokenHash).Cols(
		"key_alias", "key_name", "user_id", "team_id", "organization_id", "project_id", "agent_id", "budget_id",
		"key_type", "models_json", "max_budget", "soft_budget", "tpm_limit", "rpm_limit", "max_parallel_requests",
		"blocked", "expires_at", "budget_duration", "budget_reset_at", "metadata_json", "tags_json",
	).Update(&row)
	s.bust(new(tokenRow))
	return rowsAffected(n, err)
}

// 按哈希读取密钥。没有这一行时返回无行错误。
func (s *Store) GetByHash(hash string) (*Key, error) {
	var r tokenRow
	ok, err := s.one(&r, hash)
	if err != nil || !ok {
		if err == nil {
			err = sql.ErrNoRows
		}
		return nil, err
	}
	k := tokenFrom(r)
	return &k, nil
}

// 按创建时间从新到旧列出密钥。
func (s *Store) ListKeys() ([]Key, error) {
	var rows []tokenRow
	if err := s.Engine.Desc("created_at").Find(&rows); err != nil {
		return nil, err
	}
	out := make([]Key, 0, len(rows))
	for _, r := range rows {
		out = append(out, tokenFrom(r))
	}
	return out, nil
}

// 按哈希删除密钥，并清掉缓存。
func (s *Store) DeleteHash(hash string) error {
	_, err := s.Engine.ID(hash).Delete(&tokenRow{})
	s.bust(new(tokenRow))
	return err
}

// 设置密钥是否屏蔽。
func (s *Store) SetBlocked(hash string, blocked bool) error {
	v := boolInt(blocked)
	_, err := s.Engine.ID(hash).Cols("blocked").Update(&tokenRow{Blocked: &v})
	s.bust(new(tokenRow))
	return err
}

// 给这把密钥加上一笔花费。密钥不存在时返回无行。
func (s *Store) AddSpend(hash string, delta float64) error {
	n, err := s.Engine.ID(hash).Incr("spend", delta).Update(&tokenRow{})
	s.bust(new(tokenRow))
	return rowsAffected(n, err)
}

// 把密钥花费设成给定值。密钥不存在时返回无行。
func (s *Store) SetSpend(hash string, spend float64) error {
	n, err := s.Engine.ID(hash).Cols("spend").Update(&tokenRow{Spend: spend})
	s.bust(new(tokenRow))
	return rowsAffected(n, err)
}

// 写入一条请求花费日志。状态为空时记成成功。
func (s *Store) InsertSpendLog(requestID, callType, model, apiKeyHash string, prompt, completion int, spend sql.NullFloat64, start, end time.Time, cacheHit bool, status string) error {
	if status == "" {
		status = "success"
	}
	row := spendRow{
		RequestID: requestID, CallType: callType, Model: model, APIKey: apiKeyHash,
		Prompt: prompt, Completion: completion, Spend: fptr(spend),
		StartTime: start.UTC().Format(time.RFC3339Nano), EndTime: end.UTC().Format(time.RFC3339Nano),
		CacheHit: boolInt(cacheHit), Status: status,
	}
	_, err := s.Engine.Insert(&row)
	s.bust(new(spendRow))
	return err
}

// 列出花费日志，并带上总 token 和耗时。
func (s *Store) ListSpendLogs() ([]map[string]any, error) {
	var rows []spendRow
	if err := s.Engine.Find(&rows); err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		row := map[string]any{
			"request_id": r.RequestID, "call_type": r.CallType, "model": r.Model, "api_key": r.APIKey,
			"prompt_tokens": r.Prompt, "completion_tokens": r.Completion, "total_tokens": r.Prompt + r.Completion,
			"startTime": r.StartTime, "endTime": r.EndTime, "cache_hit": r.CacheHit != 0,
			"status": nz(r.Status, "success"), "session_total_count": 1,
		}
		if r.Spend != nil {
			row["spend"] = *r.Spend
		} else {
			row["spend"] = nil
		}
		if startAt := parseRFC(r.StartTime); !startAt.IsZero() {
			if endAt := parseRFC(r.EndTime); !endAt.IsZero() {
				row["request_duration_ms"] = endAt.Sub(startAt).Milliseconds()
			}
		}
		out = append(out, row)
	}
	return out, nil
}

// 按 id 更新代理模型，没有这一行就插入。
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

// 列出全部代理模型。
func (s *Store) ListProxyModels() ([]ProxyModel, error) {
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

// 按 id 删除代理模型，并清掉缓存。
func (s *Store) DeleteProxyModel(id string) error {
	_, err := s.Engine.ID(id).Delete(&proxyModelRow{})
	s.bust(new(proxyModelRow))
	return err
}

// 写入一条命名空间配置。已有则更新，没有则插入。
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

// 删除一条命名空间配置，并清掉缓存。
func (s *Store) DeleteConfig(namespace, key string) error {
	_, err := s.Engine.ID(coreIDs(namespace, key)).Delete(&configRow{})
	s.bust(new(configRow))
	return err
}

// 读出某个命名空间下的全部配置。
func (s *Store) ListConfig(namespace string) (map[string]any, error) {
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
