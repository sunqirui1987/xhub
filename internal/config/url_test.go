package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDatabaseURLRequiresPostgresScheme(t *testing.T) {
	for _, u := range []string{"mysql://host/db", "sqlite:local", "file:local", "garbage", "postgres://host/db", "postgresql://host/db"} {
		t.Run(u, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("general_settings:\n  database_url: '"+u+"'\n"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := Load(path)
			valid := u == "postgres://host/db" || u == "postgresql://host/db"
			if (err == nil) != valid {
				t.Fatalf("url=%s err=%v", u, err)
			}
		})
	}
}
