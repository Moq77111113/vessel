package site

import (
	"fmt"
	"io"
	"strings"

	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site/disk"
	"github.com/Moq77111113/vessel/internal/site/release"
	"github.com/Moq77111113/vessel/internal/site/values"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

type plan struct {
	checks   []string
	skip     []release.Step
	marks    []string
	actions  []string
	files    []change
	secrets  []string
	images   []string
	stops    []string
	starts   []string
	unknowns []string
}

type change struct {
	verb string
	line string
}

func (i Install) dryRun(req InstallRequest, c *pass) error {
	p, err := i.plan(req, c)
	if err != nil {
		return err
	}
	p.print(req.Out, req.Artifact.Config.Name, req.Artifact.Config.Version)
	return nil
}

func (i Install) plan(req InstallRequest, c *pass) (plan, error) {
	config := req.Artifact.Config
	before, resolution, files := c.before, c.resolution, c.files
	progress := before.progress
	p := plan{checks: checks(before), skip: progress.Steps}
	if before.skip != "" {
		p.marks = []string{before.skip}
	}
	if !progress.Has(release.StepActions) {
		p.actions = config.Actions[min(len(progress.Actions), len(config.Actions)):]
	}
	unknown := dryValues(config.Variables, resolution)
	if !progress.Has(release.StepFiles) {
		changes, err := i.fileChanges(files, before.current.Files, unknown)
		if err != nil {
			return plan{}, err
		}
		p.files = changes
		stops, err := i.stops(before, files)
		if err != nil {
			return plan{}, err
		}
		p.stops = stops
	}
	if !progress.Has(release.StepSecrets) {
		p.secrets = secretNames(resolution.Secrets)
	}
	if !progress.Has(release.StepImages) {
		for _, image := range config.Images {
			p.images = append(p.images, image.Ref+" "+image.Digest)
		}
	}
	for _, service := range config.Services {
		p.starts = append(p.starts, service.Name)
	}
	p.unknowns = p.unknownLines(config.Variables, unknown, before.bypass)
	return p, nil
}

func checks(before state) []string {
	record := "no install is left unfinished"
	switch {
	case before.progress.Root != "":
		record = "this bundle is the one the machine stopped installing"
	case before.exists && !before.current.Done():
		record = "this bundle is the last finished release"
	}
	return []string{"the machine is ready", "the bundle matches its digests", "every variable has a value", record}
}

func (i Install) stops(before state, files []delivery.File) ([]string, error) {
	if !before.exists {
		return nil, nil
	}
	drop, err := release.Drops(disk.NewTree(i.Root), before.current.Files, release.Entries(files))
	if err != nil {
		return nil, err
	}
	remove := drop.Remove
	services := servicesIn(before.current.Services, remove)
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names, nil
}

func dryValues(variables []delivery.Variable, resolution values.Resolution) map[string]string {
	marks := map[string]string{}
	for _, variable := range variables {
		value, ok := resolution.Values[variable.Name]
		if !ok {
			value = resolution.Secrets[variable.Name]
		}
		if strings.HasPrefix(value, disk.DryMark) {
			marks[value] = variable.Name
		}
	}
	return marks
}

func (p plan) unknownLines(variables []delivery.Variable, unknown map[string]string, bypass string) []string {
	var lines []string
	for _, variable := range variables {
		for _, name := range unknown {
			if name == variable.Name {
				lines = append(lines, fmt.Sprintf("%s from %s, not run", variable.Name, variable.From))
			}
		}
	}
	if bypass != "" {
		lines = append(lines, bypass+" may have stopped halfway, this rollback passes over it")
	}
	if len(p.actions) > 0 {
		lines = append(lines, "the result of every action")
	}
	if len(p.starts) > 0 {
		lines = append(lines, "service health, known once started")
	}
	return lines
}

func (p plan) print(out io.Writer, name, version string) {
	fmt.Fprintf(out, "%s %s, dry run: nothing on this machine changed\n", name, version)
	lines := report.New(out)
	for _, check := range p.checks {
		lines.Line("Checked", check)
	}
	for _, step := range p.skip {
		lines.Line("Skip", string(step)+", the last run finished it")
	}
	for _, command := range p.marks {
		lines.Line("Skip", command+", marked finished without running it")
	}
	for _, action := range p.actions {
		lines.Line("Run", action)
	}
	for _, file := range p.files {
		lines.Line(file.verb, file.line)
	}
	for _, service := range p.stops {
		lines.Line("Stop", service)
	}
	for _, name := range p.secrets {
		lines.Line("Create", "secret "+name)
	}
	for _, image := range p.images {
		lines.Line("Load", image)
	}
	for _, service := range p.starts {
		lines.Line("Start", service)
	}
	for _, line := range p.unknowns {
		lines.Line("Unverified", line)
	}
}
