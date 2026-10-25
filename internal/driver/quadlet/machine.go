package quadlet

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Moq77111113/vessel/internal/oci"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

var _ driver.Driver = (*Machine)(nil)

// ErrCommand says a systemctl line did not succeed.
var ErrCommand = errors.New("command failed")

// Machine runs a quadlet delivery on podman and systemd.
type Machine struct {
	*Reader
	podman *podman
	root   string
}

// New returns the quadlet machine writing under root and driving the given command runner.
func New(root string, run Run) *Machine {
	return &Machine{Reader: NewReader(), podman: &podman{run: run}, root: root}
}

// Secrets names the secrets this machine already holds.
func (m *Machine) Secrets(ctx context.Context) ([]string, error) { return m.podman.Secrets(ctx) }

// AddSecret puts one secret on this machine.
func (m *Machine) AddSecret(ctx context.Context, secret delivery.Secret) error {
	return m.podman.CreateSecret(ctx, secret.Name, secret.Value)
}

// RemoveHint is the command an operator types to take a secret off this machine.
func (m *Machine) RemoveHint(secret string) string { return "podman secret rm " + secret }

// AddImages puts every image of the layout into the local container storage.
func (m *Machine) AddImages(ctx context.Context, work report.Report, images *oci.Images) ([]string, error) {
	return m.podman.Load(ctx, work, images)
}

// Start brings these services up, in order.
func (m *Machine) Start(ctx context.Context, services []delivery.Service) error {
	return m.do(ctx, startLines(services))
}

// Stop brings these services down, and disables the plain units among them.
func (m *Machine) Stop(ctx context.Context, services []delivery.Service) error {
	return m.do(ctx, stopLines(services))
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

// Running asks systemd which of these services are active.
func (m *Machine) Running(ctx context.Context, services []delivery.Service) ([]delivery.Service, error) {
	var running []delivery.Service
	for _, service := range services {
		// systemctl is-active exits non-zero on a down service: the text is the answer, an empty one means it never ran.
		output, err := m.podman.run(ctx, nil, "systemctl", "is-active", service.Name)
		state := strings.TrimSpace(string(output))
		if state == "" && err != nil {
			return nil, fmt.Errorf("systemctl is-active %s: %w: %s", service.Name, ErrCommand, err)
		}
		if state == "active" && !slices.Contains(running, service) {
			running = append(running, service)
		}
	}
	return running, nil
}
