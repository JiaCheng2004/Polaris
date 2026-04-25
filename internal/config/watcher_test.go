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
	if err := os.WriteFile(path, []byte("server:\n  host: 127.0.0.1\n"), 0o600); err != nil {
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
	if err := os.WriteFile(path, []byte("version: 1\n"), 0o600); err != nil {
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
	if err := os.WriteFile(path, []byte("version: 3\n"), 0o600); err != nil {
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
