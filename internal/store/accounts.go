package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type lastAccount struct {
	AccountID string `json:"accountId"`
}

func lastAccountPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(configDir, "Wamio", "last_account.json"), nil
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
