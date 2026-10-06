package repofiles

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watch reports, debounced, which Data Apps changed and whether any AML did. fsnotify isn't
// recursive, so every directory (dot-directories aside) is watched, including ones created later.
func Watch(dataFolder string, notify func(Change)) (stop func(), err error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	root, _ := filepath.Abs(dataFolder)
	addTree := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if p != root && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			_ = w.Add(p)
			return nil
		})
	}
	addTree(root)

	d := &debouncer{apps: map[string]bool{}, delay: 150 * time.Millisecond, notify: notify}
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				rel, err := filepath.Rel(root, ev.Name)
				if err != nil {
					continue
				}
				if ev.Op&fsnotify.Create != 0 {
					if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
						addTree(ev.Name)
					}
				}
				if app, aml, ok := classify(rel); ok {
					d.add(app, !aml, aml)
				}
			case <-w.Errors:
			}
		}
	}()
	return func() {
		close(done)
		_ = w.Close()
	}, nil
}
