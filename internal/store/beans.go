// Package store declares the framework table rows: key-value documents,
// dashboard-saved proxy models, and namespaced configuration. Column types match
// the PostgreSQL tables already in use.
//
// Identity is not here. Users, organizations, teams, memberships, projects,
// access groups, keys, usage and the audit log belong to internal/iam, which
// owns their schema and their constraints.
package store

import (
	"github.com/sunqirui1987/xhub/internal/logx"
	"sync"
)

var logTraceOnceBeans sync.Once

type kvRow struct {
	Kind      string `xorm:"pk text 'kind'"`
	ID        string `xorm:"pk text 'id'"`
	Body      string `xorm:"text 'body'"`
	CreatedAt string `xorm:"text 'created_at'"`
}

// TableName returns the key-value table name.
func (kvRow) TableName() string {
	logTraceOnceBeans.Do(func() { logx.Trace("enter store.TableName") })
	return "kv"
}

type proxyModelRow struct {
	ID        string `xorm:"pk text 'id'"`
	ModelName string `xorm:"text 'model_name'"`
	Params    string `xorm:"text 'litellm_params_json'"`
	Info      string `xorm:"text 'model_info_json'"`
	UpdatedAt string `xorm:"text 'updated_at'"`
}

// TableName returns the proxy-model table name.
func (proxyModelRow) TableName() string { return "proxy_models" }

type configRow struct {
	Namespace string `xorm:"pk text 'namespace'"`
	Key       string `xorm:"pk text 'key'"`
	ValueJSON string `xorm:"text 'value_json'"`
}

// TableName returns the proxy-config table name.
func (configRow) TableName() string { return "proxy_config" }
