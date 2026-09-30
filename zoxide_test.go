package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestQueryZoxideDirsNotInstalled(t *testing.T) {
	t.Setenv("PATH", filepath.Join(t.TempDir(), "empty"))
	_, err := QueryZoxideDirs("")
	if !errors.Is(err, ErrZoxideNotInstalled) {
		t.Fatalf("expected ErrZoxideNotInstalled, got %v", err)
	}
}

func TestQueryZoxideDirsWithFakeBinary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "zoxide")
	script := `#!/bin/sh
if [ "$1" = query ] && [ "$2" = -l ]; then
  echo /tmp/alpha
  echo /tmp/beta
fi
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	paths, err := QueryZoxideDirs("")
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 2 || paths[0] != "/tmp/alpha" || paths[1] != "/tmp/beta" {
		t.Fatalf("paths = %v", paths)
	}
}
