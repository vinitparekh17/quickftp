package transfer

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateDownloadDestination checks local path names and containment before a write.
func ValidateDownloadDestination(root, dest string) (string, error) {
	base, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	target, e := filepath.Abs(dest)
	if e != nil {
		return "", e
	}
	rel, e := filepath.Rel(base, target)
	if e != nil {
		return "", e
	}
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("destination is outside the selected download directory")
	}
	if rel != "." {
		if _, err := safeRelative(filepath.ToSlash(rel)); err != nil {
			return "", err
		}
	}
	if e = os.MkdirAll(base, 0700); e != nil {
		return "", fmt.Errorf("create download directory: %w", e)
	}
	rootReal, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	probe := target
	for {
		_, e = os.Lstat(probe)
		if e == nil {
			resolved, re := filepath.EvalSymlinks(probe)
			if re != nil {
				return "", re
			}
			inside, re := filepath.Rel(rootReal, resolved)
			if re != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(os.PathSeparator)) {
				return "", fmt.Errorf("destination resolves outside the selected download directory")
			}
			break
		}
		if !os.IsNotExist(e) {
			return "", e
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return "", fmt.Errorf("could not resolve destination parent")
		}
		probe = parent
	}
	return target, nil
}
