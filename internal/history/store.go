package history

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // 纯 Go 驱动，注册 "sqlite" 到 database/sql
)

// Store 是打开的 SQLite 统一库句柄（设计文档 §8）。
// Open 建库并顺序迁移至最新；重复 Open 同一文件幂等不重放。
type Store struct {
	db *sql.DB
}

// Open 打开（必要时创建）dbPath 处的 SQLite 库，设 PRAGMA 并迁移到最新。
// 父目录不存在则创建。迁移幂等可重入（运行历史设计 §9.3）。
func Open(ctx context.Context, dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("open history database: path must not be empty")
	}
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir %s: %w", dir, err)
		}
	}

	// 单连接串行化：单连接上设的 PRAGMA 全程有效（运行历史设计 §9.2）。
	// 跨进程并发由 WAL + busy_timeout 兜底；单进程内并行 Engine 的写也
	// 只在调度主循环 snapshot 点发生（天然单 goroutine）。
	dsn, err := sqliteDSN(dbPath, url.Values{"_txlock": {"immediate"}})
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)

	// PRAGMA（运行历史设计 §9.2）：WAL 写不阻塞读、崩溃安全；
	// busy_timeout 同项目两 CLI 进程并发时排队；foreign_keys 使 FK+CASCADE 生效。
	for _, p := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}

	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// OpenReadOnly 打开已存在的 SQLite 库，不创建目录、不迁移也不设置持久化 PRAGMA。
func OpenReadOnly(ctx context.Context, dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("open history database read-only: path must not be empty")
	}
	dsn, err := sqliteDSN(dbPath, url.Values{
		"mode":    {"ro"},
		"_pragma": {"query_only(1)"},
	})
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite read-only %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite read-only %s: %w", dbPath, err)
	}
	return &Store{db: db}, nil
}

func sqliteDSN(dbPath string, query url.Values) (string, error) {
	absPath, err := filepath.Abs(dbPath)
	if err != nil {
		return "", fmt.Errorf("resolve sqlite path %s: %w", dbPath, err)
	}
	dsn := url.URL{Scheme: "file", Path: filepath.ToSlash(absPath), RawQuery: query.Encode()}
	return dsn.String(), nil
}

// Close 关闭库句柄。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// UserVersion 返回 SQLite PRAGMA user_version，不执行迁移。
func (s *Store) UserVersion(ctx context.Context) (int, error) {
	var version int
	if err := s.db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil {
		return 0, fmt.Errorf("read user_version: %w", err)
	}
	return version, nil
}
