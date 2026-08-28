package history

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // 纯 Go 驱动，注册 "sqlite" 到 database/sql
)

// DefaultDBPath 是 .workflow 下的统一库路径（设计文档 §8.1），
// 相对进程 CWD（与 executions 目录同根）。
const DefaultDBPath = ".workflow/gum-workflows.db"

// Store 是打开的 SQLite 统一库句柄（设计文档 §8）。
// Open 建库并顺序迁移至最新；重复 Open 同一文件幂等不重放。
type Store struct {
	db *sql.DB
}

// Open 打开（必要时创建）dbPath 处的 SQLite 库，设 PRAGMA 并迁移到最新。
// 父目录不存在则创建。迁移幂等可重入（运行历史设计 §9.3）。
func Open(dbPath string) (*Store, error) {
	return openAt(context.Background(), dbPath)
}

func openAt(ctx context.Context, dbPath string) (*Store, error) {
	if dbPath == "" {
		dbPath = DefaultDBPath
	}
	if dir := filepath.Dir(dbPath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir %s: %w", dir, err)
		}
	}

	// 单连接串行化：单连接上设的 PRAGMA 全程有效（运行历史设计 §9.2）。
	// 跨进程并发由 WAL + busy_timeout 兜底；单进程内并行 Engine 的写也
	// 只在调度主循环 snapshot 点发生（天然单 goroutine）。
	db, err := sql.Open("sqlite", "file:"+dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	db.SetMaxOpenConns(1)

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping sqlite %s: %w", dbPath, err)
	}

	// PRAGMA（运行历史设计 §9.2）：WAL 写不阻塞读、崩溃安全；
	// busy_timeout 同项目两 CLI 进程并发时排队；foreign_keys 使 FK+CASCADE 生效。
	for _, p := range []string{
		`PRAGMA journal_mode = WAL`,
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA foreign_keys = ON`,
	} {
		if _, err := db.ExecContext(ctx, p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("set pragma %q: %w", p, err)
		}
	}

	if err := migrate(ctx, db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close 关闭库句柄。
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
