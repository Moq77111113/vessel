package machine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const (
	recordName   = "record.json"
	previousName = "record.previous.json"
)

// Entry is one file a delivery put on this machine.
type Entry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
}

// Record is what one delivery put on this machine.
type Record struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Machine string    `json:"machine"`
	Root    string    `json:"root"`
	Files   []Entry   `json:"files"`
	Images  []string  `json:"images"`
	Secrets []string  `json:"secrets"`
	Start   time.Time `json:"start"`
	End     time.Time `json:"end,omitzero"`
}

// Done reports whether the install that opened this record ran to its end.
func (r Record) Done() bool { return !r.End.IsZero() }

// Records is the install history one delivery keeps on this machine.
type Records struct {
	dir string
}

// NewRecords returns the record store of a delivery under the target root.
func NewRecords(root, name string) *Records {
	return &Records{dir: filepath.Join(root, valuesPath, name)}
}

// Read returns the record this machine holds, and whether it holds one.
func (r *Records) Read() (Record, bool, error) { return read(filepath.Join(r.dir, recordName)) }

// Previous returns the record the last write pushed aside.
func (r *Records) Previous() (Record, bool, error) {
	return read(filepath.Join(r.dir, previousName))
}

// Retire moves the record aside as the previous one, so this machine no longer claims to hold it.
func (r *Records) Retire() error {
	return rename(filepath.Join(r.dir, recordName), filepath.Join(r.dir, previousName))
}

// Write replaces the record through a temporary file, so a power cut leaves it readable.
func (r *Records) Write(record Record) error {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", r.dir, err)
	}
	path := filepath.Join(r.dir, recordName)
	current, found, err := read(path)
	if err != nil {
		return err
	}
	if found && current.Version != record.Version {
		if err := rename(path, filepath.Join(r.dir, previousName)); err != nil {
			return err
		}
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode the record: %w", err)
	}
	temp, err := os.CreateTemp(r.dir, ".record-*")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", r.dir, err)
	}
	defer os.Remove(temp.Name())

	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return fmt.Errorf("write %s: %w", path, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close %s: %w", path, err)
	}
	if err := os.Chmod(temp.Name(), 0o600); err != nil {
		return fmt.Errorf("set the mode of %s: %w", path, err)
	}
	return rename(temp.Name(), path)
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

func rename(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("move into %s: %w", to, err)
	}
	return nil
}
