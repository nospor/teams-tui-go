package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ErrZoxideNotInstalled is returned when the zoxide executable is not on PATH.
var ErrZoxideNotInstalled = errors.New("zoxide not installed")

// QueryZoxideDirs runs `zoxide query -l` with optional filter words from query.
func QueryZoxideDirs(query string) ([]string, error) {
	if _, err := exec.LookPath("zoxide"); err != nil {
		return nil, ErrZoxideNotInstalled
	}

	args := []string{"query", "-l"}
	trimmed := strings.TrimSpace(query)
	if trimmed != "" {
		args = append(args, strings.Fields(trimmed)...)
	}

	out, err := exec.Command("zoxide", args...).Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok && len(ee.Stderr) > 0 {
			return nil, fmt.Errorf("%w: %s", err, strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}

	var paths []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, line)
	}
	return paths, nil
}
