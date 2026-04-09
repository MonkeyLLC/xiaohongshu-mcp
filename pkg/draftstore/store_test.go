package draftstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLocalDraft(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv(draftsDirEnv, rootDir)

	sourceImagePath := writeTestImage(t, "fake-image-content")

	result, err := SaveLocalDraft(SaveInput{
		Title:               "Test title",
		Content:             "Test content",
		Tags:                []string{"tag1", "tag2"},
		SourceImages:        []string{"https://example.com/a.png"},
		ProcessedImagePaths: []string{sourceImagePath},
		IsOriginal:          true,
		Visibility:          "public",
		Products:            []string{"product-a"},
	})
	if err != nil {
		t.Fatalf("SaveLocalDraft() error = %v", err)
	}

	if result.DraftID == "" {
		t.Fatalf("DraftID should not be empty")
	}
	if result.DraftPath == "" {
		t.Fatalf("DraftPath should not be empty")
	}

	recordPath := filepath.Join(result.DraftPath, "draft.json")
	if _, err := os.Stat(recordPath); err != nil {
		t.Fatalf("draft.json should exist: %v", err)
	}

	recordData, err := os.ReadFile(recordPath)
	if err != nil {
		t.Fatalf("read draft.json: %v", err)
	}

	var record DraftRecord
	if err := json.Unmarshal(recordData, &record); err != nil {
		t.Fatalf("unmarshal draft.json: %v", err)
	}

	if record.Title != "Test title" {
		t.Fatalf("record.Title = %q", record.Title)
	}
	if len(record.Images) != 1 {
		t.Fatalf("expected 1 stored image, got %d", len(record.Images))
	}

	storedImageData, err := os.ReadFile(record.Images[0])
	if err != nil {
		t.Fatalf("read stored image: %v", err)
	}
	if string(storedImageData) != "fake-image-content" {
		t.Fatalf("stored image content mismatch")
	}
}

func TestListLocalDrafts(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv(draftsDirEnv, rootDir)

	firstImage := writeTestImage(t, "image-a")
	secondImage := writeTestImage(t, "image-b")

	first, err := SaveLocalDraft(SaveInput{
		Title:               "Draft A",
		Content:             "Content A",
		ProcessedImagePaths: []string{firstImage},
	})
	if err != nil {
		t.Fatalf("SaveLocalDraft(first) error = %v", err)
	}

	second, err := SaveLocalDraft(SaveInput{
		Title:               "Draft B",
		Content:             "Content B",
		ProcessedImagePaths: []string{secondImage},
	})
	if err != nil {
		t.Fatalf("SaveLocalDraft(second) error = %v", err)
	}

	listResult, err := ListLocalDrafts()
	if err != nil {
		t.Fatalf("ListLocalDrafts() error = %v", err)
	}

	if listResult.RootDir != rootDir {
		t.Fatalf("RootDir = %q, want %q", listResult.RootDir, rootDir)
	}
	if listResult.Count != 2 {
		t.Fatalf("Count = %d, want 2", listResult.Count)
	}
	if len(listResult.Drafts) != 2 {
		t.Fatalf("len(Drafts) = %d, want 2", len(listResult.Drafts))
	}

	found := map[string]bool{}
	for _, draft := range listResult.Drafts {
		found[draft.ID] = true
		if draft.DraftPath == "" {
			t.Fatalf("DraftPath should not be empty")
		}
	}

	if !found[first.DraftID] {
		t.Fatalf("first draft id %q not found", first.DraftID)
	}
	if !found[second.DraftID] {
		t.Fatalf("second draft id %q not found", second.DraftID)
	}
}

func writeTestImage(t *testing.T, content string) string {
	t.Helper()

	sourceImageDir := t.TempDir()
	sourceImagePath := filepath.Join(sourceImageDir, "source.png")
	if err := os.WriteFile(sourceImagePath, []byte(content), 0644); err != nil {
		t.Fatalf("write source image: %v", err)
	}

	return sourceImagePath
}

func TestGetLocalDraft(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv(draftsDirEnv, rootDir)

	imagePath := writeTestImage(t, "image-content")
	saved, err := SaveLocalDraft(SaveInput{
		Title:               "Draft title",
		Content:             "Draft content",
		ProcessedImagePaths: []string{imagePath},
	})
	if err != nil {
		t.Fatalf("SaveLocalDraft() error = %v", err)
	}

	result, err := GetLocalDraft(saved.DraftID)
	if err != nil {
		t.Fatalf("GetLocalDraft() error = %v", err)
	}
	if result.DraftID != saved.DraftID {
		t.Fatalf("DraftID = %q, want %q", result.DraftID, saved.DraftID)
	}
	if result.DraftPath != saved.DraftPath {
		t.Fatalf("DraftPath = %q, want %q", result.DraftPath, saved.DraftPath)
	}
	if result.Record == nil {
		t.Fatal("Record should not be nil")
	}
	if result.Record.Title != "Draft title" {
		t.Fatalf("Record.Title = %q", result.Record.Title)
	}
}

func TestGetLocalDraftNotFound(t *testing.T) {
	rootDir := t.TempDir()
	t.Setenv(draftsDirEnv, rootDir)

	if _, err := GetLocalDraft("draft_missing"); err == nil {
		t.Fatal("expected error for missing draft")
	}
}
