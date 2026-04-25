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
	path      string
	dir       string
	base      string
	debounce  time.Duration
	watcher   *fsnotify.Watcher
	onReload  func(string)
	onError   func(error)
	closeOnce sync.Once
}

func NewWatcher(path string, debounce time.Duration, onReload func(string), onError func(error)) (*Watcher, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve config path: %w", err)
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

	dir := filepath.Dir(absolutePath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return nil, fmt.Errorf("watch config directory: %w", err)
	}

	return &Watcher{
		path:      absolutePath,
		dir:       dir,
		base:      filepath.Base(absolutePath),
		debounce:  debounce,
		watcher:   watcher,
		onReload:  onReload,
		onError:   onError,
		closeOnce: sync.Once{},
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
	}

	flush := func() {
		if pending == "" {
			return
		}
		trigger := pending
		pending = ""
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
				w.onReload("signal:" + sig.String())
			}
		case <-timerCh:
			timerCh = nil
			flush()
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			if w.isRelevant(event) {
				schedule("fsnotify:" + event.Op.String())
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
	if event.Name == "" {
		return false
	}

	cleanName := filepath.Clean(event.Name)
	if cleanName == w.path {
		return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) != 0
	}
	if filepath.Dir(cleanName) != w.dir {
		return false
	}
	if filepath.Base(cleanName) != w.base {
		return false
	}
	return event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename|fsnotify.Chmod) != 0
}
