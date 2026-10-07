package appserve

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// listening counts an event stream in, starting the watcher for the first one. The repo is
// watched only while at least one stream is open (a Data App open in a browser): the last one to
// close stops the watcher, and Close, on shutdown, closes them all. A server only agents and CLI
// calls use watches nothing.
func (s *Server) listening() {
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	s.live.streams++
	if s.live.streams > 1 {
		return
	}
	ctx, stop := context.WithCancel(context.Background())
	if err := s.watch(ctx); err != nil {
		stop()
		if s.opts.Logger != nil {
			s.opts.Logger.Warn("appserve.watch", "error", err.Error())
		}
		return
	}
	s.live.stop = stop
}

// unlistening counts an event stream out, stopping the watcher after the last.
func (s *Server) unlistening() {
	s.live.mu.Lock()
	defer s.live.mu.Unlock()
	s.live.streams--
	if s.live.streams == 0 && s.live.stop != nil {
		s.live.stop()
		s.live.stop = nil
	}
}

// watch publishes, debounced, what changed in the repo: paths under apps/, and whether any AML
// did. fsnotify is not recursive, so every directory is watched, ones created later included;
// dot-directories and node_modules are not. It stops when ctx ends.
func (s *Server) watch(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	root, err := filepath.Abs(s.opts.RepoDir)
	if err != nil {
		_ = w.Close()
		return err
	}
	addTree := func(dir string) {
		_ = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err != nil || !d.IsDir() {
				return nil
			}
			if p != root && skipDir(d.Name()) {
				return filepath.SkipDir
			}
			_ = w.Add(p)
			return nil
		})
	}
	addTree(root)

	d := &debouncer{delay: 150 * time.Millisecond, publish: s.events.publish}
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
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
					d.add(app, aml)
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				if s.opts.Logger != nil {
					s.opts.Logger.Warn("appserve.watch", "error", err.Error())
				}
			}
		}
	}()
	return nil
}

func skipDir(name string) bool { return strings.HasPrefix(name, ".") || name == "node_modules" }

// classify says what a changed path under the repo is to live reload: a path under apps/, the
// AML, or nothing.
func classify(rel string) (app string, aml bool, ok bool) {
	rel = filepath.ToSlash(rel)
	for _, part := range strings.Split(rel, "/") {
		if skipDir(part) {
			return "", false, false
		}
	}
	if rel == "apps" || strings.HasPrefix(rel, "apps/") {
		return strings.TrimPrefix(strings.TrimPrefix(rel, "apps"), "/"), false, true
	}
	if strings.HasSuffix(strings.ToLower(rel), ".aml") {
		return "", true, true
	}
	return "", false, false
}

// debouncer folds a burst of changes into one event of each type.
type debouncer struct {
	mu      sync.Mutex
	timer   *time.Timer
	apps    []string
	aml     bool
	delay   time.Duration
	publish func(Event)
}

func (d *debouncer) add(app string, aml bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if aml {
		d.aml = true
	} else if !slices.Contains(d.apps, app) {
		d.apps = append(d.apps, app)
	}
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.delay, d.flush)
}

func (d *debouncer) flush() {
	d.mu.Lock()
	apps, aml := d.apps, d.aml
	d.apps, d.aml = nil, false
	d.mu.Unlock()
	if len(apps) > 0 {
		slices.Sort(apps)
		d.publish(Event{Type: "apps", Paths: apps})
	}
	if aml {
		d.publish(Event{Type: "aml"})
	}
}
