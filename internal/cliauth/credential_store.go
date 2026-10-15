// Package cliauth owns browser OAuth login and durable, machine-local credentials.
package cliauth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/home"
	"github.com/holistics/anfra/shared/jsonkit"
)

type storedCredential struct {
	Org                cloudapi.Org `json:"org"`
	OrgName            string       `json:"org_name,omitempty"`
	GrantID            string       `json:"grant_id"`
	OrgID              string       `json:"org_id"`
	GrantExpiry        time.Time    `json:"grant_expires_at"`
	AccessToken        string       `json:"access_token"`
	RefreshToken       string       `json:"refresh_token"`
	AccessExpiry       time.Time    `json:"access_expires_at"`
	TokenEndpoint      string       `json:"token_endpoint"`
	RevocationEndpoint string       `json:"revocation_endpoint"`
	Pending            bool         `json:"pending_refresh"`
}

type credentialInventory struct {
	Version int                         `json:"version"`
	Entries map[string]storedCredential `json:"entries"`
}

func credentialKey(org cloudapi.Org) string { return org.String() }

// withCredentialLock rereads only after obtaining the single machine-wide lock.
func withCredentialLock(ctx context.Context, use func(*credentialInventory, string) error) error {
	dir := home.Dir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return errors.New("cliauth: cannot create credential directory")
	}
	lock, err := os.OpenFile(filepath.Join(dir, "credentials.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return errors.New("cliauth: cannot open credential lock")
	}
	defer lock.Close()
	if err := acquireCredentialLock(ctx, lock); err != nil {
		return err
	}
	defer releaseCredentialLock(lock)
	path := filepath.Join(dir, "credentials.json")
	inventory := credentialInventory{Version: 1, Entries: map[string]storedCredential{}}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		info, statErr := f.Stat()
		if statErr != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
			return errors.New("cliauth: credentials.json must be a regular file with mode 0600")
		}
		raw, readErr := io.ReadAll(io.LimitReader(f, 4<<20+1))
		if readErr != nil || len(raw) > 4<<20 || jsonkit.Unmarshal(raw, &inventory) != nil || inventory.Version != 1 || inventory.Entries == nil {
			return errors.New("cliauth: invalid credentials.json; restore it or log in again")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("cliauth: cannot read credentials.json")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(&inventory, path)
}

func saveCredentialInventory(path string, inventory *credentialInventory) error {
	raw, err := jsonkit.MarshalIndent(inventory)
	if err != nil {
		return errors.New("cliauth: cannot encode credentials")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return errors.New("cliauth: cannot create credential temporary file")
	}
	temp := f.Name()
	defer os.Remove(temp)
	defer f.Close()
	if err = f.Chmod(0o600); err == nil {
		_, err = f.Write(append(raw, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(temp, path)
	}
	if err == nil {
		err = syncCredentialDirectory(path)
	}
	if err != nil {
		return fmt.Errorf("cliauth: durable credential save failed; log in again")
	}
	return nil
}
