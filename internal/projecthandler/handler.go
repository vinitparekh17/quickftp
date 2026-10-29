// Package projecthandler builds projects and selects publish artifacts.
package projecthandler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type Project struct{ Type, LocalPath, BuildCommand, OutputRelPath, ArtifactRulesJSON string }
type Artifact struct{ LocalPath, RelativePath string }
type LogFunc func(stream, line string)
type Handler interface {
	Prepare(context.Context, Project, LogFunc) ([]Artifact, error)
}
type Angular struct{}
type DotNet struct{}

func ForType(t string) (Handler, error) {
	switch strings.ToLower(t) {
	case "angular":
		return Angular{}, nil
	case "dotnet":
		return DotNet{}, nil
	default:
		return nil, fmt.Errorf("unsupported project type %q", t)
	}
}

func (Angular) Prepare(ctx context.Context, p Project, log LogFunc) ([]Artifact, error) {
	return prepare(ctx, p, log)
}
func (DotNet) Prepare(ctx context.Context, p Project, log LogFunc) ([]Artifact, error) {
	if p.LocalPath == "" {
		return nil, errors.New("project path is required")
	}
	resolved, err := resolveDotNet(p, log)
	if err != nil {
		return nil, err
	}
	return prepare(ctx, resolved, log)
}

type artifactRules struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

func prepare(ctx context.Context, p Project, log LogFunc) ([]Artifact, error) {
	if p.LocalPath == "" {
		return nil, errors.New("project path is required")
	}
	root, err := filepath.Abs(p.LocalPath)
	if err != nil {
		return nil, err
	}
	if p.BuildCommand != "" {
		cmd, err := buildCommand(ctx, root, p.BuildCommand)
		if err != nil {
			return nil, err
		}
		streamLogs(cmd, log)
		runErr := cmd.Run()
		if w, ok := cmd.Stdout.(*lineWriter); ok {
			w.flush()
		}
		if w, ok := cmd.Stderr.(*lineWriter); ok {
			w.flush()
		}
		if runErr != nil {
			return nil, fmt.Errorf("build failed: %w", runErr)
		}
	}
	if p.OutputRelPath == "" {
		return nil, errors.New("output path is required")
	}
	output := filepath.Join(root, filepath.Clean(p.OutputRelPath))
	rel, err := filepath.Rel(root, output)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("output path escapes project root")
	}
	info, err := os.Stat(output)
	if err != nil {
		return nil, fmt.Errorf("publish/build output directory %s is unavailable: %w; check that the build command publishes to the configured output path relative to %s", output, err, root)
	}
	if !info.IsDir() {
		return nil, errors.New("output path must be a directory")
	}
	var rules artifactRules
	if p.ArtifactRulesJSON != "" && p.ArtifactRulesJSON != "{}" {
		if err := json.Unmarshal([]byte(p.ArtifactRulesJSON), &rules); err != nil {
			return nil, fmt.Errorf("parse artifact rules: %w", err)
		}
	}
	var artifacts []Artifact
	err = filepath.WalkDir(output, func(name string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		pathRel, err := filepath.Rel(output, name)
		if err != nil {
			return err
		}
		pathRel = filepath.ToSlash(pathRel)
		matches, err := matchesRules(pathRel, rules)
		if err != nil {
			return err
		}
		if matches {
			artifacts = append(artifacts, Artifact{name, pathRel})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(artifacts) == 0 {
		return nil, errors.New("build output contains no deployable files")
	}
	return artifacts, nil
}

func matchesRules(name string, rules artifactRules) (bool, error) {
	included := len(rules.Include) == 0
	for _, pattern := range rules.Include {
		match, err := matchArtifactPattern(name, pattern)
		if err != nil {
			return false, fmt.Errorf("invalid artifact include pattern %q: %w", pattern, err)
		}
		if match {
			included = true
		}
	}
	if !included {
		return false, nil
	}
	for _, pattern := range rules.Exclude {
		match, err := matchArtifactPattern(name, pattern)
		if err != nil {
			return false, fmt.Errorf("invalid artifact exclude pattern %q: %w", pattern, err)
		}
		if match {
			return false, nil
		}
	}
	return true, nil
}

// matchArtifactPattern supports ** as a whole path segment for recursive directory selection.
func matchArtifactPattern(name, pattern string) (bool, error) {
	nameParts := strings.Split(path.Clean(filepath.ToSlash(name)), "/")
	patternParts := strings.Split(path.Clean(strings.ReplaceAll(pattern, "\\", "/")), "/")
	var match func(int, int) (bool, error)
	match = func(i, j int) (bool, error) {
		if j == len(patternParts) {
			return i == len(nameParts), nil
		}
		if patternParts[j] == "**" {
			if ok, err := match(i, j+1); ok || err != nil {
				return ok, err
			}
			if i < len(nameParts) {
				return match(i+1, j)
			}
			return false, nil
		}
		if i >= len(nameParts) {
			return false, nil
		}
		segmentOK, err := filepath.Match(patternParts[j], nameParts[i])
		if err != nil || !segmentOK {
			return false, err
		}
		return match(i+1, j+1)
	}
	return match(0, 0)
}
func buildCommand(ctx context.Context, dir, command string) (*exec.Cmd, error) {
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.CommandContext(ctx, "cmd.exe", "/D", "/S", "/C", command)
	} else {
		cmd = exec.CommandContext(ctx, "/bin/sh", "-c", command)
	}
	cmd.Dir = dir
	return cmd, nil
}
func streamLogs(cmd *exec.Cmd, log LogFunc) {
	if log == nil {
		return
	}
	var callbackMu sync.Mutex
	safeLog := func(stream, line string) { callbackMu.Lock(); defer callbackMu.Unlock(); log(stream, line) }
	cmd.Stdout = &lineWriter{stream: "stdout", log: safeLog}
	cmd.Stderr = &lineWriter{stream: "stderr", log: safeLog}
}

type lineWriter struct {
	mu      sync.Mutex
	stream  string
	log     LogFunc
	pending string
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n := len(p)
	w.pending += string(p)
	for {
		i := strings.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		line := strings.TrimSuffix(w.pending[:i], "\r")
		w.log(w.stream, line)
		w.pending = w.pending[i+1:]
	}
	return n, nil
}
func (w *lineWriter) flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.pending != "" {
		w.log(w.stream, w.pending)
		w.pending = ""
	}
}
