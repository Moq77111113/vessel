package compose

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	ErrOutsideProject = errors.New("a path leaves the compose directory")
	ErrCarriesSecrets = errors.New("a path carries the secrets directory, the site gives secrets")
)

func (p project) sources() ([]string, error) {
	var files, binds []string
	for service, svc := range p.Services {
		env, err := envFiles(resolve(&svc.EnvFile))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", service, err)
		}
		files = append(files, env...)
		mounts, err := bindSources(svc.Volumes)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", service, err)
		}
		binds = append(binds, mounts...)
	}
	for _, config := range p.Configs {
		files = append(files, config.File)
	}
	return inside(slices.Concat(localFiles(files), localBinds(binds)))
}

func bindSources(volumes []yaml.Node) ([]string, error) {
	var binds []string
	for _, node := range volumes {
		bind, err := bindSource(resolve(&node))
		if err != nil {
			return nil, err
		}
		binds = append(binds, bind)
	}
	return binds, nil
}

func envFiles(node *yaml.Node) ([]string, error) {
	switch node.Kind {
	case 0:
		return nil, nil
	case yaml.ScalarNode:
		return []string{node.Value}, nil
	}
	var entries []yaml.Node
	if err := node.Decode(&entries); err != nil {
		return nil, fmt.Errorf("read env_file: %w", err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		file, err := envFile(resolve(&entry))
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}

func envFile(node *yaml.Node) (string, error) {
	if node.Kind == yaml.ScalarNode {
		return node.Value, nil
	}
	var long struct {
		Path string `yaml:"path"`
	}
	if err := node.Decode(&long); err != nil {
		return "", fmt.Errorf("read env_file: %w", err)
	}
	return long.Path, nil
}

func bindSource(node *yaml.Node) (string, error) {
	if node.Kind == yaml.ScalarNode {
		source, _, ok := strings.Cut(node.Value, ":")
		if !ok {
			return "", nil
		}
		return source, nil
	}
	var long struct {
		Type   string `yaml:"type"`
		Source string `yaml:"source"`
	}
	if err := node.Decode(&long); err != nil {
		return "", fmt.Errorf("read a volume: %w", err)
	}
	if long.Type != "bind" {
		return "", nil
	}
	return long.Source, nil
}

// compose reads an env or config file from any relative path, but a volume from a relative one only when it starts with a dot.
func localFiles(sources []string) []string {
	return slices.DeleteFunc(sources, func(source string) bool { return source == "" || path.IsAbs(source) })
}

func localBinds(sources []string) []string {
	return slices.DeleteFunc(sources, func(source string) bool { return !strings.HasPrefix(source, ".") })
}

func inside(sources []string) ([]string, error) {
	var list []string
	for _, source := range sources {
		clean := path.Clean(source)
		if clean == ".." || strings.HasPrefix(clean, "../") {
			return nil, fmt.Errorf("%s: %w", source, ErrOutsideProject)
		}
		if clean == "." || clean == secretsDir || strings.HasPrefix(clean, secretsDir+"/") {
			return nil, fmt.Errorf("%s: %w", source, ErrCarriesSecrets)
		}
		list = append(list, clean)
	}
	return list, nil
}
