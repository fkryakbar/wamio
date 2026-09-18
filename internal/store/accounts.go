package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type lastAccount struct {
	AccountID string `json:"accountId"`
}

// AccountRecord is the small local registry used to show and switch linked
// accounts. WhatsApp's encrypted session remains in its own per-account
// SQLite database; this file only stores display metadata.
type AccountRecord struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	JID         string `json:"jid"`
	PushName    string `json:"pushName"`
	PhoneNumber string `json:"phoneNumber"`
	Platform    string `json:"platform"`
	LastUsedAt  int64  `json:"lastUsedAt"`
}

type accountRegistry struct {
	Accounts []AccountRecord `json:"accounts"`
}

func wamioConfigDir() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "Wamio"), nil
}

func lastAccountPath() (string, error) {
	configDir, err := wamioConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "last_account.json"), nil
}

func accountRegistryPath() (string, error) {
	configDir, err := wamioConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "accounts.json"), nil
}

// LoadLastAccountID returns the most recently connected account, if any.
func LoadLastAccountID() (string, error) {
	path, err := lastAccountPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var value lastAccount
	if err := json.Unmarshal(data, &value); err != nil {
		return "", fmt.Errorf("invalid saved account: %w", err)
	}
	return value.AccountID, nil
}

func SaveLastAccountID(accountID string) error {
	if accountID == "" {
		return fmt.Errorf("account ID is empty")
	}
	path, err := lastAccountPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(lastAccount{AccountID: accountID})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func ClearLastAccountID() error {
	path, err := lastAccountPath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// LoadAccounts returns known linked accounts in most-recently-used order.
func LoadAccounts() ([]AccountRecord, error) {
	path, err := accountRegistryPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return []AccountRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	var registry accountRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return nil, fmt.Errorf("invalid saved accounts: %w", err)
	}
	accounts := make([]AccountRecord, 0, len(registry.Accounts))
	for _, account := range registry.Accounts {
		account.ID, account.Label = strings.TrimSpace(account.ID), strings.TrimSpace(account.Label)
		if account.ID != "" && account.Label != "" {
			accounts = append(accounts, account)
		}
	}
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].LastUsedAt > accounts[j].LastUsedAt })
	return accounts, nil
}

// UpsertAccount adds or refreshes a successfully linked account. Aliases are
// unique case-insensitively so the profile menu cannot become ambiguous.
func UpsertAccount(record AccountRecord) error {
	record.ID, record.Label = strings.TrimSpace(record.ID), strings.TrimSpace(record.Label)
	if record.ID == "" || record.Label == "" {
		return fmt.Errorf("account ID and label are required")
	}
	accounts, err := LoadAccounts()
	if err != nil {
		return err
	}
	for i := range accounts {
		if accounts[i].ID != record.ID && strings.EqualFold(accounts[i].Label, record.Label) {
			return fmt.Errorf("account label already exists")
		}
	}
	if record.LastUsedAt == 0 {
		record.LastUsedAt = time.Now().Unix()
	}
	updated := false
	for i := range accounts {
		if accounts[i].ID == record.ID {
			accounts[i] = record
			updated = true
			break
		}
	}
	if !updated {
		accounts = append(accounts, record)
	}
	return saveAccounts(accounts)
}

func TouchAccount(accountID string) error {
	accounts, err := LoadAccounts()
	if err != nil {
		return err
	}
	for i := range accounts {
		if accounts[i].ID == accountID {
			accounts[i].LastUsedAt = time.Now().Unix()
			return saveAccounts(accounts)
		}
	}
	return fmt.Errorf("account not found")
}

func RemoveAccount(accountID string) error {
	accounts, err := LoadAccounts()
	if err != nil {
		return err
	}
	filtered := make([]AccountRecord, 0, len(accounts))
	for _, account := range accounts {
		if account.ID != accountID {
			filtered = append(filtered, account)
		}
	}
	return saveAccounts(filtered)
}

func saveAccounts(accounts []AccountRecord) error {
	path, err := accountRegistryPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(accountRegistry{Accounts: accounts})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

// RemoveAccountSession removes only a fresh, unlinked account directory.
// It verifies the exact child directory under Wamio/accounts before deletion.
func RemoveAccountSession(accountID string) error {
	if strings.TrimSpace(accountID) == "" || filepath.Base(accountID) != accountID {
		return fmt.Errorf("invalid account ID")
	}
	dbPath, err := GetDefaultDBPath(accountID)
	if err != nil {
		return err
	}
	accountDir := filepath.Dir(dbPath)
	configDir, err := wamioConfigDir()
	if err != nil {
		return err
	}
	accountsDir := filepath.Join(configDir, "accounts")
	rel, err := filepath.Rel(accountsDir, accountDir)
	if err != nil || rel != accountID {
		return fmt.Errorf("invalid account session path")
	}
	return os.RemoveAll(accountDir)
}
