package compose_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/driver/compose"
	"github.com/Moq77111113/vessel/internal/oci/layouttest"
	"github.com/Moq77111113/vessel/internal/report"
	"github.com/Moq77111113/vessel/pkg/delivery"
	"github.com/Moq77111113/vessel/pkg/driver"
)

type calls struct {
	lines  []string
	stdin  int
	answer map[string]string
	err    error
}

func (c *calls) run(_ context.Context, command compose.Command) ([]byte, error) {
	line := strings.Join(command.Line, " ")
	c.lines = append(c.lines, line)
	if command.Stdin != nil {
		io.Copy(io.Discard, command.Stdin)
		c.stdin++
	}
	return []byte(c.answer[line]), c.err
}

var stationService = delivery.Service{Name: "station", Path: "etc/compose/station/compose.yaml"}

func stationFile(root string) string { return filepath.Join(root, "etc/compose/station/compose.yaml") }

func machine(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.CopyFS(root, os.DirFS("testdata/machine")); err != nil {
		t.Fatalf("CopyFS: %v", err)
	}
	return root
}

const readyInfo = `[["driver-type","io.containerd.snapshotter.v1"]]`

func ready() map[string]string {
	return map[string]string{
		"docker version --format {{.Server.Version}}": "29.9.3\n",
		"docker compose version --short":              "2.29.7\n",
		"docker info --format {{json .DriverStatus}}": readyInfo + "\n",
	}
}

func TestStartBringsUpTheProjectWithoutPulling(t *testing.T) {
	root, docker := machine(t), &calls{}
	if err := compose.New(root, docker.run).Start(context.Background(), []delivery.Service{stationService}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	want := []string{"docker compose -f " + stationFile(root) + " up --detach --pull never --remove-orphans --wait"}
	if !slices.Equal(docker.lines, want) {
		t.Errorf("got %v, want %v", docker.lines, want)
	}
}

func TestStartReportsACommandThatFailed(t *testing.T) {
	docker := &calls{err: errors.New("exit status 1")}
	err := compose.New(machine(t), docker.run).Start(context.Background(), []delivery.Service{stationService})
	if !errors.Is(err, compose.ErrCommand) {
		t.Errorf("got %v, want compose.ErrCommand", err)
	}
}

func TestStopTakesTheProjectDownAndKeepsItsVolumes(t *testing.T) {
	docker := &calls{}
	if err := compose.New("/", docker.run).Stop(context.Background(), []delivery.Service{stationService}); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	want := []string{"docker compose -f /etc/compose/station/compose.yaml down"}
	if !slices.Equal(docker.lines, want) {
		t.Errorf("got %v, want %v", docker.lines, want)
	}
}

func running(t *testing.T, containers string) []delivery.Service {
	t.Helper()
	docker := &calls{answer: map[string]string{
		"docker compose -f /etc/compose/station/compose.yaml config --services":      "init\nsensor\nscreen\n",
		"docker compose -f /etc/compose/station/compose.yaml ps --all --format json": containers,
	}}
	services, err := compose.New("/", docker.run).Running(context.Background(), []delivery.Service{stationService})
	if err != nil {
		t.Fatalf("Running: %v", err)
	}
	return services
}

func TestRunningCountsAProjectWhoseServicesRunOrFinished(t *testing.T) {
	containers := `{"Service":"init","State":"exited","ExitCode":0}
{"Service":"sensor","State":"running","ExitCode":0}
{"Service":"screen","State":"running","ExitCode":0}
`
	if got, want := running(t, containers), []delivery.Service{stationService}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestRunningLeavesOutAProjectWithAServiceThatFailed(t *testing.T) {
	containers := `{"Service":"init","State":"exited","ExitCode":1}
{"Service":"sensor","State":"running","ExitCode":0}
{"Service":"screen","State":"running","ExitCode":0}
`
	if got := running(t, containers); len(got) != 0 {
		t.Errorf("got %v, want nothing running", got)
	}
}

func TestRunningLeavesOutAProjectWithAServiceThatHasNoContainer(t *testing.T) {
	containers := `{"Service":"init","State":"exited","ExitCode":0}
{"Service":"sensor","State":"running","ExitCode":0}
`
	if got := running(t, containers); len(got) != 0 {
		t.Errorf("got %v, want nothing running", got)
	}
}

func TestAddImagesHandsOneArchiveToDockerPerImage(t *testing.T) {
	docker := &calls{answer: map[string]string{"docker load": "Loaded image: registry.example.com/acme/web:1.0\n"}}
	images, err := compose.New("/", docker.run).AddImages(context.Background(), report.New(io.Discard), layouttest.Open(t))
	if err != nil {
		t.Fatalf("AddImages: %v", err)
	}
	if docker.stdin != 2 {
		t.Errorf("archives: got %d, want 2", docker.stdin)
	}
	if want := []string{"registry.example.com/acme/web:1.0", "registry.example.com/acme/web:1.0"}; !slices.Equal(images, want) {
		t.Errorf("got %v, want %v", images, want)
	}
}

func TestAddImagesReportsALoadThatFailed(t *testing.T) {
	docker := &calls{err: errors.New("exit status 1")}
	_, err := compose.New("/", docker.run).AddImages(context.Background(), report.New(io.Discard), layouttest.Open(t))
	if !errors.Is(err, compose.ErrCommand) {
		t.Errorf("got %v, want compose.ErrCommand", err)
	}
}

func TestSecretsListsWhatTheMachineHolds(t *testing.T) {
	secrets, err := compose.New(machine(t), (&calls{}).run).Secrets(context.Background())
	if err != nil {
		t.Fatalf("Secrets: %v", err)
	}
	if want := []string{"OLD_TOKEN"}; !slices.Equal(secrets, want) {
		t.Errorf("got %v, want %v", secrets, want)
	}
}

func TestSecretsOnABareMachineIsEmpty(t *testing.T) {
	secrets, err := compose.New(t.TempDir(), (&calls{}).run).Secrets(context.Background())
	if err != nil {
		t.Fatalf("Secrets: %v", err)
	}
	if len(secrets) != 0 {
		t.Errorf("got %v, want none", secrets)
	}
}

func TestAddSecretWritesTheFileTheProjectDeclares(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("giving the secret to uid 472 needs root, running as uid %d", os.Getuid())
	}
	root := machine(t)
	secret := delivery.Secret{Name: "SCREEN_PASSWORD", Value: "s3cret"}
	if err := compose.New(root, (&calls{}).run).AddSecret(context.Background(), secret); err != nil {
		t.Fatalf("AddSecret: %v", err)
	}
	target := filepath.Join(root, "etc/compose/.secrets/SCREEN_PASSWORD")
	info, err := os.Stat(target)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o400 {
		t.Errorf("mode: got %o, want 400", info.Mode().Perm())
	}
	data, _ := os.ReadFile(target)
	if string(data) != "s3cret" {
		t.Errorf("value: got %q, want %q", data, "s3cret")
	}
}

func TestAddSecretWithoutDeclaredOwnerLeavesItToRootAlone(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "etc/compose/plain")
	if err := os.CopyFS(project, os.DirFS("testdata/plain")); err != nil {
		t.Fatalf("CopyFS: %v", err)
	}
	if err := compose.New(root, (&calls{}).run).AddSecret(context.Background(), delivery.Secret{Name: "TOKEN", Value: "t"}); err != nil {
		t.Fatalf("AddSecret: %v", err)
	}
	info, err := os.Stat(filepath.Join(root, "etc/compose/.secrets/TOKEN"))
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode: got %o, want 600", info.Mode().Perm())
	}
}

