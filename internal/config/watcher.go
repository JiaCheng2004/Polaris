package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

type Watcher struct {
	files       map[string]struct{}
	dirs        map[string]struct{}
	fileWatches map[string]struct{}
	root        string
	debounce    time.Duration
	watcher     *fsnotify.Watcher
	onReload    func(string)
	onError     func(error)
	closeOnce   sync.Once
}

func NewWatcher(path string, debounce time.Duration, onReload func(string), onError func(error)) (*Watcher, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
	}
	files, err := ConfigFiles(absolutePath)
	if err != nil {
		return nil, err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create fsnotify watcher: %w", err)
	}

	if debounce <= 0 {
		debounce = 250 * time.Millisecond
	}
	if onReload == nil {
		onReload = func(string) {}
	}
	if onError == nil {
		onError = func(error) {}
	}

	watchedDirs := map[string]struct{}{}
	fileWatches := map[string]struct{}{}
	watchedFiles := make(map[string]struct{}, len(files))
	for _, file := range files {
		cleanFile := filepath.Clean(file)
		watchedFiles[cleanFile] = struct{}{}
		dir := filepath.Dir(cleanFile)
		if _, exists := watchedDirs[dir]; !exists {
			if err := watcher.Add(dir); err != nil {
				_ = watcher.Close()
				return nil, fmt.Errorf("watch config directory %s: %w", dir, err)
			}
			watchedDirs[dir] = struct{}{}
		}
		if err := watcher.Add(cleanFile); err != nil {
			_ = watcher.Close()
			return nil, fmt.Errorf("watch config file %s: %w", cleanFile, err)
		}
		fileWatches[cleanFile] = struct{}{}
	}

	return &Watcher{
		files:       watchedFiles,
		dirs:        watchedDirs,
		fileWatches: fileWatches,
		root:        absolutePath,
		debounce:    debounce,
		watcher:     watcher,
		onReload:    onReload,
		onError:     onError,
		closeOnce:   sync.Once{},
	}, nil
}

func (w *Watcher) Run(ctx context.Context, signals <-chan os.Signal) {
	var (
		timer   *time.Timer
		timerCh <-chan time.Time
		pending string
	)

	defer func() {
		_ = w.Close()
	}()

	schedule := func(trigger string) {
		pending = trigger
		if timer == nil {
			timer = time.NewTimer(w.debounce)
			timerCh = timer.C
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(w.debounce)
		timerCh = timer.C
	}

	flush := func() {
		if pending == "" {
			return
		}
		trigger := pending
		pending = ""
		if err := w.refreshFiles(); err != nil {
			w.onError(err)
		}
		w.onReload(trigger)
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return
		case sig, ok := <-signals:
			if !ok {
				signals = nil
				continue
			}
			if sig != nil {
				if err := w.refreshFiles(); err != nil {
					w.onError(err)
				}
				w.onReload("signal:" + sig.String())
			}
		case <-timerCh:
			timerCh = nil
			flush()
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
				delete(w.fileWatches, filepath.Clean(event.Name))
			}
			if w.isRelevant(event) {
				schedule("fsnotify:" + event.Op.String())
				continue
			}
			if w.shouldRefreshForUnknownEvent(event) {
				if err := w.refreshFiles(); err != nil {
					w.onError(err)
					continue
				}
				if w.isRelevant(event) {
					schedule("fsnotify:" + event.Op.String())
				}
			}
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.onError(err)
		}
	}
}

func (w *Watcher) Close() error {
	if w == nil {
		return nil
	}

	var closeErr error
	w.closeOnce.Do(func() {
		closeErr = w.watcher.Close()
	})
	return closeErr
}

func (w *Watcher) isRelevant(event fsnotify.Event) bool {
	if event.Name == "" || !isConfigWatchOp(event.Op) {
		return false
	}

	cleanName := filepath.Clean(event.Name)
	if _, exists := w.files[cleanName]; !exists {
		if _, dirExists := w.dirs[cleanName]; !dirExists {
			return false
		}
	}
	return true
}

func (w *Watcher) shouldRefreshForUnknownEvent(event fsnotify.Event) bool {
	if event.Name == "" || !isConfigWatchOp(event.Op) {
		return false
	}
	_, exists := w.dirs[filepath.Dir(filepath.Clean(event.Name))]
	return exists
}

func isConfigWatchOp(op fsnotify.Op) bool {
	return op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Remove|fsnotify.Chmod) != 0
}

func (w *Watcher) refreshFiles() error {
	files, err := ConfigFiles(w.root)
	if err != nil {
		return err
	}

	nextFiles := make(map[string]struct{}, len(files))
	for _, file := range files {
		cleanFile := filepath.Clean(file)
		nextFiles[cleanFile] = struct{}{}

		dir := filepath.Dir(cleanFile)
		if _, exists := w.dirs[dir]; !exists {
			if err := w.watcher.Add(dir); err != nil {
				return fmt.Errorf("watch config directory %s: %w", dir, err)
			}
			w.dirs[dir] = struct{}{}
		}
		if _, exists := w.fileWatches[cleanFile]; !exists {
			if err := w.watcher.Add(cleanFile); err != nil {
				return fmt.Errorf("watch config file %s: %w", cleanFile, err)
			}
			w.fileWatches[cleanFile] = struct{}{}
		}
	}

	w.files = nextFiles
	return nil
}
