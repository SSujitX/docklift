//go:build unix

package files

import (
	"path/filepath"
	"syscall"
	"testing"
)

// A FIFO is opened without blocking and refused.
func TestSpecialFiles(t *testing.T) {
	f := newFixture(t)
	if err := syscall.Mkfifo(filepath.Join(f.proj, "pipe"), 0o644); err != nil {
		t.Skipf("mkfifo: %v", err)
	}
	code, body := f.do(t, "GET", contentURL(projectID, "pipe"), "")
	expectErr(t, code, body, 400, "Path is not a regular file")
	code, body = f.do(t, "PUT", contentURL(projectID, "pipe"), `{"content":"x"}`)
	expectErr(t, code, body, 500, "Failed to update file")
}
