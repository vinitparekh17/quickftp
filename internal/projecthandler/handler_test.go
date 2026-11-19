package projecthandler

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAngularSelectsFilesAndAppliesArtifactRules(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "dist")
	if err := os.MkdirAll(filepath.Join(out, "assets"), 0700); err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string]string{"index.html": "html", "assets/app.js": "js", "assets/app.js.map": "map"} {
		if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(name)), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	got, err := (Angular{}).Prepare(context.Background(), Project{LocalPath: root, OutputRelPath: "dist", ArtifactRulesJSON: `{"include":["*.html","assets/*.js","assets/*.map"],"exclude":["assets/*.map"]}`}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d artifacts: %+v", len(got), got)
	}
}

func TestHandlerRejectsEmptyOutputAndEscapingOutput(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := (DotNet{}).Prepare(context.Background(), Project{LocalPath: root, OutputRelPath: "dist"}, nil); err == nil {
		t.Fatal("empty output was accepted")
	}
	if _, err := (Angular{}).Prepare(context.Background(), Project{LocalPath: root, OutputRelPath: "../outside"}, nil); err == nil {
		t.Fatal("escaping output was accepted")
	}
}

func TestBuildLogsAreForwarded(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "dist"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "dist", "app.dll"), []byte("app"), 0600); err != nil {
		t.Fatal(err)
	}
	var lines []string
	_, err := (DotNet{}).Prepare(context.Background(), Project{LocalPath: root, BuildCommand: "echo build-log", OutputRelPath: "dist"}, func(stream, line string) { lines = append(lines, stream+":"+line) })
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) == 0 {
		t.Fatal("expected build output to be forwarded")
	}
}

func TestLineWriterBoundsOversizedLines(t *testing.T) {
	var lines []string
	w := lineWriter{stream: "stdout", log: func(_ string, line string) { lines = append(lines, line) }}
	prefix := strings.Repeat("x", maxBuildLogLineBytes-1)
	for _, part := range []string{prefix + "é", strings.Repeat("x", maxBuildLogLineBytes*2), "\r\nnext\r", "\n\nunterminated"} {
		n, err := w.Write([]byte(part))
		if err != nil || n != len(part) {
			t.Fatalf("write accepted %d of %d bytes: %v", n, len(part), err)
		}
		if len(w.pending) > maxBuildLogLineBytes+1 {
			t.Fatalf("pending buffer grew to %d bytes", len(w.pending))
		}
	}
	w.flush()
	want := []string{prefix + " [line truncated]", "next", "", "unterminated"}
	if !slices.Equal(lines, want) {
		t.Fatalf("unexpected lines after truncation: got %d lines", len(lines))
	}
}

func TestLineWriterFlushesOversizedUnterminatedLine(t *testing.T) {
	var line string
	w := lineWriter{log: func(_, value string) { line = value }}
	w.Write([]byte(strings.Repeat("Z", maxBuildLogLineBytes+2)))
	w.flush()
	if line != strings.Repeat("Z", maxBuildLogLineBytes)+" [line truncated]" {
		t.Fatal("oversized final line was not truncated")
	}
}
