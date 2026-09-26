package delivery

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site"
)

// plan is what an install would do to this machine, in the order it would do it.
type plan struct {
	checks   []string
	skip     []record.Step
	marks    []string
	actions  []string
	files    []change
	secrets  []string
	images   []string
	stops    []string
	starts   []string
	unknowns []string
}

// change is one file an install would add, replace, keep or remove: the verb and the line it prints.
type change struct {
	verb string
	line string
}

// plan reads the machine and lays out what run would change, touching nothing.
func (i Install) plan(ctx context.Context, before prelude, resolution site.Resolution, files []descriptor.File) (plan, error) {
	config := i.Artifact.Config
	progress := before.progress
	p := plan{checks: checks(before), skip: progress.Steps}
	if before.skip != "" {
		p.marks = []string{before.skip}
	}
	if !progress.Has(record.StepActions) {
		p.actions = config.Actions[min(len(progress.Actions), len(config.Actions)):]
	}
	unknown := dryValues(config.Variables, resolution)
	if !progress.Has(record.StepFiles) {
		changes, err := fileChanges(i.Root, files, before.current.Files, unknown)
		if err != nil {
			return plan{}, err
		}
		p.files = changes
		stops, err := i.stops(ctx, before, files)
		if err != nil {
			return plan{}, err
		}
		p.stops = stops
	}
	if !progress.Has(record.StepSecrets) {
		p.secrets = namesOf(resolution.Secrets)
	}
	if !progress.Has(record.StepImages) {
		for _, image := range config.Images {
			p.images = append(p.images, image.Ref+" "+image.Digest)
		}
	}
	services, err := before.host.Services(ctx, files)
	if err != nil {
		return plan{}, err
	}
	for _, service := range services {
		p.starts = append(p.starts, service.Name)
	}
	p.unknowns = unknowns(config.Variables, unknown, before.bypass, p.actions, p.starts)
	return p, nil
}

// checks names what the dry run confirmed before it planned anything.
func checks(before prelude) []string {
	record := "no install is left unfinished"
	switch {
	case before.progress.Root != "":
		record = "this bundle is the one the machine stopped installing"
	case before.found && !before.current.Done():
		record = "this bundle is the last finished release"
	}
	return []string{"the machine is ready", "the bundle matches its digests", "every variable has a value", record}
}

// stops names the services of the files the previous version carried and this one drops.
func (i Install) stops(ctx context.Context, before prelude, files []descriptor.File) ([]string, error) {
	if !before.found {
		return nil, nil
	}
	remove, _, err := drops(machine.NewTree(i.Root), before.current.Files, entriesOf(files))
	if err != nil {
		return nil, err
	}
	services, err := before.host.Services(ctx, units(remove))
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names, nil
}

// dryValues maps the value a dry run put in place of each from: command to the variable it answers.
func dryValues(variables []descriptor.Variable, resolution site.Resolution) map[string]string {
	marks := map[string]string{}
	for _, variable := range variables {
		value, ok := resolution.Values[variable.Name]
		if !ok {
			value = resolution.Secrets[variable.Name]
		}
		if strings.HasPrefix(value, machine.DryMark) {
			marks[value] = variable.Name
		}
	}
	return marks
}

// fileChanges compares every file the install carries with the disk, then names the files it would remove or keep.
func fileChanges(root string, files []descriptor.File, current []record.Entry, unknown map[string]string) ([]change, error) {
	changes := make([]change, 0, len(files))
	for _, file := range files {
		changes = append(changes, fileChange(filepath.Join(root, file.Path), file, unknown))
	}
	remove, keep, err := drops(machine.NewTree(root), current, entriesOf(files))
	if err != nil {
		return nil, err
	}
	for _, path := range remove {
		changes = append(changes, change{verb: "Remove", line: path})
	}
	for _, path := range keep {
		changes = append(changes, change{verb: "Keep", line: path + ", edited on this machine"})
	}
	return changes, nil
}

// fileChange says what writing file at path would do, naming the from: values its content waits on.
func fileChange(path string, file descriptor.File, unknown map[string]string) change {
	var names []string
	for mark, name := range unknown {
		if bytes.Contains(file.Data, []byte(mark)) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	_, err := os.Stat(path)
	absent := os.IsNotExist(err)
	switch {
	case len(names) > 0 && absent:
		return change{verb: "Add", line: file.Path + ", content depends on " + strings.Join(names, ", ")}
	case len(names) > 0:
		return change{verb: "Depends", line: file.Path + " on " + strings.Join(names, ", ")}
	case absent:
		return change{verb: "Add", line: file.Path}
	case machine.Same(path, machine.DigestOf(file.Data)):
		return change{verb: "Keep", line: file.Path}
	}
	return change{verb: "Replace", line: file.Path}
}

// unknowns names what a dry run cannot know: from: values, an action a rollback passes, action results, service health.
func unknowns(variables []descriptor.Variable, unknown map[string]string, bypass string, actions, services []string) []string {
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
	if len(actions) > 0 {
		lines = append(lines, "the result of every action")
	}
	if len(services) > 0 {
		lines = append(lines, "service health, known once started")
	}
	return lines
}

// print writes the plan, one verb per line.
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
