package quadlet

import (
	"bytes"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const (
	secretKey     = "Secret"
	credentialKey = "LoadCredentialEncrypted"
)

func reads(files []delivery.File) []string {
	return collect(files, func(line []byte) (string, bool) {
		if name, ok := secretName(line); ok {
			return name, true
		}
		return credentialName(line)
	})
}

func credentials(files []delivery.File) []string {
	return collect(files, credentialName)
}

func collect(files []delivery.File, read func(line []byte) (string, bool)) []string {
	set := map[string]bool{}
	for _, file := range files {
		if !owns(file.Path) {
			continue
		}
		for line := range bytes.SplitSeq(file.Data, []byte("\n")) {
			if secret, ok := read(line); ok {
				set[secret] = true
			}
		}
	}
	list := make([]string, 0, len(set))
	for name := range set {
		list = append(list, name)
	}
	sort.Strings(list)
	return list
}

func secretName(line []byte) (string, bool) {
	return keyValue(line, secretKey, ",")
}

func credentialName(line []byte) (string, bool) {
	return keyValue(line, credentialKey, ":")
}

func keyValue(line []byte, key, end string) (string, bool) {
	rest := bytes.TrimLeft(line, " \t")
	if len(rest) == 0 || rest[0] == '#' || rest[0] == ';' {
		return "", false
	}
	value, ok := cutKey(rest, key)
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(string(bytes.TrimSpace(value)), end)
	if name == "" {
		return "", false
	}
	return name, true
}
