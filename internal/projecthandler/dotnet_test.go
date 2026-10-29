package projecthandler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDotNetProject(t *testing.T, root, name, sdk, frameworks string) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".csproj"), []byte(`<Project Sdk="`+sdk+`"><PropertyGroup>`+frameworks+`</PropertyGroup></Project>`), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestDotNetDetectsWebProjectAndFrameworkFromSolution(t *testing.T) {
	root := t.TempDir()
	app := writeDotNetProject(t, root, "API", "Microsoft.NET.Sdk.Web", "<TargetFramework>net10.0</TargetFramework>")
	writeDotNetProject(t, root, "Core", "Microsoft.NET.Sdk", "<TargetFramework>net10.0</TargetFramework>")
	out := filepath.Join(app, "bin", "Release", "net10.0", "publish")
	if err := os.MkdirAll(out, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(out, "API.dll"), []byte("app"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, configured := range []string{"auto", "bin/Release/net8.0/publish"} {
		p, err := resolveDotNet(Project{LocalPath: root, BuildCommand: "dotnet publish -c Release", OutputRelPath: configured}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if p.LocalPath != app || filepath.Join(p.LocalPath, p.OutputRelPath) != out {
			t.Fatalf("incorrect detection: %+v", p)
		}
		p.BuildCommand = ""
		artifacts, err := (DotNet{}).Prepare(context.Background(), p, nil)
		if err != nil || len(artifacts) != 1 {
			t.Fatalf("artifact collection: %v, %v", artifacts, err)
		}
	}
}

func TestDotNetRejectsAmbiguousProjectsAndFrameworks(t *testing.T) {
	root := t.TempDir()
	writeDotNetProject(t, root, "One", "Microsoft.NET.Sdk.Web", "<TargetFramework>net10.0</TargetFramework>")
	writeDotNetProject(t, root, "Two", "Microsoft.NET.Sdk.Web", "<TargetFramework>net10.0</TargetFramework>")
	if _, err := resolveDotNet(Project{LocalPath: root, BuildCommand: "dotnet publish -c Release", OutputRelPath: "auto"}, nil); err == nil {
		t.Fatal("ambiguous solution silently selected an application")
	}
	multi := writeDotNetProject(t, t.TempDir(), "Multi", "Microsoft.NET.Sdk.Web", "<TargetFrameworks>net8.0;net10.0</TargetFrameworks>")
	if _, err := resolveDotNet(Project{LocalPath: multi, OutputRelPath: "auto"}, nil); err == nil || !strings.Contains(err.Error(), "TargetFramework") {
		t.Fatalf("ambiguous framework: %v", err)
	}
}

func TestDotNetPreservesCustomOutputAndCommand(t *testing.T) {
	original := Project{LocalPath: t.TempDir(), BuildCommand: "dotnet publish -o custom", OutputRelPath: "custom"}
	got, err := resolveDotNet(original, nil)
	if err != nil || got != original {
		t.Fatalf("changed explicit configuration: %+v, %v", got, err)
	}
}

func TestDotNetDoesNotGuessConditionalFramework(t *testing.T) {
	root := writeDotNetProject(t, t.TempDir(), "API", "Microsoft.NET.Sdk.Web", `<TargetFramework Condition="'$(Configuration)' == 'Debug'">net8.0</TargetFramework>`)
	if _, err := resolveDotNet(Project{LocalPath: root, OutputRelPath: "auto"}, nil); err == nil {
		t.Fatal("conditional framework was guessed")
	}
}

func TestDotNetFirstPublishIncludesCompleteOutputByDefault(t *testing.T) {
	root := writeDotNetProject(t, t.TempDir(), "API", "Microsoft.NET.Sdk.Web", "<TargetFramework>net10.0</TargetFramework>")
	names := []string{"API.dll", "API.deps.json", "API.runtimeconfig.json", "web.config", "runtimes/win-x64/native/dependency.dll"}
	for _, name := range names {
		file := filepath.Join(root, "bin", "Release", "net10.0", "publish", filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("publish artifact"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	artifacts, err := (DotNet{}).Prepare(context.Background(), Project{LocalPath: root, OutputRelPath: "auto", ArtifactRulesJSON: `{"include":[],"exclude":[]}`}, nil)
	if err != nil || len(artifacts) != len(names) {
		t.Fatalf("first deployment lost publish files: %+v, %v", artifacts, err)
	}
}
