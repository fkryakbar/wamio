package store

import (
	"path/filepath"
	"testing"
)

func useTemporaryAccountConfig(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	t.Setenv("APPDATA", configDir)
	t.Setenv("LOCALAPPDATA", configDir)
}

func TestAccountRegistryStoresUniqueAliasesAndRecency(t *testing.T) {
	useTemporaryAccountConfig(t)
	if err := UpsertAccount(AccountRecord{ID: "first", Label: "Kantor", LastUsedAt: 10}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertAccount(AccountRecord{ID: "second", Label: "Pribadi", LastUsedAt: 20}); err != nil {
		t.Fatal(err)
	}
	if err := UpsertAccount(AccountRecord{ID: "third", Label: "kantor"}); err == nil {
		t.Fatal("case-insensitive duplicate alias must be rejected")
	}

	accounts, err := LoadAccounts()
	if err != nil {
		t.Fatal(err)
	}
	if len(accounts) != 2 || accounts[0].ID != "second" || accounts[1].ID != "first" {
		t.Fatalf("accounts = %#v, want newest first", accounts)
	}
	if err := TouchAccount("first"); err != nil {
		t.Fatal(err)
	}
	accounts, err = LoadAccounts()
	if err != nil || accounts[0].ID != "first" {
		t.Fatalf("touch must promote first account, accounts=%#v err=%v", accounts, err)
	}
	if err := RemoveAccount("first"); err != nil {
		t.Fatal(err)
	}
	accounts, err = LoadAccounts()
	if err != nil || len(accounts) != 1 || accounts[0].ID != "second" {
		t.Fatalf("remove result = %#v, err=%v", accounts, err)
	}
}

func TestRemoveAccountSessionRejectsUnsafeID(t *testing.T) {
	useTemporaryAccountConfig(t)
	if err := RemoveAccountSession(filepath.Join("..", "outside")); err == nil {
		t.Fatal("unsafe account ID must be rejected")
	}
}
