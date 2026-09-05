package quadlet

import (
	"path"
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

// Start returns the two systemctl lines that bring these units up.
//
// systemctl start, never enable: a quadlet unit is generated, so enable fails on it.
// Starting at boot comes from the [Install] section the unit already carries.
func (r *Reader) Start(files []descriptor.File) []string {
	services := Services(files)
	if len(services) == 0 {
		return nil
	}
	return []string{"systemctl daemon-reload", "systemctl start " + strings.Join(services, " ")}
}

// Services names the systemd services these units generate, in start order.
func Services(files []descriptor.File) []string {
	var names []string
	for _, file := range files {
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
