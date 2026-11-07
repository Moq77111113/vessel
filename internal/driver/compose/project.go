package compose

import (
	"errors"
	"fmt"
	"path"
	"regexp"

	"gopkg.in/yaml.v3"
)

const composeFile = "compose.yaml"

const composePath = "etc/compose"

var ErrProjectName = errors.New("the compose file carries no name compose accepts")

var projectName = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

type project struct {
	Name     string             `yaml:"name"`
	Services map[string]service `yaml:"services"`
	Secrets  map[string]source  `yaml:"secrets"`
	Configs  map[string]source  `yaml:"configs"`
}

type service struct {
	Image   yaml.Node   `yaml:"image"`
	Build   yaml.Node   `yaml:"build"`
	Volumes []yaml.Node `yaml:"volumes"`
	EnvFile yaml.Node   `yaml:"env_file"`
	Secrets []yaml.Node `yaml:"secrets"`
}

type source struct {
	File string `yaml:"file"`
}

func readProject(data []byte) (project, error) {
	var file project
	if err := yaml.Unmarshal(data, &file); err != nil {
		return project{}, fmt.Errorf("read %s: %w", composeFile, err)
	}
	if !projectName.MatchString(file.Name) {
		return project{}, fmt.Errorf("%q: %w", file.Name, ErrProjectName)
	}
	return file, nil
}

func (p project) dir() string { return path.Join(composePath, p.Name) }

func (p project) path() string { return path.Join(p.dir(), composeFile) }

func resolve(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}
