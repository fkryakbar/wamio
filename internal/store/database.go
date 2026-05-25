package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
)

// Database wraps the whatsmeow SQL store for device session persistence
type Database struct {
	Container *sqlstore.Container
	sqlDB     *sql.DB
	dbPath    string
}

// NewDatabase creates a new SQLite-backed database at the given path.
// Uses the pure-Go ncruces/go-sqlite3 driver (no CGO required).
// If the directory does not exist, it will be created.
func NewDatabase(dbPath string) (*Database, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_journal_mode=WAL", dbPath)

	sqlDB, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	dbLog := waLog.Stdout("Database", "WARN", true)

	container := sqlstore.NewWithDB(sqlDB, "sqlite3", dbLog)

	// Run migrations to create the whatsmeow tables
	if err = container.Upgrade(context.Background()); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("failed to upgrade database schema: %w", err)
	}

	return &Database{
		Container: container,
		sqlDB:     sqlDB,
		dbPath:    dbPath,
	}, nil
}

// GetDBPath returns the path to the database file
func (d *Database) GetDBPath() string {
	return d.dbPath
}

// GetSQLDB returns the underlying sql.DB connection for shared use
func (d *Database) GetSQLDB() *sql.DB {
	return d.sqlDB
}

// Close closes the database connection
func (d *Database) Close() error {
	if d.Container != nil {
		d.Container.Close()
	}
	if d.sqlDB != nil {
		return d.sqlDB.Close()
	}
	return nil
}

// GetDefaultDBPath returns the default database path in the user's app data directory
func GetDefaultDBPath(accountID string) (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("failed to get config directory: %w", err)
	}
	return filepath.Join(configDir, "Wamio", "accounts", accountID, "session.db"), nil
}
