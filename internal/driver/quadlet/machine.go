package quadlet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var _ driver.Machine = (*Machine)(nil)

// ErrCommand says a systemctl line did not succeed.
var ErrCommand = errors.New("command failed")

// Machine runs a quadlet delivery on podman and systemd.
type Machine struct {
	*Reader
	podman *podman
}

// New returns the quadlet machine driving the given command runner.
func New(run Run) *Machine {
	return &Machine{Reader: NewReader(), podman: &podman{run: run}}
}

// Secrets names the secrets this machine already holds.
func (m *Machine) Secrets(ctx context.Context) ([]string, error) { return m.podman.Secrets(ctx) }

// AddSecret puts one secret on this machine.
func (m *Machine) AddSecret(ctx context.Context, name, value string) error {
	return m.podman.CreateSecret(ctx, name, value)
}

// AddImages puts every image of the layout into the local container storage.
func (m *Machine) AddImages(ctx context.Context, work report.Report, layout *oci.Layout) ([]string, error) {
	return m.podman.Load(ctx, work, layout)
}

// Start brings the services these units generate up, in order.
func (m *Machine) Start(ctx context.Context, files []delivery.File) error {
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
			return fmt.Errorf("%s: %w: %s", line, ErrCommand, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

// Services asks systemd what it does with the services these units generate.
func (m *Machine) Services(ctx context.Context, files []delivery.File) ([]driver.Service, error) {
	names := append(serviceNames(files), siblings(files)...)
	services := make([]driver.Service, 0, len(names))
	for _, name := range names {
		// systemctl is-active exits non-zero on a down service: the text is the answer, an empty one means it never ran.
		output, err := m.podman.run(ctx, nil, "systemctl", "is-active", name)
		state := strings.TrimSpace(string(output))
		if state == "" && err != nil {
			return nil, fmt.Errorf("systemctl is-active %s: %w: %s", name, ErrCommand, err)
		}
		services = append(services, driver.Service{Name: name, Running: state == "active"})
	}
	return services, nil
}

// Stop brings the services these units generate down, and disables the sibling units.
func (m *Machine) Stop(ctx context.Context, files []delivery.File) error {
	return m.do(ctx, m.Reader.Stop(files))
}
