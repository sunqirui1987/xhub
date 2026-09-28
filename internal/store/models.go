// Package store persists gateway records in PostgreSQL. Dashboard-saved models override a YAML entry with the same name.
package store

// ProxyModel is a model the dashboard stored in the database. When the name also exists in YAML, this row wins.
type ProxyModel struct {
	ID        string
	ModelName string
	Params    map[string]any
	Info      map[string]any
}
