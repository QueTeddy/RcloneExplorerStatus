package logging

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoggerWritesStructuredEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	logger, err := NewLogger(path, "info")
	if err != nil {
		t.Fatalf("NewLogger() error = %v", err)
	}
	defer logger.Close()

	logger.Info("startup complete", "component", "service")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "INFO") || !strings.Contains(text, "startup complete") {
		t.Fatalf("log output = %q, want info entry", text)
	}
}

func TestLoggerWritesLiveOutputAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	var live bytes.Buffer
	logger, err := NewLoggerWithOutput(path, "debug", &live)
	if err != nil {
		t.Fatalf("NewLoggerWithOutput() error = %v", err)
	}
	defer logger.Close()

	logger.Debug("install phase", "step", "compile", "result", "ok")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	for label, output := range map[string]string{
		"file": string(contents),
		"live": live.String(),
	} {
		if !strings.Contains(output, "DEBUG") || !strings.Contains(output, "install phase") || !strings.Contains(output, "step=compile") {
			t.Errorf("%s output = %q, want live structured debug entry", label, output)
		}
	}
}

func TestFollowPrintsTailAndStreamsAppendedEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "service.log")
	if err := os.WriteFile(path, []byte("old entry\nlatest entry\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	reader, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		err := Follow(ctx, path, writer, 1, 10*time.Millisecond)
		_ = writer.Close()
		done <- err
	}()

	lines := make(chan string, 2)
	go func() {
		scanner := bufio.NewScanner(reader)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	readLine := func() string {
		t.Helper()
		select {
		case line := <-lines:
			return line
		case <-time.After(2 * time.Second):
			t.Fatal("timed out waiting for live log line")
			return ""
		}
	}
	if got := readLine(); got != "latest entry" {
		t.Fatalf("initial tail line = %q, want latest entry", got)
	}

	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("new live entry\n"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readLine(); got != "new live entry" {
		t.Fatalf("streamed line = %q, want new live entry", got)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Follow() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("log follower did not stop after context cancellation")
	}
	_ = reader.Close()
}
