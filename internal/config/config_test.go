package config

import (
	"os"
	"path/filepath"
	"testing"
)

// writeConfig writes a config file into a temp directory and returns its path.
func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

const baseConfig = `model_list: []
router_settings:
  routing_strategy: simple-shuffle
general_settings:
  master_key: sk-local-master
  database_url: postgres://user:pass@127.0.0.1:5432/xhub?sslmode=disable
`

// TestLoadReadsTheConfiguredAdministrator covers the plain form: the address,
// password and name are read straight from the file.
func TestLoadReadsTheConfiguredAdministrator(t *testing.T) {
	path := writeConfig(t, `model_list: []
general_settings:
  master_key: sk-local-master
  database_url: postgres://user:pass@127.0.0.1:5432/xhub?sslmode=disable
  admin_email: admin@example.com
  admin_password: initial-password
  admin_name: Platform Admin
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	g := cfg.GeneralSettings
	if g.AdminEmail != "admin@example.com" || g.AdminPassword != "initial-password" || g.AdminName != "Platform Admin" {
		t.Fatalf("configured administrator: %q %q %q", g.AdminEmail, g.AdminPassword, g.AdminName)
	}
}

// TestLoadResolvesTheAdministratorPasswordFromTheEnvironment covers the form a
// deployment should use: the password stays out of the file and comes from the
// environment.
func TestLoadResolvesTheAdministratorPasswordFromTheEnvironment(t *testing.T) {
	t.Setenv("XHUB_ADMIN_PASSWORD", "from-environment-1234")
	path := writeConfig(t, `model_list: []
general_settings:
  master_key: sk-local-master
  database_url: postgres://user:pass@127.0.0.1:5432/xhub?sslmode=disable
  admin_email: os.environ/XHUB_ADMIN_EMAIL
  admin_password: os.environ/XHUB_ADMIN_PASSWORD
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.GeneralSettings.AdminPassword != "from-environment-1234" {
		t.Fatalf("password: %q", cfg.GeneralSettings.AdminPassword)
	}
	// An unset variable resolves to the empty string. That is the safe answer
	// for this key in particular: seeding then does not run at all, rather than
	// creating an administrator whose password is the placeholder text.
	if cfg.GeneralSettings.AdminEmail != "" {
		t.Fatalf("unset variable: %q", cfg.GeneralSettings.AdminEmail)
	}
}

// TestLoadDoesNotRequireAMasterKey pins that the console administrator is the
// admin_email account. A file with no master_key still starts.
func TestLoadDoesNotRequireAMasterKey(t *testing.T) {
	path := writeConfig(t, `model_list: []
general_settings:
  database_url: postgres://user:pass@127.0.0.1:5432/xhub?sslmode=disable
  admin_email: admin@example.com
  admin_password: initial-password
  admin_name: Admin
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.GeneralSettings.MasterKey != "" {
		t.Fatalf("master key: %q", cfg.GeneralSettings.MasterKey)
	}
	if cfg.GeneralSettings.AdminEmail != "admin@example.com" || cfg.GeneralSettings.AdminPassword != "initial-password" {
		t.Fatalf("administrator: %q %q", cfg.GeneralSettings.AdminEmail, cfg.GeneralSettings.AdminPassword)
	}
}

// TestLoadWithoutAnAdministratorStillSucceeds pins that seeding is optional: a
// deployment that configures no administrator must still start, and falls back
// to POST /bootstrap.
func TestLoadWithoutAnAdministratorStillSucceeds(t *testing.T) {
	cfg, err := Load(writeConfig(t, baseConfig))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.GeneralSettings.AdminEmail != "" || cfg.GeneralSettings.AdminPassword != "" {
		t.Fatal("an unconfigured administrator must stay empty")
	}
	if cfg.GeneralSettings.DisableEnvCredentialLogin {
		t.Fatal("disable_env_credential_login must default to false")
	}
}

// TestLoadReadsDisableEnvCredentialLogin pins the switch an operator uses to
// refuse a config-supplied account once real accounts exist.
func TestLoadReadsDisableEnvCredentialLogin(t *testing.T) {
	path := writeConfig(t, `model_list: []
general_settings:
  master_key: sk-local-master
  database_url: postgres://user:pass@127.0.0.1:5432/xhub?sslmode=disable
  admin_email: admin@example.com
  admin_password: initial-password
  disable_env_credential_login: true
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !cfg.GeneralSettings.DisableEnvCredentialLogin {
		t.Fatal("disable_env_credential_login was not read")
	}
}
