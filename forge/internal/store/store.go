// Package store provides persistent storage for the forge server using SQLite.
// It stores members, apps, invite codes, tokens, enhancements, and messages.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection with forge-specific operations.
type DB struct {
	conn *sql.DB
	path string
}

// Open initializes the database in the given data directory.
func Open(dataDir string) (*DB, error) {
	if err := os.MkdirAll(dataDir, 0750); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	dbPath := filepath.Join(dataDir, "forge.db")
	conn, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	db := &DB{conn: conn, path: dbPath}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return db, nil
}

// Close shuts down the database connection.
func (db *DB) Close() error {
	return db.conn.Close()
}

// Conn returns the underlying *sql.DB for direct queries.
func (db *DB) Conn() *sql.DB {
	return db.conn
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS members (
			id TEXT PRIMARY KEY,
			display_name TEXT NOT NULL,
			email TEXT UNIQUE NOT NULL,
			invited_by TEXT,
			invite_code TEXT,
			joined_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			is_operator BOOLEAN DEFAULT FALSE,
			password_hash TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS member_apps (
			id TEXT PRIMARY KEY,
			member_id TEXT NOT NULL REFERENCES members(id),
			name TEXT NOT NULL,
			description TEXT,
			client_id TEXT UNIQUE NOT NULL,
			secret_hash TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			scopes TEXT,
			mesh_enabled BOOLEAN DEFAULT TRUE,
			config TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS invite_codes (
			code TEXT PRIMARY KEY,
			created_by TEXT NOT NULL REFERENCES members(id),
			used_by TEXT,
			expires_at DATETIME NOT NULL,
			used BOOLEAN DEFAULT FALSE
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_tokens (
			access_token TEXT PRIMARY KEY,
			refresh_token TEXT UNIQUE,
			token_type TEXT DEFAULT 'Bearer',
			expires_at DATETIME NOT NULL,
			scopes TEXT,
			member_id TEXT NOT NULL REFERENCES members(id),
			app_id TEXT NOT NULL REFERENCES member_apps(id)
		)`,
		`CREATE TABLE IF NOT EXISTS auth_codes (
			code TEXT PRIMARY KEY,
			client_id TEXT NOT NULL,
			member_id TEXT NOT NULL,
			redirect_uri TEXT NOT NULL,
			scopes TEXT,
			expires_at DATETIME NOT NULL,
			used BOOLEAN DEFAULT FALSE
		)`,
		`CREATE TABLE IF NOT EXISTS enhancements (
			id TEXT PRIMARY KEY,
			from_app_id TEXT NOT NULL,
			to_app_id TEXT NOT NULL,
			title TEXT NOT NULL,
			description TEXT,
			priority TEXT DEFAULT 'medium',
			status TEXT DEFAULT 'proposed',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS enhancement_votes (
			enhancement_id TEXT NOT NULL REFERENCES enhancements(id),
			member_id TEXT NOT NULL,
			PRIMARY KEY (enhancement_id, member_id)
		)`,
		`CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			from_agent TEXT,
			to_agent TEXT,
			channel TEXT NOT NULL,
			body TEXT,
			metadata TEXT,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			reply_to TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS repos (
			id TEXT PRIMARY KEY,
			name TEXT UNIQUE NOT NULL,
			path TEXT NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("exec migration: %w", err)
		}
	}
	return nil
}
