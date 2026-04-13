package draftstore

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

const (
	draftTableName      = "xhs_drafts"
	draftImageTableName = "xhs_draft_images"
)

type MySQLStore struct {
	cfg         MySQLConfig
	mu          sync.Mutex
	db          *sql.DB
	schemaReady bool
}

type draftImageBlob struct {
	Index    int
	Filename string
	Data     []byte
}

func NewMySQLStore(cfg MySQLConfig) Store {
	return &MySQLStore{cfg: cfg}
}

func (c MySQLConfig) FormatDSN() (string, error) {
	if dsn := strings.TrimSpace(c.DSN); dsn != "" {
		return dsn, nil
	}

	if err := c.Validate(); err != nil {
		return "", err
	}

	cfg := mysqlDriver.NewConfig()
	cfg.User = c.User
	cfg.Passwd = c.Password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(c.Host, c.Port)
	cfg.DBName = c.Database
	cfg.ParseTime = true
	cfg.Loc = time.Local
	cfg.Params = map[string]string{
		"charset": "utf8mb4",
	}

	return cfg.FormatDSN(), nil
}

func (s *MySQLStore) EnsureReady(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.db == nil {
		dsn, err := s.cfg.FormatDSN()
		if err != nil {
			return err
		}

		db, err := sql.Open("mysql", dsn)
		if err != nil {
			return fmt.Errorf("init mysql connection failed: %w", err)
		}

		db.SetConnMaxLifetime(time.Hour)
		db.SetMaxIdleConns(5)
		db.SetMaxOpenConns(10)
		s.db = db
	}

	if err := s.db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping mysql failed: %w", err)
	}

	if s.schemaReady {
		return nil
	}

	if err := s.ensureSchema(ctx); err != nil {
		return err
	}

	s.schemaReady = true
	return nil
}

func (s *MySQLStore) ensureSchema(ctx context.Context) error {
	statements := []string{
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	id VARCHAR(64) NOT NULL PRIMARY KEY,
	title VARCHAR(255) NOT NULL,
	content LONGTEXT NOT NULL,
	tags_json LONGTEXT NULL,
	images_json LONGTEXT NOT NULL,
	source_images_json LONGTEXT NULL,
	schedule_at VARCHAR(64) NULL,
	is_original TINYINT(1) NOT NULL DEFAULT 0,
	visibility VARCHAR(64) NULL,
	products_json LONGTEXT NULL,
	created_at VARCHAR(64) NOT NULL,
	updated_at VARCHAR(64) NOT NULL,
	source VARCHAR(64) NOT NULL,
	storage_version INT NOT NULL,
	created_ts TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_ts TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, draftTableName),
		fmt.Sprintf(`
CREATE TABLE IF NOT EXISTS %s (
	draft_id VARCHAR(64) NOT NULL,
	image_index INT NOT NULL,
	filename VARCHAR(255) NOT NULL,
	image_data LONGBLOB NOT NULL,
	PRIMARY KEY (draft_id, image_index),
	KEY idx_draft_id (draft_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, draftImageTableName),
	}

	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialize draft schema failed: %w", err)
		}
	}

	return nil
}

func (s *MySQLStore) Save(ctx context.Context, input SaveInput) (_ *SaveResult, err error) {
	if err := s.EnsureReady(ctx); err != nil {
		return nil, err
	}

	draftID, err := generateDraftID()
	if err != nil {
		return nil, err
	}

	imageBlobs, err := readImageBlobs(input.ProcessedImagePaths)
	if err != nil {
		return nil, err
	}

	now := time.Now().Format(time.RFC3339)
	record := &DraftRecord{
		ID:             draftID,
		Title:          input.Title,
		Content:        input.Content,
		Tags:           cloneStrings(input.Tags),
		Images:         buildStoredImageRefs(draftID, imageBlobs),
		SourceImages:   cloneStrings(input.SourceImages),
		ScheduleAt:     input.ScheduleAt,
		IsOriginal:     input.IsOriginal,
		Visibility:     input.Visibility,
		Products:       cloneStrings(input.Products),
		CreatedAt:      now,
		UpdatedAt:      now,
		Source:         draftSource,
		StorageVersion: storageVersionV2,
	}

	row, err := newMySQLDraftRow(record)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin draft transaction failed: %w", err)
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	insertDraftSQL := fmt.Sprintf(`
INSERT INTO %s (
	id, title, content, tags_json, images_json, source_images_json,
	schedule_at, is_original, visibility, products_json,
	created_at, updated_at, source, storage_version
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, draftTableName)

	if _, err = tx.ExecContext(
		ctx,
		insertDraftSQL,
		row.ID,
		row.Title,
		row.Content,
		row.TagsJSON,
		row.ImagesJSON,
		row.SourceImagesJSON,
		row.ScheduleAt,
		row.IsOriginal,
		row.Visibility,
		row.ProductsJSON,
		row.CreatedAt,
		row.UpdatedAt,
		row.Source,
		row.StorageVersion,
	); err != nil {
		return nil, fmt.Errorf("insert draft record failed: %w", err)
	}

	insertImageSQL := fmt.Sprintf(`
INSERT INTO %s (draft_id, image_index, filename, image_data)
VALUES (?, ?, ?, ?)`, draftImageTableName)

	for _, image := range imageBlobs {
		if _, err = tx.ExecContext(ctx, insertImageSQL, draftID, image.Index, image.Filename, image.Data); err != nil {
			return nil, fmt.Errorf("insert draft image failed %s: %w", image.Filename, err)
		}
	}

	if err = tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit draft transaction failed: %w", err)
	}

	return &SaveResult{
		DraftID:   draftID,
		DraftPath: buildDraftLocator(draftID),
		Record:    record,
	}, nil
}

