package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStreamedDownloadPublishesOnlyCompleteFiles(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partial" {
			w.Header().Set("Content-Length", "100")
		}
		fmt.Fprint(w, "payload")
	}))
	defer server.Close()
	if downloadClient.Timeout <= 15*time.Second {
		t.Fatal("attachment transfers still use the short API timeout")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "download")
	if err := DownloadFile("unused", server.URL, path); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{path, filepath.Join(dir, "new-download")} {
		if err := DownloadFile("unused", server.URL+"/partial", target); err == nil {
			t.Fatal("incomplete transfer reported success")
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "payload" {
		t.Fatal("failed transfer replaced the complete file")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatal("failed transfer left a partial file or temporary artifact")
	}
}
