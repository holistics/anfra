package main

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/holistics/anfra/internal/clierr"
	"github.com/holistics/anfra/internal/command/sl"
	"github.com/holistics/anfra/shared/jsonkit"
)

func isModelCommand(name string) bool { return name == "sl.add" || name == "sl.update" }

// writeModelResult is CLI delivery, after a local or forwarded core operation.
// Neither the engine nor the sidecar writes the repo for an API caller.
func writeModelResult(name string, body []byte) error {
	var result sl.Result
	if err := jsonkit.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode the generated model: %w", err)
	}
	if err := writeModelFile(".", name, result); err != nil {
		return fmt.Errorf("write model %q to %q: %w", result.ModelName, result.Path, err)
	}
	return nil
}

func writeModelFile(dir, name string, result sl.Result) error {
	if !filepath.IsLocal(result.Path) || !strings.HasSuffix(result.Path, ".aml") || strings.TrimSpace(result.Content) == "" {
		return fmt.Errorf("the generated model must have a repo-relative AML path and nonempty content")
	}
	if name == "sl.update" {
		if result.Changed == nil {
			return fmt.Errorf("the model update did not report whether its content changed")
		}
		if !*result.Changed {
			return nil
		}
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	if name == "sl.add" {
		if err := root.MkdirAll(filepath.Dir(result.Path), 0o755); err != nil {
			return err
		}
		file, err := root.OpenFile(result.Path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			if os.IsExist(err) {
				return clierr.Precondition("The file already exists. Use sl update to update its model.")
			}
			return err
		}
		if err := writeAML(file, result.Content); err != nil {
			_ = root.Remove(result.Path)
			return err
		}
		return nil
	}
	info, err := root.Lstat(result.Path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return clierr.Precondition("The model's path must be a regular file, not a directory or symlink.")
	}
	return replaceModelFile(root, result.Path, result.Content, info.Mode().Perm())
}

// The temporary file is on the destination filesystem, so rename is atomic.
func replaceModelFile(root *os.Root, path, content string, mode fs.FileMode) error {
	temporary := filepath.Join(filepath.Dir(path), ".sl-"+rand.Text())
	file, err := root.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temporary) }()
	if err := file.Chmod(mode); err != nil {
		_ = file.Close()
		return err
	}
	if err := writeAML(file, content); err != nil {
		return err
	}
	return root.Rename(temporary, path)
}

func writeAML(file *os.File, content string) error {
	_, err := file.WriteString(content)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
