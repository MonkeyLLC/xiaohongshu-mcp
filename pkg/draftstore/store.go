package draftstore

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/xpzouying/xiaohongshu-mcp/configs"
)

const (
	mysqlDSNEnv      = "XHS_MYSQL_DSN"
	mysqlHostEnv     = "XHS_MYSQL_HOST"
	mysqlPortEnv     = "XHS_MYSQL_PORT"
	mysqlDatabaseEnv = "XHS_MYSQL_DATABASE"
	mysqlUserEnv     = "XHS_MYSQL_USER"
	mysqlPasswordEnv = "XHS_MYSQL_PASSWORD"
	defaultMySQLPort = "3306"
	draftRootLocator = "mysql://xhs_drafts"
	draftSource      = "mcp.save_draft"
	storageVersionV2 = 2
)

var ErrMySQLNotConfigured = stderrors.New("mysql is not configured; draft features are unavailable")

type Store interface {
	EnsureReady(ctx context.Context) error
	Save(ctx context.Context, input SaveInput) (*SaveResult, error)
	Get(ctx context.Context, draftID string) (*GetResult, error)
	List(ctx context.Context) (*ListResult, error)
}

type MySQLConfig struct {
	DSN      string
	Host     string
	Port     string
	Database string
	User     string
	Password string
}

func loadMySQLConfigFromEnv() MySQLConfig {
	port := strings.TrimSpace(os.Getenv(mysqlPortEnv))
	if port == "" {
		port = defaultMySQLPort
	}

	return MySQLConfig{
		DSN:      strings.TrimSpace(os.Getenv(mysqlDSNEnv)),
		Host:     strings.TrimSpace(os.Getenv(mysqlHostEnv)),
		Port:     port,
		Database: strings.TrimSpace(os.Getenv(mysqlDatabaseEnv)),
		User:     strings.TrimSpace(os.Getenv(mysqlUserEnv)),
		Password: os.Getenv(mysqlPasswordEnv),
	}
}

func LoadMySQLConfig() (MySQLConfig, error) {
	cfg := MySQLConfig{}

	fileCfg, err := configs.LoadMySQLConfigFile()
	if err != nil {
		return MySQLConfig{}, err
	}

	cfg.merge(MySQLConfig{
		DSN:      strings.TrimSpace(fileCfg.DSN),
		Host:     strings.TrimSpace(fileCfg.Host),
		Port:     strings.TrimSpace(fileCfg.Port),
		Database: strings.TrimSpace(fileCfg.Database),
		User:     strings.TrimSpace(fileCfg.User),
		Password: fileCfg.Password,
	})

	cfg.merge(loadMySQLConfigFromEnv())

	if strings.TrimSpace(cfg.Port) == "" {
		cfg.Port = defaultMySQLPort
	}

	return cfg, nil
}

func (c *MySQLConfig) merge(other MySQLConfig) {
	if strings.TrimSpace(other.DSN) != "" {
		c.DSN = strings.TrimSpace(other.DSN)
	}
	if strings.TrimSpace(other.Host) != "" {
		c.Host = strings.TrimSpace(other.Host)
	}
	if strings.TrimSpace(other.Port) != "" {
		c.Port = strings.TrimSpace(other.Port)
	}
	if strings.TrimSpace(other.Database) != "" {
		c.Database = strings.TrimSpace(other.Database)
	}
	if strings.TrimSpace(other.User) != "" {
		c.User = strings.TrimSpace(other.User)
	}
	if other.Password != "" {
		c.Password = other.Password
	}
}

func (c MySQLConfig) Validate() error {
	if strings.TrimSpace(c.DSN) != "" {
		return nil
	}

	required := map[string]string{
		mysqlHostEnv:     c.Host,
		mysqlDatabaseEnv: c.Database,
		mysqlUserEnv:     c.User,
		mysqlPasswordEnv: c.Password,
	}

	allEmpty := true
	missing := make([]string, 0, len(required))
	for key, value := range required {
		if strings.TrimSpace(value) == "" {
			missing = append(missing, key)
			continue
		}
		allEmpty = false
	}

	if allEmpty {
		return ErrMySQLNotConfigured
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("mysql config is incomplete, missing: %s", strings.Join(missing, ", "))
	}

	return nil
}

func NewStoreFromEnv() Store {
	cfg, err := LoadMySQLConfig()
	if err != nil {
		return &disabledStore{err: err}
	}
	if err := cfg.Validate(); err != nil {
		return &disabledStore{err: err}
	}

	return NewMySQLStore(cfg)
}

