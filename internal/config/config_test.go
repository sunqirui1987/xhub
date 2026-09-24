package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadLeavesModelListEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte("model_list: []\nrouter_settings:\n  routing_strategy: simple-shuffle\n  routing_groups:\n    - group_name: yaml-group\n      models: [gpt-4o]\n      routing_strategy: simple-shuffle\ngeneral_settings:\n  master_key: sk-test\n  database_url: postgres://xhub:xhub@127.0.0.1:5433/xhub?sslmode=disable\n  store_model_in_db: true\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ModelList) != 0 {
		t.Fatalf("loaded %d models, want none", len(cfg.ModelList))
	}
	groups, _ := cfg.RouterRaw["routing_groups"].([]any)
	if len(groups) != 1 {
		t.Fatalf("routing_groups %#v", cfg.RouterRaw["routing_groups"])
	}
}

func TestLoadRejectsSQLite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	raw := []byte("model_list: []\ngeneral_settings:\n  master_key: sk-test\n  database_url: sqlite://./xhub.db\n")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "sqlite") {
		t.Fatalf("err %v", err)
	}
}
