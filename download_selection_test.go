package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadDestination(t *testing.T) {
	destination := t.TempDir()
	for _, tc := range []struct {
		name, base, remote, relative string
	}{
		{"file in a remote subfolder", "/site/assets", "/site/assets/logo.png", "logo.png"},
		{"nested file in selected folder", "/site/assets", "/site/assets/images/icons/logo.png", "images/icons/logo.png"},
		{"folder from server root", "/", "/images", "images"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := downloadDestination(destination, tc.base, tc.remote)
			want := filepath.Join(destination, filepath.FromSlash(tc.relative))
			if err != nil || got != want {
				t.Fatalf("got %q, %v; want %q", got, err, want)
			}
		})
	}
	for _, remote := range []string{"/site/assets/../outside", "/site/assets-other/file", "/site/assets/log.txt:stream", "/site/assets/CON.txt", "/site/assets/COM¹.txt", "/site/assets/folder./file", "/site/assets/file ", "/site/assets/dir\\file"} {
		if _, err := downloadDestination(destination, "/site/assets", remote); err == nil {
			t.Errorf("accepted unsafe remote path %q", remote)
		}
	}

	if err := os.Symlink(t.TempDir(), filepath.Join(destination, "images")); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	if _, err := downloadDestination(destination, "/site", "/site/images/logo.png"); err == nil {
		t.Error("accepted destination through a symlink outside the selected folder")
	}
}