type disabledStore struct {
	err error
}

func (s *disabledStore) EnsureReady(ctx context.Context) error {
	_ = ctx
	return s.err
}

func (s *disabledStore) Save(ctx context.Context, input SaveInput) (*SaveResult, error) {
	_ = ctx
	_ = input
	return nil, s.err
}

func (s *disabledStore) Get(ctx context.Context, draftID string) (*GetResult, error) {
	_ = ctx
	_ = draftID
	return nil, s.err
}

func (s *disabledStore) List(ctx context.Context) (*ListResult, error) {
	_ = ctx
	return nil, s.err
}

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

type mysqlDraftRow struct {
	ID               string
	Title            string
	Content          string
	TagsJSON         string
	ImagesJSON       string
	SourceImagesJSON string
	ScheduleAt       string
	IsOriginal       bool
	Visibility       string
	ProductsJSON     string
	CreatedAt        string
	UpdatedAt        string
	Source           string
	StorageVersion   int
}

func newMySQLDraftRow(record *DraftRecord) (*mysqlDraftRow, error) {
	tagsJSON, err := encodeStringSlice(record.Tags)
	if err != nil {
		return nil, fmt.Errorf("encode draft tags failed: %w", err)
	}

	imagesJSON, err := encodeStringSlice(record.Images)
	if err != nil {
		return nil, fmt.Errorf("encode draft images failed: %w", err)
	}

	sourceImagesJSON, err := encodeStringSlice(record.SourceImages)
	if err != nil {
		return nil, fmt.Errorf("encode draft source images failed: %w", err)
	}

	productsJSON, err := encodeStringSlice(record.Products)
	if err != nil {
		return nil, fmt.Errorf("encode draft products failed: %w", err)
	}

	return &mysqlDraftRow{
		ID:               record.ID,
		Title:            record.Title,
		Content:          record.Content,
		TagsJSON:         tagsJSON,
		ImagesJSON:       imagesJSON,
		SourceImagesJSON: sourceImagesJSON,
		ScheduleAt:       record.ScheduleAt,
		IsOriginal:       record.IsOriginal,
		Visibility:       record.Visibility,
		ProductsJSON:     productsJSON,
		CreatedAt:        record.CreatedAt,
		UpdatedAt:        record.UpdatedAt,
		Source:           record.Source,
		StorageVersion:   record.StorageVersion,
	}, nil
}

func (r mysqlDraftRow) toRecord() (*DraftRecord, error) {
	tags, err := decodeStringSlice(r.TagsJSON)
	if err != nil {
		return nil, fmt.Errorf("decode draft tags failed: %w", err)
	}

	images, err := decodeStringSlice(r.ImagesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode draft images failed: %w", err)
	}

	sourceImages, err := decodeStringSlice(r.SourceImagesJSON)
	if err != nil {
		return nil, fmt.Errorf("decode draft source images failed: %w", err)
	}

	products, err := decodeStringSlice(r.ProductsJSON)
	if err != nil {
		return nil, fmt.Errorf("decode draft products failed: %w", err)
	}

	return &DraftRecord{
		ID:             r.ID,
		Title:          r.Title,
		Content:        r.Content,
		Tags:           tags,
		Images:         images,
		SourceImages:   sourceImages,
		ScheduleAt:     r.ScheduleAt,
		IsOriginal:     r.IsOriginal,
		Visibility:     r.Visibility,
		Products:       products,
		CreatedAt:      r.CreatedAt,
		UpdatedAt:      r.UpdatedAt,
		Source:         r.Source,
		StorageVersion: r.StorageVersion,
	}, nil
}

func buildDraftLocator(draftID string) string {
	return fmt.Sprintf("%s/%s", draftRootLocator, draftID)
}

func buildDraftImageLocator(draftID, filename string) string {
	return fmt.Sprintf("%s/images/%s", buildDraftLocator(draftID), filename)
}

func encodeStringSlice(values []string) (string, error) {
	if len(values) == 0 {
		return "[]", nil
	}

	data, err := json.Marshal(values)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

func decodeStringSlice(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, err
	}

	return values, nil
}

func generateDraftID() (string, error) {
	randomPart := make([]byte, 4)
	if _, err := rand.Read(randomPart); err != nil {
		return "", fmt.Errorf("generate draft id failed: %w", err)
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
