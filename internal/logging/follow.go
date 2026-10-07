package logging

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

func Follow(ctx context.Context, path string, output io.Writer, tailLines int, interval time.Duration) error {
	if output == nil {
		return fmt.Errorf("log output is required")
	}
	if interval <= 0 {
		interval = 300 * time.Millisecond
	}
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", path, err)
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("read log file %q: %w", path, err)
	}
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	if len(data) > 0 && tailLines >= 0 && len(lines) > tailLines {
		lines = lines[len(lines)-tailLines:]
	}
	if len(data) > 0 {
		for _, line := range lines {
			if _, err := fmt.Fprintln(output, string(line)); err != nil {
				return fmt.Errorf("write log output: %w", err)
			}
		}
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	buffer := make([]byte, 4096)
	var pending []byte
	for {
		count, readErr := file.Read(buffer)
		if count > 0 {
			pending = append(pending, buffer[:count]...)
			for {
				newline := bytes.IndexByte(pending, '\n')
				if newline < 0 {
					break
				}
				line := strings.TrimSuffix(string(pending[:newline]), "\r")
				if _, err := fmt.Fprintln(output, line); err != nil {
					return fmt.Errorf("write live log output: %w", err)
				}
				pending = pending[newline+1:]
			}
		}
		if readErr != nil && readErr != io.EOF {
			return fmt.Errorf("read live log file: %w", readErr)
		}
		select {
		case <-ctx.Done():
			if len(pending) > 0 {
				_, _ = fmt.Fprintln(output, string(pending))
			}
			return nil
		case <-ticker.C:
		}
	}
}
