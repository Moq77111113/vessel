package compose

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const secretsDir = "secrets"

const storePath = "etc/compose/.secrets"

var secretName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]*$`)

var (
	ErrSecretName  = errors.New("a secret name is not a plain file name")
	ErrSecretFile  = errors.New("a secret is not declared as file: ./secrets/<name>")
	ErrSecretOwner = errors.New("two services give one secret two owners")
	ErrSecretField = errors.New("a secret uid, gid or mode is not a number")
)

type owner struct {
	uid  int
	gid  int
	mode fs.FileMode
}

var rootOnly = owner{uid: 0, gid: 0, mode: 0o600}

type longSecret struct {
	Source string    `yaml:"source"`
	UID    yaml.Node `yaml:"uid"`
	GID    yaml.Node `yaml:"gid"`
	Mode   yaml.Node `yaml:"mode"`
}

func (s longSecret) setsOwner() bool {
	return s.UID.Kind != 0 || s.GID.Kind != 0 || s.Mode.Kind != 0
}

func (p project) secretNames() ([]string, error) {
	set := map[string]bool{}
	for service, svc := range p.Services {
		for _, node := range svc.Secrets {
			secret, err := readSecret(resolve(&node))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", service, err)
			}
			if err := p.checkSecretFile(secret.Source); err != nil {
				return nil, fmt.Errorf("%s: %w", service, err)
			}
			set[secret.Source] = true
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (p project) owners() (map[string]owner, error) {
	owners := map[string]owner{}
	for service, svc := range p.Services {
		for _, node := range svc.Secrets {
			secret, err := readSecret(resolve(&node))
			if err != nil {
				return nil, fmt.Errorf("%s: %w", service, err)
			}
			if !secret.setsOwner() {
				continue
			}
			next, err := secret.owner()
			if err != nil {
				return nil, fmt.Errorf("%s: %w", service, err)
			}
			if known, ok := owners[secret.Source]; ok && known != next {
				return nil, fmt.Errorf("%s: %w", secret.Source, ErrSecretOwner)
			}
			owners[secret.Source] = next
		}
	}
	return owners, nil
}

func (p project) checkSecretFile(name string) error {
	if !secretName.MatchString(name) {
		return fmt.Errorf("%q: %w", name, ErrSecretName)
	}
	declared, ok := p.Secrets[name]
	if !ok || path.Clean(declared.File) != path.Join(secretsDir, name) {
		return fmt.Errorf("%s: %w", name, ErrSecretFile)
	}
	return nil
}

func readSecret(node *yaml.Node) (longSecret, error) {
	if node.Kind == yaml.ScalarNode {
		return longSecret{Source: node.Value}, nil
	}
	var secret longSecret
	if err := node.Decode(&secret); err != nil {
		return longSecret{}, fmt.Errorf("read a secret: %w", err)
	}
	return secret, nil
}

func (s longSecret) owner() (owner, error) {
	uid, err := number(s.UID, 10, rootOnly.uid)
	if err != nil {
		return owner{}, err
	}
	gid, err := number(s.GID, 10, rootOnly.gid)
	if err != nil {
		return owner{}, err
	}
	mode, err := number(s.Mode, 8, int(rootOnly.mode))
	if err != nil {
		return owner{}, err
	}
	return owner{uid: uid, gid: gid, mode: fs.FileMode(mode)}, nil
}

func number(node yaml.Node, base int, fallback int) (int, error) {
	if node.Kind == 0 {
		return fallback, nil
	}
	if base == 8 && !strings.HasPrefix(node.Value, "0") {
		base = 10
	}
	value, err := strconv.ParseUint(strings.TrimPrefix(node.Value, "0o"), base, 32)
	if err != nil {
		return 0, fmt.Errorf("%q: %w", node.Value, ErrSecretField)
	}
	return int(value), nil
}
