package delivery

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Moq77111113/vessel/internal/bundle"
	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/record"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/internal/site"
)

// Install is one run of putting a bundle on one machine.
type Install struct {
	Artifact *bundle.Bundle
	Root     string
	Set      map[string]string
	Machines []machine.Machine
	Shell    *machine.Shell
	preview  bool
}

// shell is the shell a from: runs in: one that runs nothing on a dry run.
func (i Install) shell() *machine.Shell {
	if i.preview {
		return machine.NewShell(machine.Dry)
	}
	return i.Shell
}

// Preview returns this install as a dry run: it checks and plans, runs no from:, and changes nothing.
func (i Install) Preview() Install {
	i.preview = true
	return i
}

// prelude carries what both Run and Upgrade need before either touches the machine.
type prelude struct {
	dir      string
	host     machine.Machine
	records  *record.Records
	current  record.Record
	found    bool
	progress record.Record
	bypass   string
	skip     string
}

// prepare checks the machine is ready, then reads the record this delivery may already hold.
func (i Install) prepare(ctx context.Context, name, machineName string) (prelude, error) {
	dir, err := dirFor(i.Root, name)
	if err != nil {
		return prelude{}, err
	}
	host, err := i.checkMachine(ctx, machineName)
	if err != nil {
		return prelude{}, err
	}
	records := record.NewRecords(dir)
	current, found, err := records.Read()
	if err != nil {
		return prelude{}, err
	}
	return prelude{dir: dir, host: host, records: records, current: current, found: found}, nil
}

// Upgrade installs a newer version, refusing a machine that holds no record of this delivery.
func (i Install) Upgrade(ctx context.Context, out io.Writer, work report.Report) error {
	config := i.Artifact.Config
	before, err := i.prepare(ctx, config.Name, config.Machine)
	if err != nil {
		return err
	}
	if !before.found {
		return fmt.Errorf("%s: %w, run install instead", config.Name, ErrNoRecord)
	}
	if err := i.refuseOpenRecord(before); err != nil {
		return err
	}
	before.bypass = bypass(before.current)
	return i.run(ctx, out, work, before)
}

// Run puts the images, the files and the secrets on the machine, then starts the services once.
func (i Install) Run(ctx context.Context, out io.Writer, work report.Report) error {
	config := i.Artifact.Config
	before, err := i.prepare(ctx, config.Name, config.Machine)
	if err != nil {
		return err
	}
	if err := i.refuseOpenRecord(before); err != nil {
		return err
	}
	before.bypass = bypass(before.current)
	return i.run(ctx, out, work, before)
}

// Errors Resume returns before it changes anything.
var (
	ErrNothingToResume = errors.New("this machine holds no unfinished install of that delivery")
	ErrOtherBundle     = errors.New("this bundle is not the one the machine stopped installing")
	ErrActionUnknown   = errors.New("an action may have stopped halfway, check it by hand")
	ErrSetAfterValues  = errors.New("the unfinished install already stored its values, resume without --set")
	ErrRecordTooOld    = errors.New("an older vessel left this install unfinished, it cannot say how far it ran")
)

// Resume finishes the install this bundle started, skipping every step and action the record says finished.
func (i Install) Resume(ctx context.Context, out io.Writer, work report.Report) error {
	config := i.Artifact.Config
	before, err := i.prepare(ctx, config.Name, config.Machine)
	if err != nil {
		return err
	}
	if err := i.checkResume(before); err != nil {
		return err
	}
	if action, open := before.current.OpenAction(); open {
		return fmt.Errorf("%q: %w, then resume --skip-action", action.Command, ErrActionUnknown)
	}
	before.progress = before.current
	return i.run(ctx, out, work, before)
}

var ErrNoOpenAction = errors.New("the unfinished install left no action halfway, run resume")

// SkipAction marks the action an install left halfway as finished, without running it, then resumes.
func (i Install) SkipAction(ctx context.Context, out io.Writer, work report.Report) error {
	config := i.Artifact.Config
	before, err := i.prepare(ctx, config.Name, config.Machine)
	if err != nil {
		return err
	}
	if err := i.checkResume(before); err != nil {
		return err
	}
	if _, open := before.current.OpenAction(); !open {
		return fmt.Errorf("%s: %w", config.Name, ErrNoOpenAction)
	}
	before.progress = before.current
	before.progress.Actions = slices.Clone(before.current.Actions)
	before.progress.Actions[len(before.progress.Actions)-1].End = time.Now().UTC()
	before.skip = before.progress.Actions[len(before.progress.Actions)-1].Command
	return i.run(ctx, out, work, before)
}

