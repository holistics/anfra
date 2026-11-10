// Package cliauth owns browser OAuth login and durable, machine-local credentials.
package cliauth

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/cloudapi"
	"github.com/holistics/anfra/internal/filelock"
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
		return clierr.Precondition("can't create anfra's folder: %v", err)
	}
	lock, err := os.OpenFile(filepath.Join(dir, "credentials.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return clierr.Precondition("can't open the authentication lock file: %v", err)
	}
	defer lock.Close()
	if err := filelock.Lock(ctx, lock); err != nil {
		if ctx.Err() != nil {
			return err
		}
		return clierr.Precondition("can't lock %s: %v. anfra's folder may be on a network drive that doesn't support file locks. If so, set ANFRA_HOME to a folder on a local disk", lock.Name(), err)
	}
	defer filelock.Unlock(lock)
	path := filepath.Join(dir, "credentials.json")
	inventory := credentialInventory{Version: 1, Entries: map[string]storedCredential{}}
	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		info, err := f.Stat()
		switch {
		case err != nil:
			return clierr.Precondition("can't read the authentication file: %v", err)
		case !info.Mode().IsRegular():
			return clierr.Precondition("can't use the authentication file (%s), because it isn't a file", path)
		case info.Mode().Perm() != 0o600:
			return clierr.Precondition("can't use the authentication file (%s), because its permissions aren't 600 (read and write for you only). To fix them, run `chmod 600 %s`", path, path)
		}
		raw, err := io.ReadAll(io.LimitReader(f, 4<<20+1))
		if err != nil {
			return clierr.Precondition("can't read the authentication file: %v", err)
		}
		if len(raw) > 4<<20 || jsonkit.Unmarshal(raw, &inventory) != nil || inventory.Version != 1 || inventory.Entries == nil {
			return invalidCredentialFile(path)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return clierr.Precondition("can't read the authentication file: %v", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return use(&inventory, path)
}

// invalidCredentialFile is the authentication file at path holding what anfra
// didn't write, such as after a hand edit.
func invalidCredentialFile(path string) error {
	return clierr.Precondition("can't read the authentication file (%s), because its contents aren't valid. Delete it and run `anfra auth login` again for each organization. Deleting it doesn't revoke its logins on the server, so also revoke them at https://<server>/account/connected-apps", path)
}

func saveCredentialInventory(path string, inventory *credentialInventory) error {
	raw, err := jsonkit.MarshalIndent(inventory)
	if err != nil {
		return errors.New("cliauth: cannot encode credentials")
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".credentials-*")
	if err != nil {
		return clierr.Precondition("can't write to anfra's folder: %v", err)
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
		return clierr.Precondition("can't write the authentication file: %v", err)
	}
	return nil
}