func TestAddSecretRefusesTwoProjectsGivingItTwoOwners(t *testing.T) {
	root := machine(t)
	if err := os.CopyFS(filepath.Join(root, "etc/compose/other"), os.DirFS("testdata/other")); err != nil {
		t.Fatalf("CopyFS: %v", err)
	}
	err := compose.New(root, (&calls{}).run).AddSecret(context.Background(), delivery.Secret{Name: "SCREEN_PASSWORD", Value: "s3cret"})
	if !errors.Is(err, compose.ErrSecretOwner) {
		t.Errorf("got %v, want compose.ErrSecretOwner", err)
	}
}

func TestStartLinksTheProjectToTheSecretsTheMachineHolds(t *testing.T) {
	root := machine(t)
	if err := compose.New(root, (&calls{}).run).Start(context.Background(), []delivery.Service{stationService}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	target, err := os.Readlink(filepath.Join(root, "etc/compose/station/secrets"))
	if err != nil {
		t.Fatalf("Readlink: %v", err)
	}
	if target != "../.secrets" {
		t.Errorf("got %q, want ../.secrets", target)
	}
}

func TestStartRefusesASecretHeldWithAnotherOwner(t *testing.T) {
	if os.Getuid() != 0 {
		t.Skipf("a secret owned by root needs root, running as uid %d", os.Getuid())
	}
	root := machine(t)
	if err := os.WriteFile(filepath.Join(root, "etc/compose/.secrets/SCREEN_PASSWORD"), []byte("s3cret"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	docker := &calls{}
	err := compose.New(root, docker.run).Start(context.Background(), []delivery.Service{stationService})
	if !errors.Is(err, compose.ErrSecretOwner) {
		t.Errorf("got %v, want compose.ErrSecretOwner", err)
	}
	if len(docker.lines) != 0 {
		t.Errorf("got %v, want docker left alone", docker.lines)
	}
}

func TestRemoveHintNamesTheSecretFile(t *testing.T) {
	root := machine(t)
	hint := compose.New(root, (&calls{}).run).RemoveHint("OLD_TOKEN")
	if want := "rm " + filepath.Join(root, "etc/compose/.secrets/OLD_TOKEN"); hint != want {
		t.Errorf("got %q, want %q", hint, want)
	}
}

func TestCheckAcceptsDockerWithTheContainerdImageStore(t *testing.T) {
	root := t.TempDir()
	if err := compose.New(root, (&calls{answer: ready()}).run).Check(context.Background()); err != nil {
		t.Errorf("Check: %v", err)
	}
}

func TestCheckRefusesTheClassicImageStore(t *testing.T) {
	root := t.TempDir()
	answer := ready()
	answer["docker info --format {{json .DriverStatus}}"] = `[["Backing Filesystem","extfs"]]`
	err := compose.New(root, (&calls{answer: answer}).run).Check(context.Background())
	if !errors.Is(err, driver.ErrNotReady) || !errors.Is(err, compose.ErrClassicImageStore) {
		t.Fatalf("got %v, want driver.ErrNotReady and compose.ErrClassicImageStore", err)
	}
	if !strings.Contains(err.Error(), "containerd-snapshotter") {
		t.Errorf("got %q, want the daemon.json feature named", err.Error())
	}
}

func TestCheckRefusesAComposeTooOldToListContainersAsJSON(t *testing.T) {
	answer := ready()
	answer["docker compose version --short"] = "2.20.3\n"
	err := compose.New(t.TempDir(), (&calls{answer: answer}).run).Check(context.Background())
	if !errors.Is(err, driver.ErrNotReady) || !errors.Is(err, compose.ErrComposeTooOld) {
		t.Errorf("got %v, want driver.ErrNotReady and compose.ErrComposeTooOld", err)
	}
}

func TestCheckRefusesAMachineWithoutCompose(t *testing.T) {
	root := t.TempDir()
	docker := &calls{answer: ready()}
	failing := func(ctx context.Context, command compose.Command) ([]byte, error) {
		if strings.Join(command.Line, " ") == "docker compose version --short" {
			return []byte("docker: 'compose' is not a docker command."), errors.New("exit status 1")
		}
		return docker.run(ctx, command)
	}
	err := compose.New(root, failing).Check(context.Background())
	if !errors.Is(err, driver.ErrNotReady) || !errors.Is(err, compose.ErrComposeMissing) {
		t.Errorf("got %v, want driver.ErrNotReady and compose.ErrComposeMissing", err)
	}
}

func TestCheckRefusesAMachineWithoutDocker(t *testing.T) {
	root := t.TempDir()
	docker := &calls{err: &exec.Error{Name: "docker", Err: exec.ErrNotFound}}
	err := compose.New(root, docker.run).Check(context.Background())
	if !errors.Is(err, driver.ErrNotReady) || !errors.Is(err, compose.ErrDockerMissing) {
		t.Errorf("got %v, want driver.ErrNotReady and compose.ErrDockerMissing", err)
	}
}

func TestRestartRecreatesEveryContainerOfTheProject(t *testing.T) {
	docker := &calls{}
	root := machine(t)
	if err := compose.New(root, docker.run).Restart(context.Background(), []delivery.Service{stationService}); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	want := []string{"docker compose -f " + stationFile(root) + " up --detach --pull never --remove-orphans --force-recreate --wait"}
	if !slices.Equal(docker.lines, want) {
		t.Errorf("got %v, want %v", docker.lines, want)
	}
}

func TestAnImageDockerLoadedSurvivesAPowerCutOnceFlushed(t *testing.T) {
	docker := &powerDocker{}
	machine := compose.New("/", docker.run)
	if _, err := machine.AddImages(context.Background(), report.New(io.Discard), layouttest.Open(t)); err != nil {
		t.Fatalf("AddImages: %v", err)
	}
	if err := machine.Flush(context.Background()); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	docker.memory = 0
	if docker.disk != 2 {
		t.Errorf("got %d images on the disk after the power cut, want 2", docker.disk)
	}
}

func TestFlushReportsASyncThatFailed(t *testing.T) {
	docker := &calls{err: errors.New("exit status 1")}
	if err := compose.New("/", docker.run).Flush(context.Background()); !errors.Is(err, compose.ErrCommand) {
		t.Errorf("got %v, want compose.ErrCommand", err)
	}
}

const dockerRoot = "/var/lib/docker"

type powerDocker struct {
	memory int
	disk   int
}

func (d *powerDocker) run(_ context.Context, command compose.Command) ([]byte, error) {
	switch strings.Join(command.Line, " ") {
	case "docker load":
		d.memory++
		return []byte("Loaded image: registry.example.com/acme/web:1.0\n"), nil
	case "docker info --format {{.DockerRootDir}}":
		return []byte(dockerRoot + "\n"), nil
	case "sync --file-system " + dockerRoot:
		d.disk += d.memory
		d.memory = 0
	}
	return nil, nil
}
