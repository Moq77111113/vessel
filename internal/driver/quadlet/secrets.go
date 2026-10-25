package quadlet

import (
	"bytes"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

const secretKey = "Secret"

// reads names the podman secrets these units expect to find on the machine.
func reads(files []delivery.File) []string {
	seen := map[string]bool{}
	for _, file := range files {
		if !owns(file.Path) {
			continue
		}
		for line := range bytes.SplitSeq(file.Data, []byte("\n")) {
			name, ok := secretName(line)
			if ok {
				seen[name] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func secretName(line []byte) (string, bool) {
	rest := bytes.TrimLeft(line, " \t")
	if len(rest) == 0 || rest[0] == '#' || rest[0] == ';' {
		return "", false
	}
	value, ok := cutKey(rest, secretKey)
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(string(bytes.TrimSpace(value)), ",")
	if name == "" {
		return "", false
	}
	return name, true
}