// checkResume refuses a resume the record cannot carry through: nothing open, another bundle, an older format, a late --set.
func (i Install) checkResume(before prelude) error {
	if !before.found || before.current.Done() {
		return fmt.Errorf("%s: %w", i.Artifact.Config.Name, ErrNothingToResume)
	}
	if before.current.Root != i.Artifact.Root {
		return fmt.Errorf("the machine stopped installing %s %s: %w",
			before.current.Name, before.current.Version, ErrOtherBundle)
	}
	if before.current.Format < record.Format {
		return fmt.Errorf("%s %s, run uninstall: %w", before.current.Name, before.current.Version, ErrRecordTooOld)
	}
	if !before.current.Has(record.StepValues) {
		return nil
	}
	for _, variable := range i.Artifact.Config.Variables {
		if _, ok := i.Set[variable.Name]; ok && !variable.Secret {
			return fmt.Errorf("--set %s: %w", variable.Name, ErrSetAfterValues)
		}
	}
	return nil
}

var ErrRecordOpen = errors.New("the last install on this machine never finished")

// refuseOpenRecord lets an install over an open record through only when it puts the last finished release back.
func (i Install) refuseOpenRecord(before prelude) error {
	if !before.found || before.current.Done() {
		return nil
	}
	if before.current.Prior.Root == i.Artifact.Root {
		return nil
	}
	return fmt.Errorf("%s %s %s, %s: %w", before.current.Name, before.current.Version,
		position(before.current), recovery(before.current.Prior), ErrRecordOpen)
}

// bypass names the action a rollback passes over without knowing how far it ran, or nothing.
func bypass(current record.Record) string {
	if action, open := current.OpenAction(); open {
		return action.Command
	}
	return ""
}

// prior names the last finished release an install opening now would fall back to.
func prior(before prelude) record.Release {
	if before.found && before.current.Done() {
		return record.Release{Version: before.current.Version, Root: before.current.Root}
	}
	return before.current.Prior
}

// recovery names what an operator runs on a machine an install left unfinished.
func recovery(prior record.Release) string {
	if prior.Root == "" {
		return "run resume, or uninstall"
	}
	return fmt.Sprintf("run resume, or install with the %s installer", prior.Version)
}

// position says where an open record stopped, in words an operator reads.
func position(r record.Record) string {
	if len(r.Steps) == 0 {
		return "stopped before its first step"
	}
	return "stopped after " + string(r.Steps[len(r.Steps)-1])
}

// run installs or upgrades once the machine is known ready and the record is known read.
func (i Install) run(ctx context.Context, out io.Writer, work report.Report, before prelude) error {
	config := i.Artifact.Config
	secrets, err := before.host.Secrets(ctx)
	if err != nil {
		return err
	}
	store, resolution, err := i.resolveValues(ctx, before.dir, config, secrets)
	if err != nil {
		return err
	}
	files, err := descriptor.Substitute(i.Artifact.Files, resolution.Values,
		descriptor.SecretNames(config.Variables))
	if err != nil {
		return err
	}
	if i.preview {
		p, err := i.plan(ctx, before, resolution, files)
		if err != nil {
			return err
		}
		p.print(out, config.Name, config.Version)
		return nil
	}
	if before.bypass != "" {
		fmt.Fprintf(out, "rolling back over an action that may have stopped halfway, check it by hand: %s\n", before.bypass)
	}

	next := record.Record{
		Name:     config.Name,
		Version:  config.Version,
		Machine:  config.Machine,
		Platform: config.Platform,
		Root:     i.Artifact.Root,
		Files:    entriesOf(files),
		Images:   digestsOf(config.Images),
		Secrets:  unionNames(before.current.Secrets, namesOf(resolution.Secrets)),
		Insecure: config.Insecure,
		Prior:    prior(before),
		Start:    time.Now().UTC(),
	}
	opening, err := openRecord(before.records, before.current, next, before.progress)
	if err != nil {
		return err
	}
	changes, err := i.applyToMachine(ctx, out, work, &machineChange{
		host:       before.host,
		records:    before.records,
		current:    before.current,
		opening:    opening,
		next:       next,
		found:      before.found,
		store:      store,
		resolution: resolution,
		secrets:    secrets,
		files:      files,
	})
	if err != nil {
		return partway(err)
	}

	summary := fmt.Sprintf("%d of %d files changed", changes, len(files))
	if before.progress.Has(record.StepFiles) {
		summary = fmt.Sprintf("%d files already written", len(files))
	}
	report.New(out).Line("Finished", fmt.Sprintf("%s %s installed and running: %d images, %s",
		config.Name, config.Version, len(config.Images), summary))
	if config.Insecure {
		fmt.Fprintln(out, bundle.InsecureWarning)
	}
	return nil
}

