package draftstore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMySQLConfigFromEnvUsesDefaultPort(t *testing.T) {
	t.Setenv("XHS_MYSQL_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(mysqlDSNEnv, "")
	t.Setenv(mysqlHostEnv, "127.0.0.1")
	t.Setenv(mysqlPortEnv, "")
	t.Setenv(mysqlDatabaseEnv, "xhs_mcp")
	t.Setenv(mysqlUserEnv, "root")
	t.Setenv(mysqlPasswordEnv, "secret")

	cfg, err := LoadMySQLConfig()
	if err != nil {
		t.Fatalf("LoadMySQLConfig() error = %v", err)
	}
	if cfg.Port != defaultMySQLPort {
		t.Fatalf("Port = %q, want %q", cfg.Port, defaultMySQLPort)
	}
}

func TestMySQLConfigValidateWithoutConfig(t *testing.T) {
	cfg := MySQLConfig{}

	err := cfg.Validate()
	if !errors.Is(err, ErrMySQLNotConfigured) {
		t.Fatalf("Validate() error = %v, want %v", err, ErrMySQLNotConfigured)
	}
}

func TestMySQLConfigValidateWithIncompleteConfig(t *testing.T) {
	cfg := MySQLConfig{
		Host:     "127.0.0.1",
		Database: "xhs_mcp",
		User:     "root",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() expected error, got nil")
	}
	if !strings.Contains(err.Error(), mysqlPasswordEnv) {
		t.Fatalf("Validate() error = %v, want mention %s", err, mysqlPasswordEnv)
	}
}

func TestNewStoreFromEnvWithoutConfigReturnsDisabledStore(t *testing.T) {
	t.Setenv("XHS_MYSQL_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv(mysqlDSNEnv, "")
	t.Setenv(mysqlHostEnv, "")
	t.Setenv(mysqlPortEnv, "")
	t.Setenv(mysqlDatabaseEnv, "")
	t.Setenv(mysqlUserEnv, "")
	t.Setenv(mysqlPasswordEnv, "")

	store := NewStoreFromEnv()
	if err := store.EnsureReady(context.Background()); !errors.Is(err, ErrMySQLNotConfigured) {
		t.Fatalf("EnsureReady() error = %v, want %v", err, ErrMySQLNotConfigured)
	}
}

func TestLoadMySQLConfigFromFile(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "mysql.json")
	configJSON := `{
  "host": "161.118.246.241",
  "port": "3306",
  "database": "xhs_mcp",
  "user": "root",
  "password": "secret"
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("XHS_MYSQL_CONFIG_PATH", configPath)
	t.Setenv(mysqlDSNEnv, "")
	t.Setenv(mysqlHostEnv, "")
	t.Setenv(mysqlPortEnv, "")
	t.Setenv(mysqlDatabaseEnv, "")
	t.Setenv(mysqlUserEnv, "")
	t.Setenv(mysqlPasswordEnv, "")

	cfg, err := LoadMySQLConfig()
	if err != nil {
		t.Fatalf("LoadMySQLConfig() error = %v", err)
	}

	if cfg.Host != "161.118.246.241" || cfg.Database != "xhs_mcp" || cfg.User != "root" || cfg.Password != "secret" {
		t.Fatalf("unexpected config: %+v", cfg)
	}
}

func TestLoadMySQLConfigEnvOverridesFile(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "mysql.json")
	configJSON := `{
  "host": "127.0.0.1",
  "port": "3306",
  "database": "db_from_file",
  "user": "user_from_file",
  "password": "pass_from_file"
}`
	if err := os.WriteFile(configPath, []byte(configJSON), 0644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	t.Setenv("XHS_MYSQL_CONFIG_PATH", configPath)
	t.Setenv(mysqlHostEnv, "192.168.1.10")
	t.Setenv(mysqlDatabaseEnv, "db_from_env")
	t.Setenv(mysqlUserEnv, "user_from_env")
	t.Setenv(mysqlPasswordEnv, "pass_from_env")
	t.Setenv(mysqlPortEnv, "")
	t.Setenv(mysqlDSNEnv, "")

	cfg, err := LoadMySQLConfig()
	if err != nil {
		t.Fatalf("LoadMySQLConfig() error = %v", err)
	}

	if cfg.Host != "192.168.1.10" || cfg.Database != "db_from_env" || cfg.User != "user_from_env" || cfg.Password != "pass_from_env" {
		t.Fatalf("env should override file, got %+v", cfg)
	}
}

func TestBuildStoredImageFilename(t *testing.T) {
	filename := buildStoredImageFilename(0, `C:\tmp\image.png`)
	if filename != "01.png" {
		t.Fatalf("buildStoredImageFilename() = %q, want %q", filename, "01.png")
	}
}

func TestMaterializeDraftImages(t *testing.T) {
	paths, err := materializeDraftImages("draft_1", []draftImageBlob{
		{Index: 1, Filename: "01.png", Data: []byte("image-a")},
		{Index: 2, Filename: "02.jpg", Data: []byte("image-b")},
	})
	if err != nil {
		t.Fatalf("materializeDraftImages() error = %v", err)
	}

	if len(paths) != 2 {
		t.Fatalf("len(paths) = %d, want 2", len(paths))
	}
}
