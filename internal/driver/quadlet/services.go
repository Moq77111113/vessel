package quadlet

import (
	"path"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/pkg/delivery"
)

var serviceSuffixes = map[string]string{
	".container": ".service",
	".kube":      ".service",
	".pod":       "-pod.service",
}

func services(files []delivery.File) []delivery.Service {
	var generated, plain []delivery.Service
	for _, file := range files {
		base := path.Base(file.Path)
		extension := path.Ext(base)
		if under(file.Path, siblingPath) && slices.Contains(siblingSuffixes, extension) {
			plain = append(plain, delivery.Service{Name: base, Path: file.Path})
			continue
		}
		suffix, ok := serviceSuffixes[extension]
		if !under(file.Path, systemdPath) || !ok {
			continue
		}
		generated = append(generated, delivery.Service{Name: strings.TrimSuffix(base, extension) + suffix, Path: file.Path})
	}
	byName := func(a, b delivery.Service) int { return strings.Compare(a.Name, b.Name) }
	slices.SortFunc(generated, byName)
	slices.SortFunc(plain, byName)
	return append(generated, plain...)
}

func startLines(services []delivery.Service) []string {
	generated, plain := split(services)
	if len(generated) == 0 && len(plain) == 0 {
		return nil
	}
	lines := []string{"systemctl daemon-reload"}
	if len(generated) > 0 {
		lines = append(lines, "systemctl start "+strings.Join(generated, " "))
	}
	for _, name := range plain {
		lines = append(lines, "systemctl enable --now "+name)
	}
	return lines
}

func stopLines(services []delivery.Service) []string {
	generated, plain := split(services)
	var lines []string
	if len(generated) > 0 {
		lines = append(lines, "systemctl stop "+strings.Join(generated, " "))
	}
	for _, name := range plain {
		lines = append(lines, "systemctl disable --now "+name)
	}
	return lines
}

func split(services []delivery.Service) (generated, plain []string) {
	for _, service := range services {
		if under(service.Path, siblingPath) {
			plain = append(plain, service.Name)
			continue
		}
		generated = append(generated, service.Name)
	}
	return generated, plain
}
