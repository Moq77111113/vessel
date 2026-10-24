package release

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Moq77111113/vessel/internal/atomicfile"
	"github.com/Moq77111113/vessel/pkg/delivery"
)

const (
	recordName   = "record.json"
	previousName = "record.previous.json"
)

const Format = 2

type Step string

const (
	StepValues   Step = "values"
	StepActions  Step = "actions"
	StepFiles    Step = "files"
	StepSecrets  Step = "secrets"
	StepImages   Step = "images"
	StepServices Step = "services"
)

type Action struct {
	Command string    `json:"command"`
	End     time.Time `json:"end,omitzero"`
}

type Release struct {
	Version string `json:"version"`
	Root    string `json:"root"`
}

type Entry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

type Record struct {
	Format   int                `json:"format"`
	Name     string             `json:"name"`
	Version  string             `json:"version"`
	Machine  string             `json:"machine"`
	Platform string             `json:"platform"`
	Root     string             `json:"root"`
	Files    []Entry            `json:"files"`
	Images   []string           `json:"images"`
	Secrets  []string           `json:"secrets"`
	Services []delivery.Service `json:"services,omitempty"`
	Insecure bool               `json:"insecure,omitempty"`
	Steps    []Step             `json:"steps,omitempty"`
	Actions  []Action           `json:"actions,omitempty"`
	Prior    Release            `json:"prior,omitzero"`
	Start    time.Time          `json:"start"`
	End      time.Time          `json:"end,omitzero"`
}

func (r Record) Done() bool { return !r.End.IsZero() }

func (r Record) Has(step Step) bool { return slices.Contains(r.Steps, step) }

func (r Record) OpenAction() (Action, bool) {
	if len(r.Actions) == 0 {
		return Action{}, false
	}
	last := r.Actions[len(r.Actions)-1]
	return last, last.End.IsZero()
}

type Records struct {
	dir string
}

func NewRecords(dir string) *Records {
	return &Records{dir: dir}
}

func Deliveries(root string) ([]string, error) {
	dir := deliveriesDir(root)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if delivery.CheckName(entry.Name()) != nil {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, entry.Name(), recordName)); err != nil {
			continue
		}
		names = append(names, entry.Name())
	}
	return names, nil
}

func (r *Records) Read() (Record, bool, error) { return read(filepath.Join(r.dir, recordName)) }

func (r *Records) Previous() (Record, bool, error) {
	return read(filepath.Join(r.dir, previousName))
}

func (r *Records) Remove() error {
	return atomicfile.Rename(filepath.Join(r.dir, recordName), filepath.Join(r.dir, previousName))
}

func (r *Records) Write(record Record) error {
	if err := atomicfile.MkdirAll(r.dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(r.dir, recordName)
	current, found, err := read(path)
	if err != nil {
		return err
	}
	record.Format = Format
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the record: %w", err)
	}
	if found && current.Done() && current.Version != record.Version {
		if err := keep(path, filepath.Join(r.dir, previousName)); err != nil {
			return err
		}
	}
	return atomicfile.Write(path, body, 0o600)
}

func keep(from, to string) error {
	body, err := os.ReadFile(from)
	if err != nil {
		return fmt.Errorf("read %s: %w", from, err)
	}
	return atomicfile.Write(to, body, 0o600)
}

func read(path string) (Record, bool, error) {
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, fmt.Errorf("read %s: %w", path, err)
	}
	var record Record
	if err := json.Unmarshal(body, &record); err != nil {
		return Record{}, false, fmt.Errorf("decode %s: %w", path, err)
	}
	return record, true, nil
}
