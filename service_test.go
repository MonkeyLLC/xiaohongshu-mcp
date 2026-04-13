package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSaveDraftWithoutMySQLConfigFailsFast(t *testing.T) {
	t.Setenv("XHS_MYSQL_CONFIG_PATH", filepath.Join(t.TempDir(), "missing.json"))
	t.Setenv("XHS_MYSQL_DSN", "")
	t.Setenv("XHS_MYSQL_HOST", "")
	t.Setenv("XHS_MYSQL_PORT", "")
	t.Setenv("XHS_MYSQL_DATABASE", "")
	t.Setenv("XHS_MYSQL_USER", "")
	t.Setenv("XHS_MYSQL_PASSWORD", "")

	service := NewXiaohongshuService()
	_, err := service.SaveDraft(context.Background(), &PublishRequest{})
	if err == nil {
		t.Fatal("SaveDraft() expected error, got nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "mysql") {
		t.Fatalf("SaveDraft() error = %v, want mysql config error", err)
	}
}
