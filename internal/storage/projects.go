package storage

import (
	"database/sql"
	"errors"
	"fmt"
)

type Project struct {
	ID                int64
	Name              string
	Type              string
	LocalPath         string
	BuildCommand      string
	OutputRelPath     string
	ArtifactRulesJSON string
}

func (db *DB) UpsertProject(p *Project) error {
	if p.ArtifactRulesJSON == "" {
		p.ArtifactRulesJSON = "{}"
	}
	return db.Conn.QueryRow(`INSERT INTO projects(name, project_type, local_path, build_cmd, output_rel_path, artifact_rules_json) VALUES(?, ?, ?, ?, ?, ?) ON CONFLICT(name) DO UPDATE SET project_type=excluded.project_type, local_path=excluded.local_path, build_cmd=excluded.build_cmd, output_rel_path=excluded.output_rel_path, artifact_rules_json=excluded.artifact_rules_json RETURNING id`, p.Name, p.Type, p.LocalPath, p.BuildCommand, p.OutputRelPath, p.ArtifactRulesJSON).Scan(&p.ID)
}

func (db *DB) GetProjectByName(name string) (*Project, error) {
	var p Project
	err := db.Conn.QueryRow(`SELECT id,name,project_type,local_path,build_cmd,output_rel_path,artifact_rules_json FROM projects WHERE name=?`, name).Scan(&p.ID, &p.Name, &p.Type, &p.LocalPath, &p.BuildCommand, &p.OutputRelPath, &p.ArtifactRulesJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("project %q not found", name)
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}
