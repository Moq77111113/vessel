package quadlet

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const credstorePath = "etc/credstore.encrypted"

var ErrCredentialLine = errors.New("a secret a unit loads as a credential holds a line break")

type credstore struct {
	root string
	run  Run
}

func (c credstore) dir() string { return filepath.Join(c.root, credstorePath) }

func (c credstore) names() ([]string, error) {
	entries, err := os.ReadDir(c.dir())
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", c.dir(), err)
	}
	var list []string
	for _, entry := range entries {
		if entry.Type().IsRegular() {
			list = append(list, entry.Name())
		}
	}
	return list, nil
}

func (c credstore) holds(name string) bool {
	info, err := os.Stat(filepath.Join(c.dir(), name))
	return err == nil && info.Mode().IsRegular()
}

func (c credstore) add(ctx context.Context, secret delivery.Secret) error {
	if strings.ContainsAny(secret.Value, "\r\n") {
		return fmt.Errorf("%s: %w", secret.Name, ErrCredentialLine)
	}
	if err := os.MkdirAll(c.dir(), 0o700); err != nil {
		return fmt.Errorf("create %s: %w", c.dir(), err)
	}
	target := filepath.Join(c.dir(), secret.Name)
	output, err := c.run(ctx, Command{
		Line:  []string{"systemd-creds", "encrypt", "--name=" + secret.Name, "-", target},
		Stdin: strings.NewReader(secret.Name + "=" + secret.Value + "\n"),
	})
	if err != nil {
		return fmt.Errorf("systemd-creds encrypt %s: %w: %s", secret.Name, err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (c credstore) reads(name string) (bool, error) {
	units, err := c.units()
	if err != nil {
		return false, err
	}
	return slices.Contains(credentials(units), name), nil
}

func (c credstore) units() ([]delivery.File, error) {
	dir := filepath.Join(c.root, systemdPath)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var units []delivery.File
	for _, entry := range entries {
		if !entry.Type().IsRegular() || !slices.Contains(unitSuffixes, path.Ext(entry.Name())) {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", entry.Name(), err)
		}
		units = append(units, delivery.File{Path: path.Join(systemdPath, entry.Name()), Data: data})
	}
	return units, nil
}
