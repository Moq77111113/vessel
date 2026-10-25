package disk

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestValueTakesTheCommandOutputWithoutItsNewline(t *testing.T) {
	shell := NewShell(func(_ context.Context, _ string, stdout, _ io.Writer) error {
		_, err := io.WriteString(stdout, "a3f9\n")
		return err
	})
	got, err := shell.Value(context.Background(), "openssl rand -hex 2")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "a3f9" {
		t.Errorf("got %q, want %q", got, "a3f9")
	}
}

func TestStreamNamesTheCommandThatFailed(t *testing.T) {
	shell := NewShell(func(_ context.Context, _ string, _, stderr io.Writer) error {
		io.WriteString(stderr, "permission denied")
		return errors.New("exit status 1")
	})
	err := shell.Stream(context.Background(), "mkdir /etc/acme", io.Discard)
	if !errors.Is(err, ErrAction) {
		t.Fatalf("got %v, want ErrAction", err)
	}
	if got := err.Error(); !strings.Contains(got, "mkdir /etc/acme") {
		t.Errorf("got %q, want it to name the command", got)
	}
}

func TestShRunsARealCommand(t *testing.T) {
	got, err := NewShell(Sh).Value(context.Background(), "printf hello")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestAFailedValueKeepsItsStdoutOutOfTheError(t *testing.T) {
	_, err := NewShell(Sh).Value(context.Background(), "printf 's3c%sret' ''; exit 1")
	if !errors.Is(err, ErrAction) {
		t.Fatalf("got %v, want ErrAction", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("got %q, want the value kept out", err.Error())
	}
}

func TestShKeepsStderrOutOfTheValue(t *testing.T) {
	got, err := NewShell(Sh).Value(context.Background(), "printf noise >&2; printf hello")
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want %q", got, "hello")
	}
}

func TestShSaysWhyARealCommandFailed(t *testing.T) {
	_, err := NewShell(Sh).Value(context.Background(), "printf 'no such thing' >&2; exit 1")
	if !errors.Is(err, ErrAction) {
		t.Fatalf("got %v, want ErrAction", err)
	}
	if !strings.Contains(err.Error(), "no such thing") {
		t.Errorf("got %q, want it to carry what the command printed", err.Error())
	}
}

func TestShSaysACommandWasKilled(t *testing.T) {
	err := NewShell(Sh).Stream(context.Background(), "kill -9 $$", io.Discard)
	if !errors.Is(err, ErrKilled) {
		t.Fatalf("got %v, want ErrKilled", err)
	}
}

func TestShSaysACommandWhoseChildWasKilled(t *testing.T) {
	err := NewShell(Sh).Stream(context.Background(), "true && sh -c 'kill -9 $$'", io.Discard)
	if !errors.Is(err, ErrKilled) {
		t.Fatalf("got %v, want ErrKilled", err)
	}
}

func TestShDoesNotCallAFailedCommandKilled(t *testing.T) {
	err := NewShell(Sh).Stream(context.Background(), "exit 3", io.Discard)
	if errors.Is(err, ErrKilled) {
		t.Fatalf("got %v, want a plain failure", err)
	}
}

func TestStreamShowsStdoutAndStderrWhileTheCommandRuns(t *testing.T) {
	flag := filepath.Join(t.TempDir(), "seen")
	out := &flagOnWrite{flag: flag}
	command := "printf out; printf err >&2; while [ ! -f " + flag + " ]; do sleep 0.01; done"
	if err := NewShell(Sh).Stream(t.Context(), command, out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got := out.data.String(); !strings.Contains(got, "out") || !strings.Contains(got, "err") {
		t.Errorf("got %q, want both stdout and stderr", got)
	}
}

func TestStreamEndsTheOutputOnItsOwnLine(t *testing.T) {
	var out bytes.Buffer
	if err := NewShell(Sh).Stream(context.Background(), "printf ready", &out); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if out.String() != "ready\n" {
		t.Errorf("got %q, want the output closed by a newline", out.String())
	}
}

func TestAFailedCommandReportsItsExitAndTheTailOfBothStreams(t *testing.T) {
	err := NewShell(Sh).Stream(context.Background(), "echo out-line; echo err-line >&2; exit 3", io.Discard)
	if !errors.Is(err, ErrAction) {
		t.Fatalf("got %v, want ErrAction", err)
	}
	for _, want := range []string{"exit status 3", "out-line", "err-line"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("got %q, want %q in it", err.Error(), want)
		}
	}
}

func TestAFailedCommandReportsOnlyTheLastTenLines(t *testing.T) {
	err := NewShell(Sh).Stream(context.Background(), "seq 1 30; exit 1", io.Discard)
	lines := strings.Split(err.Error(), "\n")
	if !strings.Contains(err.Error(), "\n21\n") || strings.Contains(err.Error(), "\n20\n") || lines[len(lines)-1] != "30" {
		t.Errorf("got %q, want lines 21 to 30 only", err.Error())
	}
}

func TestATailHoldsAtMostItsBytesOfOneHugeLine(t *testing.T) {
	var stream tail
	line := bytes.Repeat([]byte("x"), 10*tailBytes)
	for range 4 {
		stream.Write(line)
	}
	if len(stream.data) != tailBytes {
		t.Errorf("got %d bytes, want %d", len(stream.data), tailBytes)
	}
}

func TestATailCutsNoCharacterInHalf(t *testing.T) {
	var stream tail
	stream.Write([]byte(strings.Repeat("é", tailBytes) + "x"))
	if !utf8.Valid(stream.data) {
		t.Errorf("got %q, want whole characters only", stream.data[:8])
	}
}

func TestAFailedMuteReportsItsExitWithoutItsOutput(t *testing.T) {
	err := NewShell(Sh).Mute(context.Background(), "printf 's3c%sret' ''; printf 's3c%sret' '' >&2; exit 3")
	if !errors.Is(err, ErrAction) || !strings.Contains(err.Error(), "exit status 3") {
		t.Fatalf("got %v, want ErrAction with exit status 3", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Errorf("got %q, want the output kept out", err.Error())
	}
}

func TestAFailedWriteToTheOutputLetsTheCommandFinish(t *testing.T) {
	flag := filepath.Join(t.TempDir(), "finished")
	err := NewShell(Sh).Stream(context.Background(), "echo one; echo two; touch "+flag, brokenWriter{})
	if !errors.Is(err, ErrOutput) {
		t.Fatalf("got %v, want ErrOutput", err)
	}
	if _, err := os.Stat(flag); err != nil {
		t.Errorf("the command did not finish: %v", err)
	}
}

func TestDryNamesTheCommandWithoutRunningIt(t *testing.T) {
	value, err := NewShell(Dry).Value(context.Background(), "touch /tmp/vessel-dry")
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if value != "<not run: touch /tmp/vessel-dry>" {
		t.Errorf("got %q, want the command named", value)
	}
	if _, err := os.Stat("/tmp/vessel-dry"); !os.IsNotExist(err) {
		t.Error("Dry ran the command")
	}
}

type flagOnWrite struct {
	flag string
	data bytes.Buffer
}

func (f *flagOnWrite) Write(p []byte) (int, error) {
	f.data.Write(p)
	if strings.Contains(f.data.String(), "out") && strings.Contains(f.data.String(), "err") {
		if err := os.WriteFile(f.flag, nil, 0o600); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}

type brokenWriter struct{}

func (brokenWriter) Write([]byte) (int, error) { return 0, errors.New("terminal gone") }