// machineChange carries what applyToMachine needs, once the record has marked the machine changed.
type machineChange struct {
	host       machine.Machine
	records    *record.Records
	current    record.Record
	opening    record.Record
	next       record.Record
	found      bool
	store      *site.Store
	resolution site.Resolution
	secrets    []string
	files      []descriptor.File
}

// step runs do unless the record already has step, then writes step into the record.
func (c *machineChange) step(step record.Step, do func() error) error {
	if c.opening.Has(step) {
		return nil
	}
	if err := do(); err != nil {
		return err
	}
	c.opening.Steps = append(c.opening.Steps, step)
	return c.records.Write(c.opening)
}

// close writes the finished record: the new version's files, every step, every action, an end.
func (c *machineChange) close() error {
	c.next.Steps = c.opening.Steps
	c.next.Actions = c.opening.Actions
	c.next.End = time.Now().UTC()
	return c.records.Write(c.next)
}

// applyToMachine runs every install step in order, writing each into the record, then closes it.
func (i Install) applyToMachine(ctx context.Context, out io.Writer, work report.Report,
	change *machineChange) (int, error) {
	tree := machine.NewTree(i.Root)
	var changes int

	if err := change.step(record.StepValues, func() error {
		return change.store.Write(change.resolution.Values)
	}); err != nil {
		return 0, err
	}
	if err := change.step(record.StepActions, func() error {
		return i.runActions(ctx, change)
	}); err != nil {
		return 0, err
	}
	if err := change.step(record.StepFiles, func() error {
		var err error
		if changes, err = writeFiles(tree, change.files); err != nil {
			return err
		}
		if !change.found {
			return nil
		}
		return removeGone(ctx, work, change.host, tree, change.current.Files, change.next.Files)
	}); err != nil {
		return 0, err
	}
	if err := change.step(record.StepSecrets, func() error {
		return addSecrets(ctx, change.host, change.resolution.Secrets)
	}); err != nil {
		return 0, err
	}
	if err := change.step(record.StepImages, func() error {
		_, err := addImages(ctx, work, change.host, i.Artifact.LayoutDir)
		return err
	}); err != nil {
		return 0, err
	}
	if err := change.step(record.StepServices, func() error {
		reportMissingSecrets(out, change.secrets, change.host.Requires(change.files))
		return startServices(ctx, change.host, change.files)
	}); err != nil {
		return 0, err
	}
	return changes, change.close()
}

// checkMachine finds the machine that built this bundle and refuses one that is not ready.
func (i Install) checkMachine(ctx context.Context, name string) (machine.Machine, error) {
	host, err := machine.ByName(i.Machines, name)
	if err != nil {
		return nil, err
	}
	if err := host.Check(ctx, i.Root); err != nil {
		return nil, err
	}
	return host, nil
}

// resolveValues answers every variable the delivery declares, from this machine and this operator.
func (i Install) resolveValues(ctx context.Context, dir string, config bundle.Config,
	secrets []string) (*site.Store, site.Resolution, error) {
	store := site.NewStore(dir)
	resolution, err := site.NewValues(store, i.shell(), i.Set, secrets).Resolve(ctx, config.Variables)
	if err != nil {
		return nil, site.Resolution{}, err
	}
	return store, resolution, nil
}

