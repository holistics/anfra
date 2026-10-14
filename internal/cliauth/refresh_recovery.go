package cliauth

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	"github.com/holistics/anfra/internal/cloudapi"
)

// A write-ahead recovery marker outlives a successor rename whose directory sync
// fails. It contains no secrets. All accesses run under the credential-file lock.
func refreshRecoveryPath(path string, org cloudapi.Org) string {
	digest := sha256.Sum256([]byte(org.String()))
	return filepath.Join(filepath.Dir(path), ".credentials-refresh-"+hex.EncodeToString(digest[:]))
}

func refreshRecoveryRequired(path string, org cloudapi.Org) bool {
	_, err := os.Lstat(refreshRecoveryPath(path, org))
	return !errors.Is(err, os.ErrNotExist)
}

func markRefreshRecovery(path string, org cloudapi.Org) error {
	marker, err := os.OpenFile(refreshRecoveryPath(path, org), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("cliauth: cannot durably mark pending refresh; log in again")
	}
	err = marker.Sync()
	closeErr := marker.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = syncCredentialDirectory(path)
	}
	if err != nil {
		return errors.New("cliauth: cannot durably mark pending refresh; log in again")
	}
	return nil
}

func clearRefreshRecovery(path string, org cloudapi.Org) error {
	err := os.Remove(refreshRecoveryPath(path, org))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err == nil {
		err = syncCredentialDirectory(path)
	}
	if err != nil {
		// A failed removal sync must not make an uncertain save look usable locally.
		_ = markRefreshRecovery(path, org)
		return errors.New("cliauth: cannot durably clear pending refresh; log in again")
	}
	return nil
}

func syncCredentialDirectory(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
