package quadlet

import (
	"path"
	"slices"
	"sort"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
)

// serviceSuffixes maps a quadlet unit to the suffix systemd generates for it.
// A .network or a .volume starts nothing on its own: the services that need it pull it in.
var serviceSuffixes = map[string]string{
	".container": ".service",
	".kube":      ".service",
	".pod":       "-pod.service",
}

// Start returns the systemctl lines that bring these units up.
//
// systemctl start on a quadlet unit, never enable: it is generated, so enable fails on it. Starting
// at boot comes from the [Install] section the unit already carries. A .timer is not generated, so
// it takes enable --now like any hand-written unit.
func (r *Reader) Start(files []descriptor.File) []string {
	services := serviceNames(files)
	timers := siblings(files)
	if len(services) == 0 && len(timers) == 0 {
		return nil
	}
	lines := []string{"systemctl daemon-reload"}
	if len(services) > 0 {
		lines = append(lines, "systemctl start "+strings.Join(services, " "))
	}
	for _, name := range timers {
		lines = append(lines, "systemctl enable --now "+name)
	}
	return lines
}

// siblings names the plain systemd units these files carry, in a stable order.
func siblings(files []descriptor.File) []string {
	var names []string
	for _, file := range files {
		if !under(file.Path, siblingPath) {
			continue
		}
		base := path.Base(file.Path)
		if slices.Contains(siblingSuffixes, path.Ext(base)) {
			names = append(names, base)
		}
	}
	sort.Strings(names)
	return names
}

// serviceNames names the systemd services these units generate, in start order.
func serviceNames(files []descriptor.File) []string {
	var names []string
	for _, file := range files {
		if !under(file.Path, systemdPath) {
			continue
		}
		base := path.Base(file.Path)
		extension := path.Ext(base)
		suffix, ok := serviceSuffixes[extension]
		if !ok {
			continue
		}
		names = append(names, strings.TrimSuffix(base, extension)+suffix)
	}
	sort.Strings(names)
	return names
}
