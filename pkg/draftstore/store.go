package draftstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	draftsDirEnv = "XHS_DRAFTS_DIR"
)

type SaveInput struct {
	Title               string
	Content             string
	Tags                []string
	SourceImages        []string
	ProcessedImagePaths []string
	ScheduleAt          string
	IsOriginal          bool
	Visibility          string
	Products            []string
}

type DraftRecord struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Content        string   `json:"content"`
	Tags           []string `json:"tags,omitempty"`
	Images         []string `json:"images"`
	SourceImages   []string `json:"source_images,omitempty"`
	ScheduleAt     string   `json:"schedule_at,omitempty"`
	IsOriginal     bool     `json:"is_original,omitempty"`
	Visibility     string   `json:"visibility,omitempty"`
	Products       []string `json:"products,omitempty"`
	CreatedAt      string   `json:"created_at"`
	UpdatedAt      string   `json:"updated_at"`
	Source         string   `json:"source"`
	StorageVersion int      `json:"storage_version"`
}

type SaveResult struct {
	DraftID   string
	DraftPath string
	Record    *DraftRecord
}

type GetResult struct {
	RootDir   string
	DraftID   string
	DraftPath string
	Record    *DraftRecord
}

type ListedDraft struct {
	DraftRecord
	DraftPath string `json:"draft_path"`
}

type ListResult struct {
	RootDir string        `json:"root_dir"`
	Count   int           `json:"count"`
	Drafts  []ListedDraft `json:"drafts"`
}

func SaveLocalDraft(input SaveInput) (*SaveResult, error) {
	rootDir, err := GetDraftsRootDir()
	if err != nil {
		return nil, err
	}

	draftID, err := generateDraftID()
	if err != nil {
		return nil, err
	}

	draftDir := filepath.Join(rootDir, draftID)
	imagesDir := filepath.Join(draftDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return nil, fmt.Errorf("创建草稿图片目录失败: %w", err)
	}

	storedImages, err := copyImages(imagesDir, input.ProcessedImagePaths)
	if err != nil {
		return nil, err
	}

	now := time.Now().Format(time.RFC3339)
	record := &DraftRecord{
		ID:             draftID,
		Title:          input.Title,
		Content:        input.Content,
		Tags:           cloneStrings(input.Tags),
		Images:         storedImages,
		SourceImages:   cloneStrings(input.SourceImages),
		ScheduleAt:     input.ScheduleAt,
		IsOriginal:     input.IsOriginal,
		Visibility:     input.Visibility,
		Products:       cloneStrings(input.Products),
		CreatedAt:      now,
		UpdatedAt:      now,
		Source:         "mcp.save_draft",
		StorageVersion: 1,
	}

	if err := writeDraftRecord(draftDir, record); err != nil {
		return nil, err
	}

	return &SaveResult{
		DraftID:   draftID,
		DraftPath: draftDir,
		Record:    record,
	}, nil
}

func GetLocalDraft(draftID string) (*GetResult, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return nil, fmt.Errorf("草稿 ID 不能为空")
	}

	rootDir, err := GetDraftsRootDir()
	if err != nil {
		return nil, err
	}

	draftPath := filepath.Join(rootDir, draftID)
	info, err := os.Stat(draftPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("未找到本地草稿: %s", draftID)
		}
		return nil, fmt.Errorf("读取草稿目录失败: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("草稿路径不是目录: %s", draftPath)
	}

	record, err := loadDraftRecord(draftPath)
	if err != nil {
		return nil, err
	}
	if record.ID == "" {
		record.ID = draftID
	}

	return &GetResult{
		RootDir:   rootDir,
		DraftID:   record.ID,
		DraftPath: draftPath,
		Record:    record,
	}, nil
}

func ListLocalDrafts() (*ListResult, error) {

	rootDir, err := GetDraftsRootDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(rootDir)
	if err != nil {
		return nil, fmt.Errorf("读取草稿根目录失败: %w", err)
	}

	drafts := make([]ListedDraft, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		draftPath := filepath.Join(rootDir, entry.Name())
		record, err := loadDraftRecord(draftPath)
		if err != nil {
			continue
		}

		drafts = append(drafts, ListedDraft{
			DraftRecord: *record,
			DraftPath:   draftPath,
		})
	}

	sort.Slice(drafts, func(i, j int) bool {
		if drafts[i].UpdatedAt != drafts[j].UpdatedAt {
			return drafts[i].UpdatedAt > drafts[j].UpdatedAt
		}
		if drafts[i].CreatedAt != drafts[j].CreatedAt {
			return drafts[i].CreatedAt > drafts[j].CreatedAt
		}
		return drafts[i].ID > drafts[j].ID
	})

	return &ListResult{
		RootDir: rootDir,
		Count:   len(drafts),
		Drafts:  drafts,
	}, nil
}

func GetDraftsRootDir() (string, error) {
	if dir := os.Getenv(draftsDirEnv); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("创建草稿根目录失败: %w", err)
		}
		return dir, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("获取用户目录失败: %w", err)
	}

	dir := filepath.Join(homeDir, ".xiaohongshu-mcp", "drafts")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("创建草稿根目录失败: %w", err)
	}
	return dir, nil
}

func writeDraftRecord(draftDir string, record *DraftRecord) error {
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化草稿记录失败: %w", err)
	}

	targetPath := filepath.Join(draftDir, "draft.json")
	tempPath := targetPath + ".tmp"
	if err := os.WriteFile(tempPath, data, 0644); err != nil {
		return fmt.Errorf("写入草稿临时文件失败: %w", err)
	}

	if err := os.Rename(tempPath, targetPath); err != nil {
		return fmt.Errorf("落盘草稿文件失败: %w", err)
	}

	return nil
}

func loadDraftRecord(draftDir string) (*DraftRecord, error) {
	recordPath := filepath.Join(draftDir, "draft.json")
	data, err := os.ReadFile(recordPath)
	if err != nil {
		return nil, fmt.Errorf("读取草稿文件失败: %w", err)
	}

	var record DraftRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("解析草稿文件失败: %w", err)
	}

	return &record, nil
}

func copyImages(imagesDir string, sourcePaths []string) ([]string, error) {
	result := make([]string, 0, len(sourcePaths))
	for i, sourcePath := range sourcePaths {
		ext := filepath.Ext(sourcePath)
		if ext == "" {
			ext = ".bin"
		}

		filename := fmt.Sprintf("%02d%s", i+1, ext)
		targetPath := filepath.Join(imagesDir, filename)
		if err := copyFile(sourcePath, targetPath); err != nil {
			return nil, fmt.Errorf("复制草稿图片失败 %s: %w", sourcePath, err)
		}
		result = append(result, targetPath)
	}
	return result, nil
}

func copyFile(sourcePath, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	targetFile, err := os.Create(targetPath)
	if err != nil {
		return err
	}

	_, copyErr := io.Copy(targetFile, sourceFile)
	closeErr := targetFile.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}

	return nil
}

func generateDraftID() (string, error) {
	randomPart := make([]byte, 4)
	if _, err := rand.Read(randomPart); err != nil {
		return "", fmt.Errorf("生成草稿 ID 失败: %w", err)
	}

	return fmt.Sprintf("draft_%s_%s", time.Now().Format("20060102_150405"), hex.EncodeToString(randomPart)), nil
}

func cloneStrings(src []string) []string {
	if len(src) == 0 {
		return nil
	}

	dst := make([]string, len(src))
	copy(dst, src)
	return dst
}
