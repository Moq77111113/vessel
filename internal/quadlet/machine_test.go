package quadlet_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/descriptor"
	"github.com/Moq77111113/vessel/internal/machine"
	"github.com/Moq77111113/vessel/internal/quadlet"
)

// calls records what a machine asked the system to run.
type calls struct {
	lines  []string
	answer map[string]string
	err    error
}

func (c *calls) run(_ context.Context, _ io.Reader, name string, args ...string) ([]byte, error) {
	line := strings.TrimSpace(name + " " + strings.Join(args, " "))
	c.lines = append(c.lines, line)
	return []byte(c.answer[line]), c.err
}

func units() []descriptor.File {
	return []descriptor.File{
		{Path: "etc/containers/systemd/web.container"},
		{Path: "etc/containers/systemd/db.container"},
	}
}

func TestQuadletSatisfiesTheMachinePort(t *testing.T) {
	var _ machine.Machine = quadlet.New(nil)
}

func TestStartBringsUpEveryServiceTheUnitsGenerate(t *testing.T) {
	system := &calls{}
	if err := quadlet.New(system.run).Start(context.Background(), units()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := []string{"systemctl daemon-reload", "systemctl start db.service web.service"}
	if !slices.Equal(system.lines, want) {
		t.Errorf("got %v, want %v", system.lines, want)
	}
}

func TestStartReportsACommandThatFailed(t *testing.T) {
	system := &calls{err: errors.New("exit status 1")}
	err := quadlet.New(system.run).Start(context.Background(), units())
	if !errors.Is(err, machine.ErrAction) {
		t.Errorf("got %v, want machine.ErrAction", err)
	}
}

func TestServicesReportsAServiceTheMachineDoesNotRun(t *testing.T) {
	system := &calls{answer: map[string]string{
		"systemctl is-active web.service": "active\n",
		"systemctl is-active db.service":  "failed\n",
	}}
	services, err := quadlet.New(system.run).Services(context.Background(), units())
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	want := []machine.Service{
		{Name: "db.service", Running: false},
		{Name: "web.service", Running: true},
	}
	if !slices.Equal(services, want) {
		t.Errorf("got %v, want %v", services, want)
	}
}

func TestServicesReadsTheAnswerOfAServiceThatIsDown(t *testing.T) {
	system := &calls{
		answer: map[string]string{"systemctl is-active web.service": "inactive\n"},
		err:    errors.New("exit status 3"),
	}
	services, err := quadlet.New(system.run).Services(context.Background(),
		[]descriptor.File{{Path: "etc/containers/systemd/web.container"}})
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	want := []machine.Service{{Name: "web.service", Running: false}}
	if !slices.Equal(services, want) {
		t.Errorf("got %v, want %v", services, want)
	}
}

func TestServicesFailsWhenItCannotAskSystemd(t *testing.T) {
	system := &calls{err: errors.New("exec: \"systemctl\": executable file not found in $PATH")}
	services, err := quadlet.New(system.run).Services(context.Background(), units())
	if !errors.Is(err, machine.ErrAction) {
		t.Fatalf("got %v and %v, want machine.ErrAction", services, err)
	}
}

func TestStopBringsDownEveryServiceTheUnitsGenerate(t *testing.T) {
	system := &calls{}
	if err := quadlet.New(system.run).Stop(context.Background(), units()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := []string{"systemctl stop db.service web.service"}
	if !slices.Equal(system.lines, want) {
		t.Errorf("got %v, want %v", system.lines, want)
	}
}

func TestStopAsksNothingWhenNoUnitStartsAService(t *testing.T) {
	system := &calls{}
	if err := quadlet.New(system.run).Stop(context.Background(),
		[]descriptor.File{{Path: "etc/containers/systemd/app.network"}}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(system.lines) != 0 {
		t.Errorf("got %v, want no command", system.lines)
	}
}