func (s *MySQLStore) Get(ctx context.Context, draftID string) (*GetResult, error) {
	draftID = strings.TrimSpace(draftID)
	if draftID == "" {
		return nil, fmt.Errorf("draft id is required")
	}

	if err := s.EnsureReady(ctx); err != nil {
		return nil, err
	}

	row := s.db.QueryRowContext(ctx, fmt.Sprintf(`
SELECT
	id, title, content, tags_json, images_json, source_images_json,
	schedule_at, is_original, visibility, products_json,
	created_at, updated_at, source, storage_version
FROM %s
WHERE id = ?`, draftTableName), draftID)

	record, err := scanDraftRecord(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("draft not found: %s", draftID)
		}
		return nil, err
	}

	imageBlobs, err := s.loadImageBlobs(ctx, draftID)
	if err != nil {
		return nil, err
	}

	imagePaths, err := materializeDraftImages(draftID, imageBlobs)
	if err != nil {
		return nil, err
	}
	record.Images = imagePaths

	return &GetResult{
		RootDir:   draftRootLocator,
		DraftID:   record.ID,
		DraftPath: buildDraftLocator(record.ID),
		Record:    record,
	}, nil
}

func (s *MySQLStore) List(ctx context.Context) (*ListResult, error) {
	if err := s.EnsureReady(ctx); err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT
	id, title, content, tags_json, images_json, source_images_json,
	schedule_at, is_original, visibility, products_json,
	created_at, updated_at, source, storage_version
FROM %s
ORDER BY updated_ts DESC, created_ts DESC, id DESC`, draftTableName))
	if err != nil {
		return nil, fmt.Errorf("query draft list failed: %w", err)
	}
	defer rows.Close()

	drafts := make([]ListedDraft, 0)
	for rows.Next() {
		record, err := scanDraftRecord(rows)
		if err != nil {
			return nil, err
		}

		drafts = append(drafts, ListedDraft{
			DraftRecord: *record,
			DraftPath:   buildDraftLocator(record.ID),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate draft list failed: %w", err)
	}

	return &ListResult{
		RootDir: draftRootLocator,
		Count:   len(drafts),
		Drafts:  drafts,
	}, nil
}

func scanDraftRecord(scanner interface {
	Scan(dest ...any) error
}) (*DraftRecord, error) {
	var row mysqlDraftRow
	var tagsJSON sql.NullString
	var imagesJSON sql.NullString
	var sourceImagesJSON sql.NullString
	var scheduleAt sql.NullString
	var visibility sql.NullString
	var productsJSON sql.NullString
	var source sql.NullString

	if err := scanner.Scan(
		&row.ID,
		&row.Title,
		&row.Content,
		&tagsJSON,
		&imagesJSON,
		&sourceImagesJSON,
		&scheduleAt,
		&row.IsOriginal,
		&visibility,
		&productsJSON,
		&row.CreatedAt,
		&row.UpdatedAt,
		&source,
		&row.StorageVersion,
	); err != nil {
		return nil, fmt.Errorf("scan draft record failed: %w", err)
	}

	if tagsJSON.Valid {
		row.TagsJSON = tagsJSON.String
	}
	if imagesJSON.Valid {
		row.ImagesJSON = imagesJSON.String
	}
	if sourceImagesJSON.Valid {
		row.SourceImagesJSON = sourceImagesJSON.String
	}
	if scheduleAt.Valid {
		row.ScheduleAt = scheduleAt.String
	}
	if visibility.Valid {
		row.Visibility = visibility.String
	}
	if productsJSON.Valid {
		row.ProductsJSON = productsJSON.String
	}
	if source.Valid {
		row.Source = source.String
	}

	return row.toRecord()
}

func readImageBlobs(sourcePaths []string) ([]draftImageBlob, error) {
	if len(sourcePaths) == 0 {
		return nil, fmt.Errorf("draft images cannot be empty")
	}

	result := make([]draftImageBlob, 0, len(sourcePaths))
	for index, sourcePath := range sourcePaths {
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("read draft image failed %s: %w", sourcePath, err)
		}

		result = append(result, draftImageBlob{
			Index:    index + 1,
			Filename: buildStoredImageFilename(index, sourcePath),
			Data:     data,
		})
	}

	return result, nil
}

func (s *MySQLStore) loadImageBlobs(ctx context.Context, draftID string) ([]draftImageBlob, error) {
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`
SELECT image_index, filename, image_data
FROM %s
WHERE draft_id = ?
ORDER BY image_index ASC`, draftImageTableName), draftID)
	if err != nil {
		return nil, fmt.Errorf("query draft images failed: %w", err)
	}
	defer rows.Close()

	images := make([]draftImageBlob, 0)
	for rows.Next() {
		var image draftImageBlob
		if err := rows.Scan(&image.Index, &image.Filename, &image.Data); err != nil {
			return nil, fmt.Errorf("scan draft image failed: %w", err)
		}
		images = append(images, image)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate draft images failed: %w", err)
	}

	return images, nil
}

func buildStoredImageFilename(index int, sourcePath string) string {
	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".bin"
	}

	return fmt.Sprintf("%02d%s", index+1, ext)
}

func buildStoredImageRefs(draftID string, images []draftImageBlob) []string {
	result := make([]string, 0, len(images))
	for _, image := range images {
		result = append(result, buildDraftImageLocator(draftID, image.Filename))
	}
	return result
}

func materializeDraftImages(draftID string, images []draftImageBlob) ([]string, error) {
	if len(images) == 0 {
		return nil, nil
	}

	dir, err := os.MkdirTemp("", fmt.Sprintf("xhs-draft-%s-", sanitizePathFragment(draftID)))
	if err != nil {
		return nil, fmt.Errorf("create draft temp dir failed: %w", err)
	}

	result := make([]string, 0, len(images))
	for _, image := range images {
		targetPath := filepath.Join(dir, image.Filename)
		if err := os.WriteFile(targetPath, image.Data, 0644); err != nil {
			return nil, fmt.Errorf("write draft temp image failed %s: %w", image.Filename, err)
		}
		result = append(result, targetPath)
	}

	return result, nil
}

func sanitizePathFragment(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_", "*", "_", "?", "_", "\"", "_", "<", "_", ">", "_", "|", "_")
	value = strings.TrimSpace(replacer.Replace(value))
	if value == "" {
		return "draft"
	}
	return value
}
