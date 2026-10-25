package quadlet_test

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/driver/quadlet"
	"github.com/Moq77111113/vessel/pkg/delivery"
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

var (
	db     = delivery.Service{Name: "db.service", Path: "etc/containers/systemd/db.container"}
	web    = delivery.Service{Name: "web.service", Path: "etc/containers/systemd/web.container"}
	backup = delivery.Service{Name: "backup.timer", Path: "etc/systemd/system/backup.timer"}
)

func units() []delivery.Service { return []delivery.Service{db, web} }

func TestStartBringsUpEveryServiceTheUnitsGenerate(t *testing.T) {
	system := &calls{}
	if err := quadlet.New("/", system.run).Start(context.Background(), units()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := []string{"systemctl daemon-reload", "systemctl start db.service web.service"}
	if !slices.Equal(system.lines, want) {
		t.Errorf("got %v, want %v", system.lines, want)
	}
}

func TestStartReportsACommandThatFailed(t *testing.T) {
	system := &calls{err: errors.New("exit status 1")}
	err := quadlet.New("/", system.run).Start(context.Background(), units())
	if !errors.Is(err, quadlet.ErrCommand) {
		t.Errorf("got %v, want quadlet.ErrCommand", err)
	}
}

func TestRunningLeavesOutAServiceTheMachineDoesNotRun(t *testing.T) {
	system := &calls{answer: map[string]string{
		"systemctl is-active web.service": "active\n",
		"systemctl is-active db.service":  "failed\n",
	}}
	services, err := quadlet.New("/", system.run).Running(context.Background(), units())
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	want := []delivery.Service{web}
	if !slices.Equal(services, want) {
		t.Errorf("got %v, want %v", services, want)
	}
}

func TestRunningReadsTheAnswerOfAServiceThatIsDown(t *testing.T) {
	system := &calls{
		answer: map[string]string{"systemctl is-active web.service": "inactive\n"},
		err:    errors.New("exit status 3"),
	}
	services, err := quadlet.New("/", system.run).Running(context.Background(), []delivery.Service{web})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	if len(services) != 0 {
		t.Errorf("got %v, want nothing running", services)
	}
}

func TestRunningFailsWhenItCannotAskSystemd(t *testing.T) {
	system := &calls{err: errors.New("exec: \"systemctl\": executable file not found in $PATH")}
	services, err := quadlet.New("/", system.run).Running(context.Background(), units())
	if !errors.Is(err, quadlet.ErrCommand) {
		t.Fatalf("got %v and %v, want quadlet.ErrCommand", services, err)
	}
}

func TestStopBringsDownEveryServiceTheUnitsGenerate(t *testing.T) {
	system := &calls{}
	if err := quadlet.New("/", system.run).Stop(context.Background(), units()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := []string{"systemctl stop db.service web.service"}
	if !slices.Equal(system.lines, want) {
		t.Errorf("got %v, want %v", system.lines, want)
	}
}

func TestStopAsksNothingWhenThereIsNoService(t *testing.T) {
	system := &calls{}
	if err := quadlet.New("/", system.run).Stop(context.Background(), nil); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if len(system.lines) != 0 {
		t.Errorf("got %v, want no command", system.lines)
	}
}

func TestStopDisablesATimerTheDeliveryCarries(t *testing.T) {
	system := &calls{}
	if err := quadlet.New("/", system.run).Stop(context.Background(), []delivery.Service{web, backup}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := []string{"systemctl stop web.service", "systemctl disable --now backup.timer"}
	if !slices.Equal(system.lines, want) {
		t.Errorf("got %v, want %v", system.lines, want)
	}
}

func TestRunningAsksAboutATimerTheDeliveryCarries(t *testing.T) {
	system := &calls{answer: map[string]string{"systemctl is-active backup.timer": "active\n"}}
	services, err := quadlet.New("/", system.run).Running(context.Background(), []delivery.Service{backup})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	want := []delivery.Service{backup}
	if !slices.Equal(services, want) {
		t.Errorf("got %v, want %v", services, want)
	}
}
