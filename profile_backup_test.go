package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileBackups(t *testing.T) {
	source := backupTestApp(t)
	full, err := json.Marshal(sampleBackup(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.importData(full); err != nil {
		t.Fatal(err)
	}
	c, err := source.db.GetConnectionByName("server")
	if err != nil {
		t.Fatal(err)
	}
	p, err := source.db.GetProjectByName("site")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		kind, other           string
		id                    int64
		connections, projects int
	}{
		{"connection", "project", c.ID, 1, 0},
		{"project", "connection", p.ID, 0, 1},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			dest := backupTestApp(t)
			settings := dest.GetSettings()
			dest.settingsPath = "" // Profile imports must work without writing settings.
			raw, err := source.profileBackup(tc.kind, tc.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range []string{`"configuration"`, `"password"`, c.CredTargetKey} {
				if strings.Contains(string(raw), part) {
					t.Fatalf("unexpected content in profile export: %s", part)
				}
			}
			for _, format := range []string{"quickftp", "quickftp-" + tc.other} {
				if _, err := decodeBackupFormat(raw, format); err == nil {
					t.Fatalf("profile accepted as %s", format)
				}
			}
			format := "quickftp-" + tc.kind
			if _, err := decodeBackupFormat(full, format); err == nil {
				t.Fatal("full backup accepted as profile")
			}
			backup, err := decodeBackupFormat(raw, format)
			if err != nil {
				t.Fatal(err)
			}
			backup.Configuration = &settings
			mixed, err := json.Marshal(backup)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := decodeBackupFormat(mixed, format); err == nil {
				t.Fatal("profile accepted with app settings")
			}
			backup.Configuration = nil
			result, err := dest.applyBackup(backup)
			if err != nil {
				t.Fatal(err)
			}
			if result.Connections != tc.connections || result.Projects != tc.projects || dest.GetSettings() != settings {
				t.Fatalf("wrong import scope: %+v", result)
			}
			if tc.kind == "project" {
				got, err := dest.db.GetProjectByName(p.Name)
				if err != nil || got.BuildCommand != p.BuildCommand || got.ArtifactRulesJSON != p.ArtifactRulesJSON {
					t.Fatalf("project did not round trip: %+v, %v", got, err)
				}
			} else {
				got, err := dest.db.GetConnectionByName(c.Name)
				if err != nil || got.Host != c.Host || got.Username != c.Username || got.TLSVerify != c.TLSVerify {
					t.Fatalf("connection did not round trip: %+v, %v", got, err)
				}
			}
			result, err = dest.applyBackup(backup)
			if err != nil || result.Skipped != 1 {
				t.Fatalf("duplicate import: %+v, %v", result, err)
			}
		})
	}
}
