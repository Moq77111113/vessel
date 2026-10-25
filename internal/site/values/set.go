package values

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

var (
	ErrBadSet              = errors.New("is not NAME=value")
	ErrSetIsEmpty          = errors.New("gives no value, and a value is never empty")
	ErrSecretFileOpen      = errors.New("is readable by other users, chmod 600 it")
	ErrSecretOnCommandLine = errors.New("is a secret, pass it with --set-file so it never shows on a command line")
	ErrSetFilePlain        = errors.New("is not a secret, pass it with --set")
)

func ParseSet(pairs []string) (map[string]string, error) {
	values := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		name, value, ok := strings.Cut(pair, "=")
		if !ok || name == "" {
			return nil, fmt.Errorf("--set %q %w", pair, ErrBadSet)
		}
		if value == "" {
			return nil, fmt.Errorf("--set %s %w", name, ErrSetIsEmpty)
		}
		values[name] = value
	}
	return values, nil
}

func ReadSetFile(pairs []string) (map[string]string, error) {
	values := make(map[string]string, len(pairs))
	for _, pair := range pairs {
		name, path, ok := strings.Cut(pair, "=")
		if !ok || name == "" || path == "" {
			return nil, fmt.Errorf("--set-file %q %w", pair, ErrBadSet)
		}
		body, err := readSecret(path)
		if err != nil {
			return nil, fmt.Errorf("--set-file %s: %w", name, err)
		}
		value := strings.TrimSuffix(string(body), "\n")
		if value == "" {
			return nil, fmt.Errorf("--set-file %s %w", name, ErrSetIsEmpty)
		}
		values[name] = value
	}
	return values, nil
}

func readSecret(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("%s %w", path, ErrSecretFileOpen)
	}
	return io.ReadAll(file)
}

func Answers(variables []delivery.Variable, set, files map[string]string) (map[string]string, error) {
	values := make(map[string]string, len(set)+len(files))
	for _, variable := range variables {
		if _, ok := set[variable.Name]; ok && variable.Secret {
			return nil, fmt.Errorf("--set %s %w", variable.Name, ErrSecretOnCommandLine)
		}
		if _, ok := files[variable.Name]; ok && !variable.Secret {
			return nil, fmt.Errorf("--set-file %s %w", variable.Name, ErrSetFilePlain)
		}
	}
	maps.Copy(values, set)
	maps.Copy(values, files)
	return values, nil
}
