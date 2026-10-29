package projecthandler

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var targetFrameworkName = regexp.MustCompile(`^net[0-9][A-Za-z0-9.-]*$`)

var conventionalPublish = regexp.MustCompile(`^bin/(Release|Debug)/[^/]+/publish$`)

type dotnetProperty struct {
	Value     string `xml:",chardata"`
	Condition string `xml:"Condition,attr"`
}

type dotnetMetadata struct {
	XMLName xml.Name `xml:"Project"`
	SDK     string   `xml:"Sdk,attr"`
	Groups  []struct {
		Condition  string         `xml:"Condition,attr"`
		Framework  dotnetProperty `xml:"TargetFramework"`
		Frameworks dotnetProperty `xml:"TargetFrameworks"`
	} `xml:"PropertyGroup"`
}

// resolveDotNet adjusts only the conventional publish command and output path.
// Custom commands and paths remain explicit; ambiguous solutions never pick an
// arbitrary application or target framework.
func resolveDotNet(p Project, log LogFunc) (Project, error) {
	root, err := filepath.Abs(p.LocalPath)
	if err != nil {
		return p, err
	}
	command := strings.TrimSpace(p.BuildCommand)
	standardBuild := command == "dotnet publish -c Release" || command == "dotnet publish --configuration Release"
	automatic := p.OutputRelPath == "" || strings.EqualFold(p.OutputRelPath, "auto")
	conventional := conventionalPublish.MatchString(strings.ReplaceAll(p.OutputRelPath, "\\", "/"))
	if !automatic && (!conventional || !standardBuild) {
		return p, nil
	}
	paths, err := filepath.Glob(filepath.Join(root, "*.csproj"))
	if err != nil {
		return p, err
	}
	if len(paths) == 0 && standardBuild {
		// A solution folder commonly contains one web application plus libraries.
		paths, err = filepath.Glob(filepath.Join(root, "*", "*.csproj"))
		if err != nil {
			return p, err
		}
	}
	type candidate struct {
		path, framework string
		web             bool
	}
	var candidates []candidate
	for _, file := range paths {
		raw, err := os.ReadFile(file)
		if err != nil {
			return p, fmt.Errorf("read .NET project: %w", err)
		}
		var metadata dotnetMetadata
		if err := xml.Unmarshal(raw, &metadata); err != nil {
			return p, fmt.Errorf("parse .NET project %s: %w", file, err)
		}
		frameworks := map[string]bool{}
		conditional := false
		for _, group := range metadata.Groups {
			if (group.Condition != "" || group.Framework.Condition != "" || group.Frameworks.Condition != "") && (group.Framework.Value != "" || group.Frameworks.Value != "") {
				conditional = true
			}
			for _, framework := range strings.Split(group.Framework.Value+";"+group.Frameworks.Value, ";") {
				framework = strings.TrimSpace(framework)
				if framework != "" {
					frameworks[framework] = true
				}
			}
		}
		framework := ""
		if len(frameworks) == 1 && !conditional {
			for value := range frameworks {
				framework = value
			}
		}
		candidates = append(candidates, candidate{file, framework, strings.Contains(metadata.SDK, "Microsoft.NET.Sdk.Web")})
	}
	if len(candidates) > 1 {
		var web []candidate
		for _, candidate := range candidates {
			if candidate.web {
				web = append(web, candidate)
			}
		}
		if len(web) == 1 {
			candidates = web
		}
	}
	if len(candidates) != 1 {
		return p, fmt.Errorf("cannot automatically select .NET publish output: found %d candidate projects under %s; select the application folder and set an explicit output path for ambiguous solutions", len(candidates), root)
	}
	selected := candidates[0]
	if !targetFrameworkName.MatchString(selected.framework) {
		return p, fmt.Errorf("cannot automatically resolve TargetFramework in %s (inherited, conditional, or multiple frameworks); specify the build framework and output directory explicitly", selected.path)
	}
	projectRoot := filepath.Dir(selected.path)
	if standardBuild {
		p.LocalPath = projectRoot
	} else if projectRoot != root {
		return p, fmt.Errorf("select application folder %s for automatic .NET output detection", projectRoot)
	}
	if automatic && !standardBuild && command != "" {
		return p, fmt.Errorf("automatic .NET output requires the standard dotnet publish -c Release command; set the output path explicitly for custom commands")
	}
	configuration := "Release"
	if strings.HasPrefix(strings.ReplaceAll(p.OutputRelPath, "\\", "/"), "bin/Debug/") {
		configuration = "Debug"
	}
	p.OutputRelPath = filepath.Join("bin", configuration, selected.framework, "publish")
	if log != nil {
		log("stdout", fmt.Sprintf("Detected .NET target %s; project %s; publish output %s", selected.framework, selected.path, filepath.Join(p.LocalPath, p.OutputRelPath)))
	}
	return p, nil
}