// openRecord writes the record this install opens: the files both versions carry, and any progress it continues.
func openRecord(records *record.Records, current, next, progress record.Record) (record.Record, error) {
	opening := next
	opening.Files = union(current.Files, next.Files)
	opening.Steps = progress.Steps
	opening.Actions = progress.Actions
	return opening, records.Write(opening)
}

// runActions runs the declared commands in order, writing each into the record before and after.
func (i Install) runActions(ctx context.Context, change *machineChange) error {
	for index, command := range i.Artifact.Config.Actions {
		if index < len(change.opening.Actions) {
			continue
		}
		change.opening.Actions = append(change.opening.Actions, record.Action{Command: command})
		if err := change.records.Write(change.opening); err != nil {
			return err
		}
		last := len(change.opening.Actions) - 1
		err := i.Shell.Do(ctx, command)
		if errors.Is(err, machine.ErrKilled) {
			return err
		}
		if err != nil {
			change.opening.Actions = change.opening.Actions[:last]
			return errors.Join(err, change.records.Write(change.opening))
		}
		change.opening.Actions[last].End = time.Now().UTC()
		if err := change.records.Write(change.opening); err != nil {
			return err
		}
	}
	return nil
}

// writeFiles puts every file under the target root and counts the ones whose content changed.
func writeFiles(tree *machine.Tree, files []descriptor.File) (int, error) {
	changes := 0
	for _, file := range files {
		wrote, err := tree.Write(file)
		if err != nil {
			return changes, err
		}
		if wrote {
			changes++
		}
	}
	return changes, nil
}

// addSecrets puts every secret this run resolved into the machine's own secret store.
func addSecrets(ctx context.Context, host machine.Host, secrets map[string]string) error {
	for name, value := range secrets {
		if err := host.AddSecret(ctx, name, value); err != nil {
			return err
		}
	}
	return nil
}

// addImages puts the images the bundle carries into local storage, and names them.
func addImages(ctx context.Context, work report.Report, host machine.Host, dir string) ([]string, error) {
	layout, err := machine.OpenLayout(dir)
	if err != nil {
		return nil, err
	}
	return host.AddImages(ctx, work, layout)
}

// startServices starts the services the units generate and fails on any that did not come up.
func startServices(ctx context.Context, host machine.Host, files []descriptor.File) error {
	if err := host.Start(ctx, files); err != nil {
		return err
	}
	return checkServicesUp(ctx, host, files)
}

// ErrPartlyInstalled says the machine was already changed when this install stopped.
var ErrPartlyInstalled = errors.New("this machine is partway through the install")

// partway marks an error the machine was already changed for, and never marks it twice.
func partway(err error) error {
	if errors.Is(err, ErrPartlyInstalled) {
		return err
	}
	return fmt.Errorf("%w: %w", ErrPartlyInstalled, err)
}

var ErrServiceIsDown = errors.New("did not start")

// checkServicesUp fails naming every service the machine reports as down.
func checkServicesUp(ctx context.Context, host machine.Host, files []descriptor.File) error {
	services, err := host.Services(ctx, files)
	if err != nil {
		return err
	}
	var down []string
	for _, service := range services {
		if !service.Running {
			down = append(down, service.Name)
		}
	}
	if len(down) == 0 {
		return nil
	}
	return fmt.Errorf("%s: %w", strings.Join(down, ", "), ErrServiceIsDown)
}

// reportMissingSecrets names the secrets the units expect and the machine does not hold.
func reportMissingSecrets(out io.Writer, secrets, units []string) {
	if len(units) == 0 {
		return
	}
	present := make(map[string]bool, len(secrets))
	for _, name := range secrets {
		present[name] = true
	}
	var missing []string
	for _, name := range units {
		if !present[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return
	}
	fmt.Fprintf(out, "\nThese secrets are not on this machine yet, the units need them:\n")
	for _, name := range missing {
		fmt.Fprintf(out, "  podman secret create %s <file>\n", name)
	}
}

// Errors a --set flag draws before an install looks at a bundle.
var (
	ErrBadSet     = errors.New("is not NAME=value")
	ErrSetIsEmpty = errors.New("gives no value, and a value is never empty")
)

// ParseSet turns a repeated --set NAME=value flag into the values it names.
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
