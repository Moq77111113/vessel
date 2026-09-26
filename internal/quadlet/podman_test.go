package quadlet

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/Moq77111113/vessel/internal/machine/layouttest"
	"github.com/Moq77111113/vessel/internal/report"
)

// orderRecorder stands in for a report.Report and remembers, in order, when a line was
// written relative to the calls a test also logs into it.
type orderRecorder struct{ log *[]string }

func (o *orderRecorder) Line(_, _ string) { *o.log = append(*o.log, "line") }

// recorder stands in for podman and remembers what it was handed.
type recorder struct {
	args      []string
	stdin     [][]byte
	output    string
	err       error
	failAfter int
	calls     int
}

func (r *recorder) run(_ context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	if stdin != nil {
		body, _ := io.ReadAll(stdin)
		r.stdin = append(r.stdin, body)
	}
	r.calls++
	if r.failAfter > 0 && r.calls > r.failAfter {
		return []byte("Error: Cgroups v1 not supported"), errors.New("exit status 125")
	}
	return []byte(r.output), r.err
}

func TestLoadHandsOneArchiveToPodmanPerImage(t *testing.T) {
	engine := &recorder{output: "Loaded image: registry.example.com/acme/web:1.0\n"}
	if _, err := (&podman{run: engine.run}).Load(context.Background(), report.New(io.Discard), layouttest.Open(t)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := strings.Join(engine.args, " "), "podman load"; got != want {
		t.Errorf("args: got %q, want %q", got, want)
	}
	if got, want := len(engine.stdin), 2; got != want {
		t.Errorf("archives: got %d, want %d", got, want)
	}
}

func TestLoadReturnsTheImagesPodmanNamed(t *testing.T) {
	engine := &recorder{output: "Loaded image: registry.example.com/acme/web:1.0\n"}
	images, err := (&podman{run: engine.run}).Load(context.Background(), report.New(io.Discard), layouttest.Open(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got, want := len(images), 2; got != want {
		t.Errorf("images: got %d, want %d", got, want)
	}
}

func TestLoadCarriesThePodmanOutputIntoItsError(t *testing.T) {
	engine := &recorder{output: "Error: payload does not match", err: errors.New("exit status 125")}
	_, err := (&podman{run: engine.run}).Load(context.Background(), report.New(io.Discard), layouttest.Open(t))
	if err == nil {
		t.Fatal("Load: want an error, got nil")
	}
	if !strings.Contains(err.Error(), "payload does not match") {
		t.Errorf("error drops the podman output: %v", err)
	}
}

func TestLoadNamesTheImageInItsAnnouncement(t *testing.T) {
	engine := &recorder{output: "Loaded image: registry.example.com/acme/web:1.0\n"}
	var progress strings.Builder
	if _, err := (&podman{run: engine.run}).Load(context.Background(), report.New(&progress), layouttest.Open(t)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, name := range []string{"registry.example.com/acme/web:1.0", "registry.example.com/library/postgres:17.2"} {
		if !strings.Contains(progress.String(), name) {
			t.Errorf("progress does not name %s: %s", name, progress.String())
		}
	}
}

// install runs on a client site with no network for minutes at a time; if nothing prints
// until podman load returns, the operator cannot tell it apart from a dead process.
func TestLoadAnnouncesEachImageBeforePodmanLoadRuns(t *testing.T) {
	var log []string
	run := func(_ context.Context, _ io.Reader, _ string, _ ...string) ([]byte, error) {
		log = append(log, "run")
		return []byte("Loaded image: registry.example.com/acme/web:1.0\n"), nil
	}
	work := &orderRecorder{log: &log}
	if _, err := (&podman{run: run}).Load(context.Background(), work, layouttest.Open(t)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []string{"line", "run", "line", "run"}
	if !slices.Equal(log, want) {
		t.Errorf("got %v, want %v: an announcement before each podman load runs", log, want)
	}
}

func TestVersionAsksPodmanForItsVersion(t *testing.T) {
	engine := &recorder{output: "podman version 5.4.2\n"}
	version, err := (&podman{run: engine.run}).Version(context.Background())
	if err != nil {
		t.Fatalf("Version: %v", err)
	}
	if got, want := version, "5.4.2"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVersionReportsPodmanMissing(t *testing.T) {
	engine := &recorder{err: errors.New(`exec: "podman": executable file not found in $PATH`)}
	if _, err := (&podman{run: engine.run}).Version(context.Background()); !errors.Is(err, ErrPodmanMissing) {
		t.Errorf("got %v, want ErrPodmanMissing", err)
	}
}

func TestCreateSecretHandsTheValueOnStandardInput(t *testing.T) {
	var got []string
	var body []byte
	engine := &podman{run: func(ctx context.Context, stdin io.Reader, name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		body, _ = io.ReadAll(stdin)
		return nil, nil
	}}
	if err := engine.CreateSecret(context.Background(), "db-password", "a3f9"); err != nil {
		t.Fatalf("CreateSecret: %v", err)
	}
	want := []string{"podman", "secret", "create", "db-password", "-"}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if string(body) != "a3f9" {
		t.Errorf("got %q on stdin, want %q", body, "a3f9")
	}
}

func TestLoadHandsPodmanTheArchiveAsAFileOnDisk(t *testing.T) {
	var stdin io.Reader
	run := func(_ context.Context, in io.Reader, _ string, _ ...string) ([]byte, error) {
		stdin = in
		return []byte("Loaded image: registry.example.com/acme/web:1.0\n"), nil
	}
	if _, err := (&podman{run: run}).Load(context.Background(), report.New(io.Discard), layouttest.Open(t)); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := stdin.(*os.File); !ok {
		t.Errorf("got %T, want the archive handed as a file", stdin)
	}
}
