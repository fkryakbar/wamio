package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNewDatabase(t *testing.T) {
	// Use a temporary directory for test database
	tmpDir := filepath.Join(t.TempDir(), "test_db")
	dbPath := filepath.Join(tmpDir, "test_session.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Verify database was created and works
	if db.Container == nil {
		t.Fatal("Container should not be nil")
	}

	// Verify GetDBPath returns correct path
	if db.GetDBPath() != dbPath {
		t.Errorf("GetDBPath returned %s, expected %s", db.GetDBPath(), dbPath)
	}

	// Verify we can get a device (proves schema is created)
	deviceStore, err := db.Container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatalf("GetFirstDevice failed after NewDatabase: %v", err)
	}
	if deviceStore == nil {
		t.Fatal("DeviceStore should not be nil")
	}
}

func TestNewDatabase_CreatesDirectory(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "nested", "deep", "dir")
	dbPath := filepath.Join(tmpDir, "session.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed with nested dirs: %v", err)
	}
	defer db.Close()

	// Directory should be created
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Fatalf("Nested directory was not created at %s", tmpDir)
	}
}

func TestDatabase_Close(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "close_test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}

	// Close should not error
	if err := db.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Double close should not panic
	if err := db.Close(); err != nil {
		t.Fatalf("Double Close failed: %v", err)
	}
}

func TestGetDefaultDBPath(t *testing.T) {
	path, err := GetDefaultDBPath("account1")
	if err != nil {
		t.Fatalf("GetDefaultDBPath failed: %v", err)
	}

	// Should contain the account ID and session.db
	if !filepath.IsAbs(path) {
		t.Error("Path should be absolute")
	}

	base := filepath.Base(path)
	if base != "session.db" {
		t.Errorf("Expected filename session.db, got %s", base)
	}

	// Should contain account ID in path
	if dir := filepath.Dir(path); filepath.Base(dir) != "account1" {
		t.Errorf("Expected account1 in path, got %s", dir)
	}
}

func TestDatabase_GetDeviceStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "device_test.db")

	db, err := NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("NewDatabase failed: %v", err)
	}
	defer db.Close()

	// Get first device (should create a new one)
	deviceStore, err := db.Container.GetFirstDevice(context.Background())
	if err != nil {
		t.Fatalf("GetFirstDevice failed: %v", err)
	}

	if deviceStore == nil {
		t.Fatal("DeviceStore should not be nil")
	}
}
