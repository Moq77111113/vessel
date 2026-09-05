package quadlet

import (
	"bytes"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

const secretKey = "Secret="

// Requires names the podman secrets these units expect to find on the machine.
//
// A missing secret does not fail the install, it fails the start, minutes later and far
// from its cause. Naming them here turns that into a prerequisite the operator can read.
func (r *Reader) Requires(files []descriptor.File) []string {
	seen := map[string]bool{}
	for _, file := range files {
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
	trimmed := bytes.TrimLeft(line, " \t")
	if len(trimmed) == 0 || trimmed[0] == '#' || trimmed[0] == ';' {
		return "", false
	}
	value, ok := bytes.CutPrefix(trimmed, []byte(secretKey))
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(string(bytes.TrimSpace(value)), ",")
	if name == "" {
		return "", false
	}
	return name, true
}
