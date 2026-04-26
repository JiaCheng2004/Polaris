package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestWatcherTriggersReloadOnSignal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polaris.yaml")
	if err := os.WriteFile(path, []byte("version: 2\nruntime:\n  server:\n    host: 127.0.0.1\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	triggered := make(chan string, 1)
	watcher, err := NewWatcher(path, 25*time.Millisecond, func(trigger string) {
		triggered <- trigger
	}, func(err error) {
		t.Fatalf("unexpected watcher error: %v", err)
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	signals := make(chan os.Signal, 1)
	go watcher.Run(ctx, signals)

	signals <- syscall.SIGHUP

	select {
	case trigger := <-triggered:
		if !strings.HasPrefix(trigger, "signal:") {
			t.Fatalf("expected signal trigger, got %q", trigger)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for signal-triggered reload")
	}
}

func TestWatcherDebouncesFileChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "polaris.yaml")
	if err := os.WriteFile(path, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	triggered := make(chan string, 4)
	watcher, err := NewWatcher(path, 50*time.Millisecond, func(trigger string) {
		triggered <- trigger
	}, func(err error) {
		t.Fatalf("unexpected watcher error: %v", err)
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go watcher.Run(ctx, make(chan os.Signal))

	if err := os.WriteFile(path, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write updated config: %v", err)
	}
	if err := os.WriteFile(path, []byte("version: 2\nproviders: {}\n"), 0o600); err != nil {
		t.Fatalf("write updated config again: %v", err)
	}

	var first string
	select {
	case first = <-triggered:
		if !strings.HasPrefix(first, "fsnotify:") {
			t.Fatalf("expected fsnotify trigger, got %q", first)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for fsnotify-triggered reload")
	}

	select {
	case extra := <-triggered:
		t.Fatalf("expected debounced single reload, got extra trigger %q", extra)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestWatcherTriggersReloadOnImportedFileChange(t *testing.T) {
	dir := t.TempDir()
	importPath := filepath.Join(dir, "provider.yaml")
	if err := os.WriteFile(importPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write imported config: %v", err)
	}
	path := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(path, []byte("version: 2\nimports:\n  - ./provider.yaml\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	triggered := make(chan string, 1)
	watcher, err := NewWatcher(path, 25*time.Millisecond, func(trigger string) {
		triggered <- trigger
	}, func(err error) {
		t.Fatalf("unexpected watcher error: %v", err)
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, make(chan os.Signal))

	if err := os.WriteFile(importPath, []byte("version: 2\nproviders: {}\n"), 0o600); err != nil {
		t.Fatalf("write updated imported config: %v", err)
	}

	select {
	case trigger := <-triggered:
		if !strings.HasPrefix(trigger, "fsnotify:") {
			t.Fatalf("expected fsnotify trigger, got %q", trigger)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for imported file reload")
	}
}

func TestWatcherRefreshesNewImportsAfterReload(t *testing.T) {
	dir := t.TempDir()
	importDir := filepath.Join(dir, "providers")
	if err := os.Mkdir(importDir, 0o700); err != nil {
		t.Fatalf("create import dir: %v", err)
	}
	importPath := filepath.Join(importDir, "provider.yaml")
	if err := os.WriteFile(importPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write imported config: %v", err)
	}
	path := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(path, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	triggered := make(chan string, 8)
	watcher, err := NewWatcher(path, 25*time.Millisecond, func(trigger string) {
		triggered <- trigger
	}, func(err error) {
		t.Fatalf("unexpected watcher error: %v", err)
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, make(chan os.Signal))

	if err := os.WriteFile(path, []byte("version: 2\nimports:\n  - ./providers/provider.yaml\n"), 0o600); err != nil {
		t.Fatalf("write config with import: %v", err)
	}
	trigger := waitForWatcherTrigger(t, triggered)
	if !strings.HasPrefix(trigger, "fsnotify:") {
		t.Fatalf("expected fsnotify trigger, got %q", trigger)
	}
	drainWatcherTriggers(triggered, 150*time.Millisecond)
	if _, exists := watcher.files[filepath.Clean(importPath)]; !exists {
		t.Fatalf("expected watcher to track newly imported config %s", importPath)
	}
	if _, exists := watcher.fileWatches[filepath.Clean(importPath)]; !exists {
		t.Fatalf("expected watcher to watch newly imported config file %s", importPath)
	}

	if err := os.WriteFile(importPath, []byte("version: 2\nproviders: {}\n"), 0o600); err != nil {
		t.Fatalf("write updated imported config: %v", err)
	}
	trigger = waitForWatcherTrigger(t, triggered)
	if !strings.HasPrefix(trigger, "fsnotify:") {
		t.Fatalf("expected fsnotify trigger for new import, got %q", trigger)
	}
}

func TestWatcherStopsTrackingRemovedImports(t *testing.T) {
	dir := t.TempDir()
	importPath := filepath.Join(dir, "provider.yaml")
	if err := os.WriteFile(importPath, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write imported config: %v", err)
	}
	path := filepath.Join(dir, "polaris.yaml")
	if err := os.WriteFile(path, []byte("version: 2\nimports:\n  - ./provider.yaml\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	triggered := make(chan string, 8)
	watcher, err := NewWatcher(path, 25*time.Millisecond, func(trigger string) {
		triggered <- trigger
	}, func(err error) {
		t.Fatalf("unexpected watcher error: %v", err)
	})
	if err != nil {
		t.Fatalf("NewWatcher() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watcher.Run(ctx, make(chan os.Signal))

	if err := os.WriteFile(path, []byte("version: 2\n"), 0o600); err != nil {
		t.Fatalf("write config without import: %v", err)
	}
	trigger := waitForWatcherTrigger(t, triggered)
	if !strings.HasPrefix(trigger, "fsnotify:") {
		t.Fatalf("expected fsnotify trigger, got %q", trigger)
	}
	drainWatcherTriggers(triggered, 150*time.Millisecond)

	if err := os.WriteFile(importPath, []byte("version: 2\nproviders: {}\n"), 0o600); err != nil {
		t.Fatalf("write removed imported config: %v", err)
	}
	select {
	case trigger := <-triggered:
		t.Fatalf("expected removed import to be ignored, got trigger %q", trigger)
	case <-time.After(200 * time.Millisecond):
	}
}

func waitForWatcherTrigger(t *testing.T, triggered <-chan string) string {
	t.Helper()
	select {
	case trigger := <-triggered:
		return trigger
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for watcher trigger")
	}
	return ""
}

func drainWatcherTriggers(triggered <-chan string, quiet time.Duration) {
	for {
		select {
		case <-triggered:
		case <-time.After(quiet):
			return
		}
	}
}
