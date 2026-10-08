package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/louislw/AIDataAnalysis/internal/auth"
	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	DB *sql.DB
}

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	DisplayName  string `json:"displayName"`
	Dept         string `json:"dept"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"createdAt"`
}

func Open(dataRoot string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(dataRoot, "sources"), 0o755); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(filepath.Join(dataRoot, "app.db"))
	if err != nil {
		return nil, err
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     abs,
		RawQuery: "_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000",
	}).String()
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{DB: db}, nil
}

func migrate(db *sql.DB) error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("迁移失败: %w", err)
		}
	}
	return nil
}

func (s *Store) Close() error { return s.DB.Close() }

func Now() string { return time.Now().UTC().Format(time.RFC3339) }

func (s *Store) Bootstrap(adminPassword, userPassword string) error {
	var n int
	if err := s.DB.QueryRow(`SELECT COUNT(*) FROM sys_user`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	if adminPassword == "" || userPassword == "" {
		return fmt.Errorf("首次启动需要 BOOTSTRAP_ADMIN_PASSWORD 和 BOOTSTRAP_USER_PASSWORD")
	}
	if _, err := s.CreateUser(context.Background(), "admin", adminPassword, "ADMIN", "管理员", ""); err != nil {
		return err
	}
	_, err := s.CreateUser(context.Background(), "user", userPassword, "USER", "用户", "")
	return err
}

func (s *Store) CreateUser(ctx context.Context, username, password, role, displayName, dept string) (User, error) {
	hash, err := auth.HashPassword(password)
	if err != nil {
		return User{}, err
	}
	if displayName == "" {
		displayName = username
	}
	res, err := s.DB.ExecContext(ctx, `INSERT INTO sys_user (username, password_hash, role, display_name, dept, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, 1, ?)`, username, hash, role, displayName, dept, Now())
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, id)
}

func (s *Store) UserByUsername(ctx context.Context, username string) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, userSelect+` WHERE username = ?`, username))
}

func (s *Store) UserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(s.DB.QueryRowContext(ctx, userSelect+` WHERE id = ?`, id))
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.DB.QueryContext(ctx, userSelect+` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, role, displayName, dept string, enabled bool) (User, error) {
	en := 0
	if enabled {
		en = 1
	}
	res, err := s.DB.ExecContext(ctx, `UPDATE sys_user SET role = ?, display_name = ?, dept = ?, enabled = ? WHERE id = ?`,
		role, displayName, dept, en, id)
	if err != nil {
		return User{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return User{}, sql.ErrNoRows
	}
	return s.UserByID(ctx, id)
}

const userSelect = `SELECT id, username, password_hash, role, display_name, dept, enabled, created_at FROM sys_user`

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (User, error) {
	var u User
	var enabled int
	err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.DisplayName, &u.Dept, &enabled, &u.CreatedAt)
	u.Enabled = enabled == 1
	return u, err
}
