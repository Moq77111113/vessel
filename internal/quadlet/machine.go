package quadlet

import (
	"context"
	"fmt"
	"strings"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/report"
)

var _ machine.Machine = (*Machine)(nil)

// Machine runs a quadlet delivery on podman and systemd.
type Machine struct {
	*Reader
	podman *podman
}

// New returns the quadlet machine driving the given command runner.
func New(run machine.Run) *Machine {
	return &Machine{Reader: NewReader(), podman: &podman{run: run}}
}

// Secrets names the secrets this machine already holds.
func (m *Machine) Secrets(ctx context.Context) ([]string, error) { return m.podman.Secrets(ctx) }

// AddSecret puts one secret on this machine.
func (m *Machine) AddSecret(ctx context.Context, name, value string) error {
	return m.podman.CreateSecret(ctx, name, value)
}

// AddImages puts every image of the layout into the local container storage.
func (m *Machine) AddImages(ctx context.Context, work report.Report, layout *machine.Layout) ([]string, error) {
	return m.podman.Load(ctx, work, layout)
}

// Start brings the services these units generate up, in order.
func (m *Machine) Start(ctx context.Context, files []descriptor.File) error {
	return m.do(ctx, m.Reader.Start(files))
}

// do runs each systemctl line, naming the one that failed.
func (m *Machine) do(ctx context.Context, lines []string) error {
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		output, err := m.podman.run(ctx, nil, fields[0], fields[1:]...)
		if err != nil {
			return fmt.Errorf("%s: %w: %s", line, machine.ErrAction, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// Services asks systemd what it does with the services these units generate.
func (m *Machine) Services(ctx context.Context, files []descriptor.File) ([]machine.Service, error) {
	names := append(serviceNames(files), siblings(files)...)
	services := make([]machine.Service, 0, len(names))
	for _, name := range names {
		// systemctl is-active exits non-zero on a down service: the text is the answer, an empty one means it never ran.
		output, err := m.podman.run(ctx, nil, "systemctl", "is-active", name)
		state := strings.TrimSpace(string(output))
		if state == "" && err != nil {
			return nil, fmt.Errorf("systemctl is-active %s: %w: %s", name, machine.ErrAction, err)
		}
		services = append(services, machine.Service{Name: name, Running: state == "active"})
	}
	return services, nil
}

// Stop brings the services these units generate down, and disables the sibling units.
func (m *Machine) Stop(ctx context.Context, files []descriptor.File) error {
	return m.do(ctx, m.Reader.Stop(files))
}
